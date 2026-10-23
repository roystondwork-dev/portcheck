#!/bin/sh
# Uploads a new version of the browser extension to Microsoft Edge Add-ons and
# submits it for certification, with the Edge Add-ons API (v1.1).
#
#   EDGE_CLIENT_ID=… EDGE_API_KEY=… EDGE_PRODUCT_ID=… scripts/edge-publish.sh store.zip "notes"
#
# Credentials: Partner Center → Microsoft Edge → Publish API ("Create API
# credentials"). EDGE_PRODUCT_ID is the product ID shown there (a GUID, not the
# store ID).
set -eu

zip="$1"
notes="${2:-}"
api="${EDGE_API_BASE:-https://api.addons.microsoftedge.microsoft.com}/v1/products/$EDGE_PRODUCT_ID/submissions"
auth() { printf 'Authorization: ApiKey %s\nX-ClientID: %s\n' "$EDGE_API_KEY" "$EDGE_CLIENT_ID"; }

# Runs a request that starts an operation; prints the operation ID (Location).
start() {
	headers="$(mktemp)"
	code="$(curl -sS -o /dev/null -D "$headers" -w '%{http_code}' -H @- "$@" <<EOF2
$(auth)
EOF2
)"
	[ "$code" = 202 ] || { echo "Edge Add-ons API answered HTTP $code" >&2; exit 1; }
	tr -d '\r' <"$headers" | awk 'tolower($1) == "location:" { print $2 }' | sed 's#.*/##'
}

# Polls an operation until it finishes; fails unless it succeeded.
wait_for() {
	for _ in $(seq 1 60); do
		body="$(curl -sS -H @- "$1" <<EOF2
$(auth)
EOF2
)"
		status="$(printf '%s' "$body" | sed -n 's/.*"status" *: *"\([A-Za-z]*\)".*/\1/p')"
		case "$status" in
			Succeeded) return 0 ;;
			InProgress | "") sleep 10 ;;
			*)
				echo "Edge Add-ons: $status: $body" >&2
				exit 1
				;;
		esac
	done
	echo "Edge Add-ons: timed out" >&2
	exit 1
}

echo "Uploading $zip to Edge Add-ons..."
op="$(start -X POST -H 'Content-Type: application/zip' --data-binary "@$zip" "$api/draft/package")"
wait_for "$api/draft/package/operations/$op"

echo "Submitting for certification..."
payload="$(printf '{"notes":"%s"}' "$(printf '%s' "$notes" | sed 's/["\\]//g')")"
op="$(start -X POST -H 'Content-Type: application/json' --data "$payload" "$api")"
wait_for "$api/operations/$op"
echo "Submitted. Microsoft certifies it, usually within a few days."
