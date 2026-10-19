#!/usr/bin/env bash
# ci-watch.sh: the supervisor's ONE CI watcher (main + every open PR). Run it with run_in_background.
# Polls ci-state.sh every 2 minutes and exits when the state changes from what it was
# at start (an item finished, went red, or appeared; a check count, queued->running or Dependabot
# churn doesn't count), printing the new state. Exits at
# once if nothing is pending. Records its pid in supervisor/ci-watch.pid, which the Stop hook
# (.claude/hooks/supervisor-ci-guard.sh) checks: a supervisor can't end a turn with CI pending and no
# live watcher. Starting a new one replaces (kills) the previous one.
set -uo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
W=$FLEET_SUPERVISOR_DIR
pidf="$W/ci-watch.pid"
old=$(cat "$pidf" 2>/dev/null) && [ -n "$old" ] && [ "$old" != $$ ] && kill "$old" 2>/dev/null
echo $$ > "$pidf"
trap '[ "$(cat "$pidf" 2>/dev/null)" = $$ ] && rm -f "$pidf"' EXIT
# The wake-up key: Dependabot PRs still running and main's head sha (it moves with every Dependabot merge)
# are left out, so routine Dependabot churn doesn't wake the supervisor; a red one still does.
key() { grep -v '^PENDING PR #[0-9]* (dependabot)' | sed -E "s/: [0-9]+ check\(s\) running//; s/ \((queued|in_progress|waiting|pending|requested)\)\$//; s/(for )[0-9a-f]{7}/\1HEAD/"; }
start=$("$here/ci-state.sh"); rc=$?
[ $rc = 10 ] || { echo "$start"; echo "exit=$rc"; exit 0; }
while sleep 120; do
  now=$("$here/ci-state.sh"); rc=$?
  [ "$(key <<<"$now")" = "$(key <<<"$start")" ] && [ $rc = 10 ] && continue
  echo "$now"; echo "exit=$rc"; exit 0
done
