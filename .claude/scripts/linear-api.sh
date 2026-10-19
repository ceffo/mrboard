#!/usr/bin/env bash
# linear-api.sh: sends one GraphQL query to Linear's API and prints the JSON response.
#
#   linear-api.sh '<query>' ['<variables as JSON>']      # or the query on stdin
#
# Authentication: $LINEAR_API_KEY, else the first line of $FLEET_CONFIG_DIR/linear-api-key
# (default ~/.config/agents-dev-fleet/linear-api-key, mode 600). The key goes to curl through a private
# temporary file, never in an argument, and is never printed. $LINEAR_API_URL overrides the endpoint (tests).
#
# Exit 0: response without errors. Exit 1: anything else, with one line on stderr (no key, unreachable,
# HTTP error, invalid JSON, a GraphQL `errors` field; the response is still printed on stdout).
set -euo pipefail
umask 077
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"

fail() { echo "linear: $*" >&2; exit 1; }

key="${LINEAR_API_KEY:-}"
if [ -z "$key" ]; then
  keyfile="$FLEET_CONFIG_DIR/linear-api-key"
  [ ! -r "$keyfile" ] || IFS= read -r key <"$keyfile" || true
fi
[ -n "$key" ] || fail "no API key: export LINEAR_API_KEY or put it in $FLEET_CONFIG_DIR/linear-api-key (chmod 600)"

if [ $# -ge 1 ]; then query="$1"; else query="$(cat)"; fi
[ -n "$query" ] || fail "no query given"
variables="${2:-{\}}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
jq -n --arg q "$query" --argjson v "$variables" '{query: $q, variables: $v}' >"$tmp/body" \
  || fail "the variables are not valid JSON"
# Personal API keys go in Authorization as is, without "Bearer".
printf 'Authorization: %s\n' "$key" >"$tmp/headers"

code="$(curl -sS --max-time 30 -o "$tmp/response" -w '%{http_code}' \
  -H @"$tmp/headers" -H 'Content-Type: application/json' --data-binary @"$tmp/body" \
  "${LINEAR_API_URL:-https://api.linear.app/graphql}" 2>"$tmp/curl-err")" \
  || fail "cannot reach Linear: $(sed 's/Authorization:.*//' "$tmp/curl-err" | head -n1)"

if [ "$code" != 200 ]; then
  msg="$(jq -r '[.errors[]?.message] | join("; ")' "$tmp/response" 2>/dev/null || true)"
  fail "HTTP $code${msg:+: $msg}"
fi
jq -e . "$tmp/response" >/dev/null 2>&1 || fail "the response is not JSON"
cat "$tmp/response"
if jq -e '.errors | type == "array" and length > 0' "$tmp/response" >/dev/null; then
  fail "GraphQL error: $(jq -r '[.errors[].message] | join("; ")' "$tmp/response")"
fi
