"""Builds the PyPI wheels of portcheck from a release's archives.

One wheel per OS and CPU, each carrying the portcheck binary as a script, so
`pip install portcheck-tool` puts `portcheck` on PATH with no Python wrapper and
nothing to download at runtime (as ruff and uv do). Every archive is checked
against the release's checksums.txt first.

    python3 packaging/pypi/build_wheels.py <version> <assets-dir> <out-dir>
"""

from __future__ import annotations

import base64
import hashlib
import io
import sys
import tarfile
import zipfile
from pathlib import Path

NAME = "portcheck_tool"  # the distribution "portcheck-tool", normalised for file names

# Release archive suffix → wheel platform tags. The Linux binaries are static,
# so they run on glibc (manylinux) and musl (Alpine) alike. Go 1.27 needs
# macOS 12 or later.
TARGETS = {
    "darwin_arm64.tar.gz": ["macosx_12_0_arm64"],
    "darwin_amd64.tar.gz": ["macosx_12_0_x86_64"],
    "linux_amd64.tar.gz": ["manylinux_2_17_x86_64", "manylinux2014_x86_64", "musllinux_1_1_x86_64"],
    "linux_arm64.tar.gz": ["manylinux_2_17_aarch64", "manylinux2014_aarch64", "musllinux_1_1_aarch64"],
    "windows_amd64.zip": ["win_amd64"],
    "windows_arm64.zip": ["win_arm64"],
}

ROOT = Path(__file__).resolve().parents[2]


def metadata(version: str) -> str:
    readme = (Path(__file__).parent / "README.md").read_text()
    return (
        "Metadata-Version: 2.4\n"
        "Name: portcheck-tool\n"
        f"Version: {version}\n"
        "Summary: See what's using a port and free it safely: which process, who started it, stop it. "
        'Fixes "port already in use".\n'
        "Author: Royston D\n"
        "License-Expression: MIT\n"
        "License-File: LICENSE\n"
        "Project-URL: Homepage, https://roystondwork-dev.github.io/portcheck/\n"
        "Project-URL: Source, https://github.com/roystondwork-dev/portcheck\n"
        "Project-URL: Issues, https://github.com/roystondwork-dev/portcheck/issues\n"
        "Keywords: port,eaddrinuse,port-in-use,kill-port,lsof,netstat,localhost,cli\n"
        "Classifier: Development Status :: 4 - Beta\n"
        "Classifier: Environment :: Console\n"
        "Classifier: Intended Audience :: Developers\n"
        "Classifier: Operating System :: MacOS\n"
        "Classifier: Operating System :: POSIX :: Linux\n"
        "Classifier: Operating System :: Microsoft :: Windows\n"
        "Classifier: Topic :: System :: Networking\n"
        "Classifier: Topic :: Software Development\n"
        "Requires-Python: >=3.8\n"
        "Description-Content-Type: text/markdown\n"
        "\n" + readme
    )


def record_line(path: str, data: bytes) -> str:
    digest = base64.urlsafe_b64encode(hashlib.sha256(data).digest()).rstrip(b"=").decode()
    return f"{path},sha256={digest},{len(data)}"


def binary_from(archive: Path) -> tuple[str, bytes]:
    """The portcheck binary inside a release archive: (file name, bytes)."""
    if archive.name.endswith(".zip"):
        with zipfile.ZipFile(archive) as z:
            return "portcheck.exe", z.read("portcheck.exe")
    with tarfile.open(archive) as t:
        f = t.extractfile("portcheck")
        assert f is not None
        return "portcheck", f.read()


def build_wheel(version: str, archive: Path, tags: list[str], out: Path) -> Path:
    exe, binary = binary_from(archive)
    data = f"{NAME}-{version}.data"
    info = f"{NAME}-{version}.dist-info"
    files: list[tuple[str, bytes, int]] = [
        (f"{data}/scripts/{exe}", binary, 0o755),
        (f"{info}/METADATA", metadata(version).encode(), 0o644),
        (
            f"{info}/WHEEL",
            (
                "Wheel-Version: 1.0\nGenerator: portcheck build_wheels.py\nRoot-Is-Purelib: false\n"
                + "".join(f"Tag: py3-none-{t}\n" for t in tags)
            ).encode(),
            0o644,
        ),
        (f"{info}/licenses/LICENSE", (ROOT / "LICENSE").read_bytes(), 0o644),
    ]
    record = "\n".join([record_line(p, d) for p, d, _ in files] + [f"{info}/RECORD,,"]) + "\n"
    files.append((f"{info}/RECORD", record.encode(), 0o644))

    wheel = out / f"{NAME}-{version}-py3-none-{'.'.join(tags)}.whl"
    with zipfile.ZipFile(wheel, "w", zipfile.ZIP_DEFLATED) as z:
        for path, content, mode in files:
            entry = zipfile.ZipInfo(path, date_time=(1980, 1, 1, 0, 0, 0))  # reproducible
            entry.external_attr = (0o100000 | mode) << 16
            entry.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(entry, content)
    return wheel


def main(version: str, assets: Path, out: Path) -> None:
    sums = {}
    for line in (assets / "checksums.txt").read_text().splitlines():
        parts = line.split()
        if len(parts) == 2:
            sums[parts[1].lstrip("*")] = parts[0]
    out.mkdir(parents=True, exist_ok=True)
    archives = []
    # Check every archive before building any wheel.
    for suffix in TARGETS:
        archive = assets / f"portcheck_{version}_{suffix}"
        got = hashlib.sha256(archive.read_bytes()).hexdigest()
        if sums.get(archive.name) != got:
            sys.exit(f"checksum mismatch for {archive.name}")
        archives.append(archive)
    for archive, tags in zip(archives, TARGETS.values()):
        print(build_wheel(version, archive, tags, out).name)


if __name__ == "__main__":
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    main(sys.argv[1], Path(sys.argv[2]), Path(sys.argv[3]))
