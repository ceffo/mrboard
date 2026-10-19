#!/usr/bin/env bash
# supervisor-handoff.sh: the running supervisor (herdr agent sup-N) starts its successor sup-N+1
# in a new herdr tab on the main checkout, prompts it to take over, and waits for its ack file.
# The successor closes sup-N's tab once it has re-armed the waits. Run it from the supervisor's
# session after updating the ledger (refused if ledger.md is older than 15 minutes).
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
main=$FLEET_MAIN
sup=$FLEET_SUPERVISOR_DIR
ledger="$sup/ledger.md"
[ "${HERDR_ENV:-}" = 1 ] || { echo "not in herdr: hand off by hand (ledger + ask the user to start a supervisor)" >&2; exit 1; }
me="$(herdr agent list | jq -r --arg p "$HERDR_PANE_ID" '.result.agents[]|select(.pane_id==$p)|.name')"
[[ "$me" =~ ^sup-([0-9]+)$ ]] || { echo "this pane's agent is '$me', not sup-N" >&2; exit 1; }
n="${BASH_REMATCH[1]}"; next="sup-$((n + 1))"
[ -n "$(find "$ledger" -mmin -15)" ] || { echo "ledger.md older than 15 min: rewrite it first" >&2; exit 1; }
grep -q "Notes for the next supervisor ($me handoff" "$ledger" ||
  { echo "ledger has no 'Notes for the next supervisor ($me handoff' section" >&2; exit 1; }
ctx="$(herdr agent read "$me" --source recent-unwrapped --lines 8 2>/dev/null | grep -oE '[0-9.]+k' | tail -1 || true)"
ack="$sup/handoff-ack-$((n + 1))"; rm -f "$ack"
tab="$(herdr tab create --workspace "$HERDR_WORKSPACE_ID" --cwd "$main" --label "$next" --no-focus)"
pane="$(jq -r .result.root_pane.pane_id <<<"$tab")"
herdr agent start "$next" --kind claude --pane "$pane" -- --agent supervisor --model opus >/dev/null
sleep 15
herdr agent prompt "$next" "You take over from $me (handed off at ${ctx:-?} context). Read $ledger (start with 'Notes for the next supervisor ($me handoff') and the whole $sup/user-log.md, reconcile with live state, re-arm every wait, then touch $ack and close $me's tab: look it up with herdr agent list by name ($me, pane $HERDR_PANE_ID) and check it isn't yours. Keep replies to the user short." >/dev/null
for _ in $(seq 120); do [ -e "$ack" ] && { echo "ACK $next"; exit 0; }; sleep 15; done
echo "NO ACK from $next after 30 min: check its tab ($pane)" >&2; exit 1
