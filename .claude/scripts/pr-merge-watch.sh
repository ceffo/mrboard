#!/usr/bin/env bash
# pr-merge-watch.sh <pr>... : ONE low-rate poller (every 3 min, 1 API call per PR) for night mode.
# Merges (land.sh) a PR when every check is SUCCESS/SKIPPED/NEUTRAL, it is MERGEABLE and GitHub says CLEAN
# (required checks all reported: right after a push the CI checks aren't registered yet).
# No args: reads PR numbers from supervisor/watch-prs each loop (add a PR: echo N >> watch-prs; no re-arm).
# Exits on the first event: MERGED <n>, RED <n>, CONFLICTING <n>, or ERROR (rate limit etc).
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
W=$FLEET_SUPERVISOR_DIR
cd "$FLEET_MAIN"
while true; do
  if [ $# -gt 0 ]; then prs="$*"; else prs=$(grep -o "[0-9]\+" $W/watch-prs 2>/dev/null | sort -u | tr "\n" " "); fi
  for n in $prs; do
    j=$(gh pr view "$n" --json state,mergeable,mergeStateStatus,statusCheckRollup 2>&1) || { echo "ERROR $n: $(echo "$j" | head -c 200)"; sleep 300; continue; }
    st=$(jq -r .state <<<"$j"); mg=$(jq -r .mergeable <<<"$j")
    [ "$st" != OPEN ] && { echo "$st $n"; exit 0; }
    [ "$mg" = CONFLICTING ] && { echo "CONFLICTING $n"; exit 0; }
    pending=$(jq '[.statusCheckRollup[]|select((.status // "COMPLETED")!="COMPLETED")]|length' <<<"$j")
    bad=$(jq -r '[.statusCheckRollup[]|select((.status // "COMPLETED")=="COMPLETED")|(.conclusion // .state)|select(.!="SUCCESS" and .!="SKIPPED" and .!="NEUTRAL")]|join(",")' <<<"$j")
    total=$(jq '.statusCheckRollup|length' <<<"$j")
    if [ "$pending" = 0 ] && [ "$total" -gt 0 ]; then
      [ -n "$bad" ] && { echo "RED $n ($bad)"; exit 0; }
      [ "$mg" = MERGEABLE ] && [ "$(jq -r .mergeStateStatus <<<"$j")" = CLEAN ] && { ${LAND_SH:-$here/land.sh} "$n" && echo "MERGED $n"; exit 0; }
    fi
  done
  sleep 180
done
