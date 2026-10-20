#!/usr/bin/env bash
# Builds the release VSIX: one package for every platform, carrying portcheck
# for each OS and CPU in bin/<platform>-<arch>/ (the extension picks the right
# one at runtime). One file keeps uploads simple, e.g. on the Marketplace's
# website. Every archive is checked against the release's checksums.txt first.
#
# Usage: scripts/package-platforms.sh <version> <assets-dir> <out-dir>
#   <assets-dir> holds checksums.txt and the portcheck_<version>_<os>_<arch>
#   archives of a release, e.g. from `gh release download v0.5.0`.
set -euo pipefail

version=$1
assets=$(cd "$2" && pwd)
out=$(mkdir -p "$3" && cd "$3" && pwd)
cd "$(dirname "$0")/.."

manifest_version=$(node -p 'require("./package.json").version')
if [ "$manifest_version" != "$version" ]; then
  echo "package.json has version $manifest_version, expected $version (release-please keeps them in sync)" >&2
  exit 1
fi

sha256() { if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1; }

# VS Code target : portcheck os_arch
targets="darwin-arm64:darwin_arm64 darwin-x64:darwin_amd64 linux-x64:linux_amd64 linux-arm64:linux_arm64 win32-x64:windows_amd64 win32-arm64:windows_arm64"

archive() { # archive file name for a portcheck os_arch
  if [[ $1 == windows_* ]]; then echo "portcheck_${version}_$1.zip"; else echo "portcheck_${version}_$1.tar.gz"; fi
}

# Check every archive before packaging any, so a bad one stops the release early.
for t in $targets; do
  name=$(archive "${t#*:}")
  want=$(awk -v n="$name" '$2 == n { print $1 }' "$assets/checksums.txt")
  got=$(sha256 "$assets/$name")
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    echo "checksum mismatch for $name (want '${want:-missing}', got $got)" >&2
    exit 1
  fi
done

npm run build
trap 'rm -rf bin' EXIT
rm -rf bin
for t in $targets; do
  target=${t%%:*}
  name=$(archive "${t#*:}")
  mkdir -p "bin/$target"
  if [[ $name == *.zip ]]; then
    unzip -q -j "$assets/$name" portcheck.exe -d "bin/$target"
  else
    tar -xzf "$assets/$name" -C "bin/$target" portcheck
  fi
done
chmod 755 bin/*/portcheck*
npx vsce package --no-dependencies --out "$out/portcheck-vscode-${version}.vsix"
