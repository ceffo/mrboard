#!/usr/bin/env bash
# pr-watch.sh [<pr>...] : report-only PR watcher; never merges (pr-merge-watch.sh does).
# No args: reads PR numbers from supervisor/watch-prs each loop. Polls every 5 min and prints ONE
# line, then exits, on the first event: EVENT #<n> MERGED|CLOSED, CONFLICTING, BEHIND <base> by <k>,
# or CI <rollup> (the check rollup changed since the previous poll).
# A failed gh call is retried twice (30 s apart) before it is reported as an ERROR event.
# PR_WATCH_ONCE=1 polls once and exits 0 without an event (dry run).
case "${1:-}" in -h|--help) sed -n 2,7p "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0;; esac
set -u
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
W=$FLEET_SUPERVISOR_DIR
repo=$FLEET_MAIN
declare -A last
while :; do
  git -C "$repo" fetch -q origin
  if [ $# -gt 0 ]; then prs="$*"; else prs=$(grep -o "[0-9]\+" "$W/watch-prs" 2>/dev/null | sort -u | tr "\n" " "); fi
  for pr in $prs; do
    j=""
    for try in 1 2 3; do
      j=$(cd "$repo" && gh pr view "$pr" --json state,mergeable,headRefName,baseRefName,statusCheckRollup) && break
      j=""; [ "$try" -lt 3 ] && sleep 30
    done
    [ -n "$j" ] || { echo "EVENT #$pr ERROR gh pr view failed 3 times"; exit 0; }
    st=$(jq -r .state <<<"$j"); mg=$(jq -r .mergeable <<<"$j")
    h=$(jq -r .headRefName <<<"$j"); b=$(jq -r .baseRefName <<<"$j")
    ci=$(jq -r '[.statusCheckRollup[]?|(.conclusion // .status)]|join(",")' <<<"$j")
    [ "$st" != OPEN ] && { echo "EVENT #$pr $st"; exit 0; }
    [ "$mg" = CONFLICTING ] && { echo "EVENT #$pr CONFLICTING"; exit 0; }
    n=$(git -C "$repo" rev-list --count "origin/$h..origin/$b" 2>/dev/null || echo 0)
    [ "$n" -gt 0 ] && { echo "EVENT #$pr BEHIND $b by $n"; exit 0; }
    if [ -n "${last[$pr]+x}" ] && [ "${last[$pr]}" != "$ci" ]; then echo "EVENT #$pr CI $ci"; exit 0; fi
    last[$pr]=$ci
  done
  [ -z "${PR_WATCH_ONCE:-}" ] || exit 0
  sleep 300
done
