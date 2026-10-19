#!/usr/bin/env bash
# ci-state.sh: the state of every CI the supervisor owns, in one call.
#   - the default branch: the latest run of each workflow in $FLEET_CI_WORKFLOWS (default "CI"), and, when
#     $FLEET_RELEASE_WORKFLOW is set, the run of it that CI's success on the branch's head must trigger;
#   - every open PR (ours and Dependabot's): its checks.
# Prints one line per item that is not green: "PENDING <what>" or "RED <what> <url>".
# A cancelled run counts as superseded (concurrency cancels a run when a newer push arrives).
# Exit 0: all green. 10: something pending, nothing red. 20: something red.
# A red item listed (its run id or "pr<n>@<sha7>") in supervisor/ci-red-handled is reported as
# "HANDLED ..." and doesn't count: append it there once you've acted on it (rework brief, rerun).
set -uo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
cd "$FLEET_MAIN"
W=$FLEET_SUPERVISOR_DIR
B=$FLEET_DEFAULT_BRANCH
handled="$W/ci-red-handled"
pending=0; red=0
is_handled() { [ -f "$handled" ] && grep -qF -- "$1" "$handled"; }
flag() { # flag <kind PENDING|RED> <key> <text>
  if [ "$1" = RED ]; then
    if is_handled "$2"; then echo "HANDLED $3"; else echo "RED $3"; red=1; fi
  else echo "PENDING $3"; pending=1; fi
}

runs=$(gh run list --branch "$B" --limit 30 --json databaseId,workflowName,status,conclusion,headSha,event,url 2>&1) \
  || { echo "ERROR gh run list: $(head -c 200 <<<"$runs")"; exit 20; }
head_sha=$(gh api "repos/{owner}/{repo}/commits/$B" --jq .sha 2>/dev/null)
[[ "$head_sha" =~ ^[0-9a-f]{40}$ ]] || head_sha=""   # no commit yet, or the API failed
for wf in $FLEET_CI_WORKFLOWS $FLEET_RELEASE_WORKFLOW; do
  r=$(jq -c --arg w "$wf" '[.[]|select(.workflowName==$w and .event!="pull_request")][0] // empty' <<<"$runs")
  [ -n "$r" ] || continue
  id=$(jq -r .databaseId <<<"$r"); st=$(jq -r .status <<<"$r"); c=$(jq -r .conclusion <<<"$r"); url=$(jq -r .url <<<"$r")
  if [ "$st" != completed ]; then flag PENDING "$id" "$B $wf run $id ($st)"
  elif [ "$c" != success ] && [ "$c" != skipped ] && [ "$c" != neutral ] && [ "$c" != cancelled ]; then flag RED "$id" "$B $wf run $id ($c) $url"; fi
done
ci_wf=${FLEET_CI_WORKFLOWS%% *}
ci_head=$(jq -r --arg w "$ci_wf" --arg s "$head_sha" '[.[]|select(.workflowName==$w and .headSha==$s and (.event=="push" or .event=="workflow_dispatch"))][0].conclusion // empty' <<<"$runs")
if [ -n "$FLEET_RELEASE_WORKFLOW" ]; then  # CI green on the head but its release run not started yet (workflow_run gap)
  rel_head=$(jq -r --arg w "$FLEET_RELEASE_WORKFLOW" --arg s "$head_sha" '[.[]|select(.workflowName==$w and .headSha==$s)]|length' <<<"$runs")
  [ "$ci_head" = success ] && [ "$rel_head" = 0 ] && flag PENDING "release-$head_sha" "$B $FLEET_RELEASE_WORKFLOW for ${head_sha:0:7} not started yet"
fi
[ -z "$ci_head" ] && [ -n "$head_sha" ] && ! jq -e --arg w "$ci_wf" --arg s "$head_sha" 'any(.[];.workflowName==$w and .headSha==$s)' <<<"$runs" >/dev/null \
  && flag PENDING "ci-$head_sha" "$B $ci_wf for ${head_sha:0:7} not started yet"

prs=$(gh pr list --state open --json number,headRefOid,author,statusCheckRollup 2>&1) \
  || { echo "ERROR gh pr list: $(head -c 200 <<<"$prs")"; exit 20; }
while IFS= read -r p; do
  [ -n "$p" ] || continue
  n=$(jq -r .number <<<"$p"); sha=$(jq -r '.headRefOid[0:7]' <<<"$p")
  who=""; [ "$(jq -r .author.login <<<"$p")" = app/dependabot ] && who=" (dependabot)"
  np=$(jq '[.statusCheckRollup[]|select((.status // "COMPLETED")!="COMPLETED")]|length' <<<"$p")
  bad=$(jq -r '[.statusCheckRollup[]|select((.status // "COMPLETED")=="COMPLETED")|select(((.conclusion // .state)|IN("SUCCESS","SKIPPED","NEUTRAL","CANCELLED"))|not)|.name // .context]|join(",")' <<<"$p")
  [ -n "$bad" ] && flag RED "pr$n@$sha" "PR #$n$who @$sha failing: $bad"
  [ "$np" -gt 0 ] && [ -z "$bad" ] && flag PENDING "pr$n" "PR #$n$who @$sha: $np check(s) running"
done < <(jq -c '.[]' <<<"$prs")

[ "$red" = 1 ] && exit 20
[ "$pending" = 1 ] && exit 10
exit 0
