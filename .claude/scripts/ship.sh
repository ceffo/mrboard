#!/usr/bin/env bash
# ship.sh [--base <branch>] <worktree> <title> <body-file> <id>... : push the branch, open the PR, move each tracker issue to
# "In Review" with a comment holding the PR link, print "PR <n> <mergeable>". Run from anywhere.
# It never closes an issue: the PR body's "Closes <id>" line does that when the PR merges.
# A PR closed unmerged goes back with: .claude/scripts/tracker.sh state <id> Todo "PR #n closed unmerged"
# If the tracker fails for an issue the PR stays open, the failure is on stderr and the exit status is 1.
set -euo pipefail
base=()
if [ "${1:-}" = --base ]; then base=(--base "$2"); shift 2; fi
wt=$1 title=$2 body=$3; shift 3
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
main=$FLEET_MAIN
br=$(git -C "$wt" branch --show-current)
git -C "$wt" push -q -u origin "$br" 2>&1 | grep -v '^remote' || true
url=$(cd "$main" && gh pr create "${base[@]}" --head "$br" --title "$title" --body-file "$body" | tail -1)
n=${url##*/}
rc=0
for id in "$@"; do
    "$here/tracker.sh" state "$id" "In Review" "PR $url" >/dev/null || { echo "ship: $id not moved to In Review" >&2; rc=1; }
done
sleep "${SHIP_SETTLE:-5}"; echo "PR $n $(cd "$main" && gh pr view "$n" --json mergeable -q .mergeable)"
exit $rc
