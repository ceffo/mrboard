#!/usr/bin/env bash
# fleet-watch.sh: the supervisor's ONE watcher over its live agents (every herdr agent except sup-*).
# Run it with run_in_background. Polls `herdr agent list` every 30 s and exits when an agent:
#   - becomes blocked (a permission prompt or question in its tab; not scoper-*/designer-*, whose
#     questions are for the user)                                                 -> "BLOCKED <name>"
#   - ends a turn (working -> idle/done) and its last screen speaks to the supervisor
#     (supervisor / ship / push / PR / merge), or its report file is newer than   -> "NEEDS-SUPERVISOR <name>"
#     the last prompt it got
#   - disappears                                                                   -> "GONE <name>"
#   - ends a turn on a question or a request to the user (herdr shows it as idle/done, not blocked)
#                                                                                  -> "WAITING-ON-USER <name>"
# Waiting (a question) or blocked: the agent's tab gets a "❓ " / "⛔ " prefix and a herdr notification
# (sound "request") so the user sees it; the prefix goes when the agent works again. scoper-* and
# designer-* talk to the user by design: they get the mark and the notification, but don't wake the
# supervisor. Agents already waiting when the watcher starts are marked, not reported.
# The agent in this watcher's own pane ($HERDR_PANE_ID, the supervisor) is never watched.
# A report already handled is marked with fleet-ack.sh <KEY>, so re-arming doesn't report it again.
# Only the agents of this herdr workspace ($HERDR_WORKSPACE_ID) are watched.
# An agent ending a turn that only talks to the user (live steps, a scoper's questions) doesn't wake it.
# Prints the agent's last lines. Records its pid in supervisor/fleet-watch.pid, which the Stop hook
# (hooks/supervisor-ci-guard.sh) checks: a supervisor can't end a turn with live agents and no
# fleet watcher. Starting a new one replaces (kills) the previous one.
set -uo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
W=$FLEET_SUPERVISOR_DIR
R=$FLEET_REPORTS_DIR
B=$FLEET_BRIEFS_DIR
pidf="$W/fleet-watch.pid"
old=$(cat "$pidf" 2>/dev/null) && [ -n "$old" ] && [ "$old" != $$ ] && kill "$old" 2>/dev/null
echo $$ > "$pidf"
trap '[ "$(cat "$pidf" 2>/dev/null)" = $$ ] && rm -f "$pidf"' EXIT
declare -A seen state
pending=""
WS=${HERDR_WORKSPACE_ID:-}
snap() { herdr agent list 2>/dev/null | jq -r --arg ws "$WS" --arg me "${HERDR_PANE_ID:-}" '.result.agents[]|select((.name // "sup-")|startswith("sup-")|not)|select($ws=="" or .workspace_id==$ws)|select($me=="" or .pane_id!=$me)|"\(.name) \(.agent_status) \(.state_change_seq) \(.tab_id)"'; }
# The agent's last message, without herdr/Claude chrome (status bar, prompt, recap, spinner lines).
last_msg() { herdr agent read "$1" --source recent-unwrapped --lines 40 2>/dev/null \
  | sed '/※ recap/,$d' | grep -vE '^\s*$|tokens ·|───|^\s*❯|auto mode|^\s*~/|^\s*✻' | tail -10; }
# A developer/reviewer that wrote its report is finished, not waiting: its summary often ends with a
# line for the user ("log in and confirm"), which is the supervisor's to relay, never a question.
finished() { ! user_facing "$1" && grep -qiE 'report (is|written) (in|to)|reports/[^ ]+\.md|^\s*STATUS: done' <<<"$2"; }
asks_user() { grep -qE '\?\s*$|\?["»)]?\s' <<<"$1" || grep -qiE "tell me|let me know|your (call|verdict|ok|go|choice)|once you('ve| have)|you run|run (this|these)|please (run|confirm|check|log in)|confirm (that|you)|waiting (on|for) you|which (one|do you)|for you to|over to you" <<<"$1"; }
label_of() { herdr tab list 2>/dev/null | jq -r --arg t "$1" '.result.tabs[]|select(.tab_id==$t)|.label'; }
mark() { # mark <tab> <prefix|""> : set or clear the tab's waiting prefix
  local l; l=$(label_of "$1"); [ -n "$l" ] || return 0
  local base=${l#❓ }; base=${base#⛔ }
  [ "$l" = "$2$base" ] || herdr tab rename "$1" "$2$base" >/dev/null 2>&1
}
notify() { herdr notification show "$1" --body "$2" --sound request >/dev/null 2>&1 || true; }
# Agents the user drives in their own tab: scoper-*, designer-*, and any name listed (one per line)
# in $W/user-facing-agents (e.g. a Fable session launched by the user's request).
user_facing() { [[ "$1" == scoper-* || "$1" == designer-* ]] || grep -qxF "$1" "$W/user-facing-agents" 2>/dev/null; }
fresh_report() { # the agent's report newer than the supervisor's last prompt for its item
  # (an agent suffix is dropped: dev-x-12b -> X-12). The marker is a .prompt-<item> file or, without one, the item's newest
  # brief (written just before each prompt), so a forgotten marker never hides a report.
  local item=${1#*-}; item=${item^^}; item=$(sed -E 's/([0-9]+)[A-Z]+$/\1/' <<<"$item"); local f p
  # The newest of all markers is the last prompt: a report must be newer than it.
  # Exact item only (a "-" or the end after it), so item 3 never matches item 34's files.
  p=$(ls -td "$W"/.ack-"$item" "$W"/.prompt-"$item" "$W"/.prompt-"$item"-* "$W"/.prompt-*-"$item" \
    "$B"/*"$item"-[0-9]*.md "$B"/*"$item".md 2>/dev/null | head -1)
  [ -n "$p" ] || return 1
  for f in "$R"/"$item"-*.md "$R"/*-"$item".md "$R"/"$item".md; do
    [ -f "$f" ] && [ "$f" -nt "$p" ] && return 0
  done; return 1
}
while read -r n s q t; do
  [ -n "$n" ] || continue; seen[$n]=$q; state[$n]=$s
  if [ "$s" = blocked ]; then mark "$t" "⛔ "
  elif { [ "$s" = idle ] || [ "$s" = done ]; } && ! user_facing "$n" && fresh_report "$n"; then
    # finished while no watcher ran: its report is for the supervisor, never a question to the user
    mark "$t" ""; pending="$pending $n"
  elif { [ "$s" = idle ] || [ "$s" = done ]; } && m=$(last_msg "$n") && ! finished "$n" "$m" && asks_user "$m"; then mark "$t" "❓ "
  else mark "$t" ""; fi
done < <(snap)
for n in $pending; do echo "NEEDS-SUPERVISOR $n (report written before this watcher started)"; done
[ -z "$pending" ] || exit 0
while sleep 30; do
  declare -A now=()
  while read -r n s q t; do
    [ -n "$n" ] || continue; now[$n]=1
    [ "${seen[$n]:-}" = "$q" ] && continue
    prev=${state[$n]:-working}; seen[$n]=$q; state[$n]=$s
    [ "$s" = working ] && { mark "$t" ""; continue; }
    tail=$(herdr agent read "$n" --source recent-unwrapped --lines 25 2>/dev/null | grep -v '^\s*$' | tail -15)
    if [ "$s" = blocked ]; then
      mark "$t" "⛔ "; notify "$n is blocked" "A permission prompt or question in its tab."
      user_facing "$n" || { echo "BLOCKED $n"; echo "$tail"; exit 0; }
      continue
    fi
    if [ "$prev" != idle ] && [ "$prev" != done ] && { [ "$s" = idle ] || [ "$s" = done ]; }; then
      msg=$(last_msg "$n")
      if ! user_facing "$n" && { finished "$n" "$msg" || grep -qiE 'supervisor|ship|push|pull request|\bPR\b|merge' <<<"$tail" || fresh_report "$n"; }; then
        mark "$t" ""; echo "NEEDS-SUPERVISOR $n"; echo "$tail"; exit 0
      fi
      if asks_user "$msg"; then
        mark "$t" "❓ "; notify "$n is waiting for you" "$(tail -1 <<<"$msg" | cut -c1-120)"
        user_facing "$n" || { echo "WAITING-ON-USER $n"; echo "$tail"; exit 0; }
      fi
    fi
  done < <(snap)
  for n in "${!seen[@]}"; do [ -n "${now[$n]:-}" ] || { echo "GONE $n"; exit 0; }; done
  unset now
done
