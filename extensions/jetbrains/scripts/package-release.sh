#!/usr/bin/env bash
# Builds the release plugin: one zip for every platform, carrying portcheck for
# each OS and CPU in bin/<os>-<arch>/ (the plugin picks the right one at
# runtime). Every archive is checked against the release's checksums.txt first.
#
# Usage: scripts/package-release.sh <version> <assets-dir> <out-dir>
#   <assets-dir> holds checksums.txt and the portcheck_<version>_<os>_<arch>
#   archives of a release, e.g. from `gh release download v0.9.0`.
set -euo pipefail

version=$1
assets=$(cd "$2" && pwd)
out=$(mkdir -p "$3" && cd "$3" && pwd)
cd "$(dirname "$0")/.."

plugin_version=$(sed -n 's/^pluginVersion = \([^ ]*\).*/\1/p' gradle.properties)
if [ "$plugin_version" != "$version" ]; then
  echo "gradle.properties has version $plugin_version, expected $version (release-please keeps them in sync)" >&2
  exit 1
fi

sha256() { if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1; }

archive() { # archive file name for a portcheck os_arch
  if [[ $1 == windows_* ]]; then echo "portcheck_${version}_$1.zip"; else echo "portcheck_${version}_$1.tar.gz"; fi
}

targets="darwin_arm64 darwin_amd64 linux_amd64 linux_arm64 windows_amd64 windows_arm64"

# Check every archive before packaging any, so a bad one stops the release early.
for t in $targets; do
  name=$(archive "$t")
  want=$(awk -v n="$name" '$2 == n { print $1 }' "$assets/checksums.txt")
  got=$(sha256 "$assets/$name")
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    echo "checksum mismatch for $name (want '${want:-missing}', got $got)" >&2
    exit 1
  fi
done

bin=$(mktemp -d)
trap 'rm -rf "$bin"' EXIT
for t in $targets; do
  dir="$bin/${t/_/-}" # darwin_arm64 → darwin-arm64, as the plugin looks it up
  name=$(archive "$t")
  mkdir -p "$dir"
  if [[ $name == *.zip ]]; then
    unzip -q -j "$assets/$name" portcheck.exe -d "$dir"
  else
    tar -xzf "$assets/$name" -C "$dir" portcheck
  fi
done
chmod 755 "$bin"/*/portcheck*

gradle buildPlugin -PportcheckBin="$bin" --no-build-cache --rerun-tasks --console=plain
cp "build/distributions/portcheck-jetbrains-$version.zip" "$out/"
