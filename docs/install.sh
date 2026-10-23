#!/bin/sh
# Installs portcheck and connects it to your browsers, in one step.
#
#   curl -fsSL https://roystondwork-dev.github.io/portcheck/install.sh | sh
#
# With Homebrew, it installs (or upgrades) the Homebrew package. Otherwise it
# downloads the latest release for this OS and CPU from GitHub, checks it
# against the release's checksums, and installs it to ~/.local/bin (or
# $PORTCHECK_INSTALL_DIR). Then it runs `portcheck browser install`, which
# registers portcheck with the browsers installed for this user.
#
# Options (environment variables):
#   PORTCHECK_INSTALL_DIR  install the binary here instead (skips Homebrew)
#   PORTCHECK_VERSION      a version such as 0.9.0 instead of the latest
#   PORTCHECK_NO_BROWSER=1 don't connect browsers
set -eu

REPO="roystondwork-dev/portcheck"

say() { printf '%s\n' "$*"; }
fail() { printf 'portcheck installer: %s\n' "$*" >&2; exit 1; }

fetch() { # url → stdout
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$1"
  else
    fail "curl or wget is required"
  fi
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

install_with_brew() {
  say "Installing portcheck with Homebrew..."
  if brew list --cask portcheck >/dev/null 2>&1; then
    brew upgrade --cask roystondwork-dev/tap/portcheck || true
  else
    brew install --cask roystondwork-dev/tap/portcheck
  fi
  PORTCHECK="$(command -v portcheck)"
}

install_from_release() {
  os="$(uname -s)"
  case "$os" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail "unsupported OS $os (on Windows, use install.ps1)" ;;
  esac
  arch="$(uname -m)"
  case "$arch" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) fail "unsupported CPU $arch" ;;
  esac

  version="${PORTCHECK_VERSION:-}"
  if [ -n "$version" ]; then
    base="https://github.com/$REPO/releases/download/v${version#v}"
  else
    # No API call (rate-limited per IP): the latest release's checksums name
    # its archives, which carry the version.
    base="https://github.com/$REPO/releases/latest/download"
  fi

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  fetch "$base/checksums.txt" >"$tmp/checksums.txt"
  archive="$(awk -v s="_${os}_${arch}.tar.gz" '{ n = $2; sub(/^\*/, "", n) } index(n, "portcheck_") == 1 && substr(n, length(n) - length(s) + 1) == s { print n; exit }' "$tmp/checksums.txt")"
  [ -n "$archive" ] || fail "no portcheck build for ${os}/${arch} in this release"
  version="${archive#portcheck_}"
  version="${version%"_${os}_${arch}.tar.gz"}"
  say "Downloading portcheck ${version} for ${os}/${arch}..."
  fetch "$base/$archive" >"$tmp/$archive"
  want="$(awk -v n="$archive" '$2 == n { print $1 }' "$tmp/checksums.txt")"
  got="$(sha256 "$tmp/$archive")"
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    fail "checksum mismatch for $archive"
  fi
  tar -xzf "$tmp/$archive" -C "$tmp" portcheck

  dir="${PORTCHECK_INSTALL_DIR:-$HOME/.local/bin}"
  mkdir -p "$dir"
  install -m 0755 "$tmp/portcheck" "$dir/portcheck" 2>/dev/null || {
    cp "$tmp/portcheck" "$dir/portcheck"
    chmod 0755 "$dir/portcheck"
  }
  PORTCHECK="$dir/portcheck"
  say "Installed $PORTCHECK"
  case ":$PATH:" in
    *":$dir:"*) ;;
    *) say "Note: $dir isn't on your PATH; add it to use 'portcheck' in a terminal." ;;
  esac
}

if [ -z "${PORTCHECK_INSTALL_DIR:-}" ] && [ -z "${PORTCHECK_VERSION:-}" ] && command -v brew >/dev/null 2>&1; then
  install_with_brew
else
  install_from_release
fi

"$PORTCHECK" version
if [ "${PORTCHECK_NO_BROWSER:-}" != "1" ]; then
  say ""
  if "$PORTCHECK" browser install >/dev/null; then
    "$PORTCHECK" browser status -o plain
    say ""
    say "Done. The portcheck extension connects by itself."
  else
    say "No supported browser found to connect; run 'portcheck browser install' after installing one."
  fi
fi
