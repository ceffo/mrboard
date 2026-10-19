#!/usr/bin/env bash
# Stop hook: a supervisor session can't end its turn while CI it owns is pending with no watcher, or
# red and not handled, or while it has live agents and no fleet watcher (.claude/scripts/fleet-watch.sh). CI = the default branch's CI runs (and release run, if configured) and every open PR's checks
# (.claude/scripts/ci-state.sh). Other agents are not affected.
#   pending + no live ci-watch.sh  -> blocked: arm .claude/scripts/ci-watch.sh (run_in_background)
#   red, not in supervisor/ci-red-handled -> blocked: act on it, then record it there
# On the hook's own re-entry (stop_hook_active) it lets the turn end, so it can't loop.
set -uo pipefail
in=$(cat)
t=$(jq -r '.transcript_path // empty' <<<"$in")
[ -f "$t" ] && head -c 20000 "$t" | grep -q '"agentSetting":"supervisor"' || exit 0
[ "$(jq -r '.stop_hook_active // false' <<<"$in")" = true ] && exit 0
. "${CLAUDE_PROJECT_DIR:?}/.claude/scripts/fleet-lib.sh"
W=$FLEET_SUPERVISOR_DIR
state=$(timeout 60 "$FLEET_SCRIPTS/ci-state.sh" 2>&1); rc=$?
pid=$(cat "$W/ci-watch.pid" 2>/dev/null)
alive=no; [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && alive=yes
reason=""
if [ $rc = 20 ]; then
  reason="CI is RED and unhandled:
$state
CI is the supervisor's job. Act now (rework brief to the branch's developer, a fresh developer for the default branch or a Dependabot PR, or a rerun for a flake), then append each handled id (run id or pr<n>@<sha7> as printed) with one line on what you did to $W/ci-red-handled, and keep .claude/scripts/ci-watch.sh running."
elif [ $rc = 10 ] && [ $alive = no ]; then
  reason="CI is running and no watcher is armed:
$state
Start .claude/scripts/ci-watch.sh with run_in_background (it replaces any previous watcher) before ending the turn."
fi
# Live agents (anything but sup-*) need the fleet watcher: an agent that finished and waits on the
# supervisor must wake it, whatever it wrote where.
agents=$(herdr agent list 2>/dev/null | jq -r '[.result.agents[]|select(.name|startswith("sup-")|not)|.name]|join(" ")')
fpid=$(cat "$W/fleet-watch.pid" 2>/dev/null)
if [ -n "$agents" ] && ! { [ -n "$fpid" ] && kill -0 "$fpid" 2>/dev/null; }; then
  reason="${reason:+$reason

}Live agents ($agents) and no fleet watcher: start .claude/scripts/fleet-watch.sh with run_in_background before ending the turn. An agent waiting on you must never sit idle."
fi
[ -z "$reason" ] && exit 0
jq -n --arg r "$reason" '{decision:"block",reason:$r}'
