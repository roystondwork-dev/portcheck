#!/bin/sh
# Installs portcheck in a dev container. Runs as root at image build time.
# The download is checked against the release's checksums by portcheck-install.sh,
# which is the project's one-line installer (docs/install.sh), copied here when
# the feature is published.
set -eu

VERSION="${VERSION:-latest}"

need() { command -v "$1" >/dev/null 2>&1; }
if ! need curl && ! need wget; then
	if need apt-get; then
		apt-get update -y && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates curl
		rm -rf /var/lib/apt/lists/*
	elif need apk; then
		apk add --no-cache ca-certificates curl
	elif need dnf; then
		dnf install -y ca-certificates curl
	elif need microdnf; then
		microdnf install -y ca-certificates curl
	else
		echo "portcheck feature: curl or wget is needed" >&2
		exit 1
	fi
fi

export PORTCHECK_INSTALL_DIR=/usr/local/bin PORTCHECK_NO_BROWSER=1
if [ "$VERSION" != "latest" ]; then
	export PORTCHECK_VERSION="${VERSION#v}"
fi
sh "$(dirname "$0")/portcheck-install.sh"
portcheck version
