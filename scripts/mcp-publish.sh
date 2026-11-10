#!/bin/sh
# Publishes server.json to the official MCP Registry, as the GitHub OIDC
# identity of the workflow running it. TAG is the release tag (e.g. v0.12.0);
# server.json must carry the same version. Used by release.yml and
# mcp-registry.yml.
set -eu

: "${TAG:?TAG is the release tag, e.g. v0.12.0}"
MCP_PUBLISHER=v1.8.4
MCP_PUBLISHER_SHA256=5dc2fd415369c73551259f31cd33e2aacc9e0d1021e7f08fef1492d93b781628
tmp="${RUNNER_TEMP:-$(mktemp -d)}"

version=$(jq -r .version server.json)
if [ "v$version" != "$TAG" ]; then
	echo "::error::server.json has $version, expected ${TAG#v} (release-please keeps them in sync)"
	exit 1
fi

curl -fsSL -o "$tmp/mcp-publisher.tar.gz" \
	"https://github.com/modelcontextprotocol/registry/releases/download/$MCP_PUBLISHER/mcp-publisher_linux_amd64.tar.gz"
echo "$MCP_PUBLISHER_SHA256  $tmp/mcp-publisher.tar.gz" | sha256sum -c -
tar xzf "$tmp/mcp-publisher.tar.gz" -C "$tmp" mcp-publisher

# The registry checks the npm package (its mcpName) and fails while npm
# doesn't serve the new version yet, which can take minutes after publishing.
for _ in $(seq 1 30); do
	curl -fsS -o /dev/null "https://registry.npmjs.org/portcheck-cli/$version" && break
	sleep 20
done

"$tmp/mcp-publisher" login github-oidc
for attempt in 1 2 3 4 5; do
	if "$tmp/mcp-publisher" publish; then
		exit 0
	fi
	echo "publish attempt $attempt failed; retrying in 60 s"
	sleep 60
done
echo "::error::couldn't publish to the MCP Registry"
exit 1
