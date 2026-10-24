#!/usr/bin/env bash
# Builds the portcheck-tray release archives. Runs on macOS (the menu bar app
# needs cgo and Xcode's tools; Windows and Linux builds are pure Go):
#   portcheck-tray_<v>_darwin_universal.zip   portcheck tray.app (arm64 + x86_64)
#   portcheck-tray_<v>_windows_<arch>.zip     portcheck-tray.exe (no console window)
#   portcheck-tray_<v>_linux_<arch>.tar.gz
#
# Usage: scripts/package-tray.sh <version> <out-dir>
set -euo pipefail

version=${1#v}
out=$(mkdir -p "$2" && cd "$2" && pwd)
cd "$(dirname "$0")/.."
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
ldflags="-s -w"

# macOS: a universal menu bar app.
for arch in arm64 amd64; do
  clangarch=$([ "$arch" = amd64 ] && echo x86_64 || echo arm64)
  CGO_ENABLED=1 GOOS=darwin GOARCH=$arch CC="clang -arch $clangarch" \
    go build -trimpath -ldflags "$ldflags" -o "$work/tray-$arch" ./cmd/portcheck-tray
done
app="$work/portcheck tray.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
lipo -create -output "$app/Contents/MacOS/portcheck-tray" "$work/tray-arm64" "$work/tray-amd64"
cat >"$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>portcheck tray</string>
  <key>CFBundleDisplayName</key><string>portcheck tray</string>
  <key>CFBundleIdentifier</key><string>io.github.roystondwork-dev.portcheck-tray</string>
  <key>CFBundleVersion</key><string>$version</string>
  <key>CFBundleShortVersionString</key><string>$version</string>
  <key>CFBundleExecutable</key><string>portcheck-tray</string>
  <key>CFBundleIconFile</key><string>portcheck</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST
iconset="$work/portcheck.iconset"
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
  for scale in 1 2; do
    px=$((size * scale))
    suffix=""
    if [ "$scale" = 2 ]; then suffix="@2x"; fi
    name="icon_${size}x${size}${suffix}.png"
    sed "s#width=\"256\" height=\"256\"#width=\"$px\" height=\"$px\"#" extensions/browser/store/icon.svg >"$work/i.svg"
    sips -s format png "$work/i.svg" --out "$iconset/$name" >/dev/null
  done
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/portcheck.icns"
# Ad-hoc signature: required on Apple Silicon; not notarized (no Apple
# Developer account), so the first open needs right-click → Open, or Homebrew.
codesign --force --deep --sign - "$app"
(cd "$work" && ditto -c -k --keepParent "portcheck tray.app" "$out/portcheck-tray_${version}_darwin_universal.zip")

# Windows and Linux.
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=windows GOARCH=$arch go build -trimpath -ldflags "$ldflags -H windowsgui" -o "$work/win-$arch/portcheck-tray.exe" ./cmd/portcheck-tray
  (cd "$work/win-$arch" && zip -q "$out/portcheck-tray_${version}_windows_$arch.zip" portcheck-tray.exe)
  CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -ldflags "$ldflags" -o "$work/linux-$arch/portcheck-tray" ./cmd/portcheck-tray
  tar -czf "$out/portcheck-tray_${version}_linux_$arch.tar.gz" -C "$work/linux-$arch" portcheck-tray
done
(cd "$out" && shasum -a 256 portcheck-tray_"${version}"_* >"portcheck-tray_${version}_checksums.txt")
ls -1 "$out"
