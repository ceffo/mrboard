#!/usr/bin/env bash
# land.sh <pr> [tab] : THE supervisor's only merge path. Grant it as an allow rule in the project settings.
# It enforces the review gate itself and refuses (exit 2, prints why) unless ALL hold on the PR's head:
#   - every check has completed and is SUCCESS / SKIPPED / NEUTRAL, at least one check exists;
#   - GitHub says MERGEABLE and CLEAN (required checks reported, not behind or blocked);
#   - when $FLEET_REVIEW_BOT (a login regex, e.g. "greptile|coderabbit") is set: every inline comment thread
#     that bot opened has a reply (handled: fixed, or answered why not). It never waits for the bot to review.
# Then: squash-merge (pinned to the checked head sha), close the dev tab if given, prune worktrees,
# fast-forward the main checkout. Call it as ONE plain command (absolute path, nothing chained) so the
# permission allow rule matches. No tracker step: the PR's "Closes <id>" line closes the issue on merge.
set -euo pipefail
pr="${1:?usage: land.sh <pr> [tab]}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
main=$FLEET_MAIN
repo=$(fleet_repo)
base=$FLEET_DEFAULT_BRANCH
cd "$main"
refuse() { echo "REFUSED #$pr: $*"; exit 2; }
j=$(gh pr view "$pr" -R "$repo" --json state,title,mergeable,mergeStateStatus,headRefOid,statusCheckRollup)
[ "$(jq -r .state <<<"$j")" = OPEN ] || refuse "state $(jq -r .state <<<"$j")"
sha=$(jq -r .headRefOid <<<"$j")
# The squash subject is always the PR title: GitHub would use the commit subject for a one-commit PR,
# and the release level (ADR-0011) is read from that subject.
subject="$(jq -r .title <<<"$j") (#$pr)"
[ "$(jq '.statusCheckRollup|length' <<<"$j")" -gt 0 ] || refuse "no checks reported yet"
pending=$(jq -r '[.statusCheckRollup[]|select((.status // "COMPLETED")!="COMPLETED")|.name]|join(",")' <<<"$j")
[ -z "$pending" ] || refuse "checks pending: $pending"
bad=$(jq -r '[.statusCheckRollup[]|select(((.conclusion // .state)|IN("SUCCESS","SKIPPED","NEUTRAL"))|not)|(.name // .context)]|join(",")' <<<"$j")
[ -z "$bad" ] || refuse "checks not green: $bad"
[ "$(jq -r .mergeable <<<"$j")" = MERGEABLE ] || refuse "mergeable=$(jq -r .mergeable <<<"$j")"
[ "$(jq -r .mergeStateStatus <<<"$j")" = CLEAN ] || refuse "mergeStateStatus=$(jq -r .mergeStateStatus <<<"$j")"
if [ -n "$FLEET_REVIEW_BOT" ]; then
  unanswered=$(gh api --paginate "repos/$repo/pulls/$pr/comments" | jq -s --arg bot "$FLEET_REVIEW_BOT" 'add // [] | . as $c
    | [$c[]|select(.in_reply_to_id==null and (.user.login|test($bot;"i")))|.id as $r
       |select([$c[]|select(.in_reply_to_id==$r)]|length==0)]|length')
  [ "$unanswered" = 0 ] || refuse "$unanswered review-bot comment thread(s) without a reply"
fi
# Pinned to the checked head: if the head moved after the checks, the merge fails and nothing is merged.
if ! err=$(gh pr merge "$pr" -R "$repo" --squash --subject "$subject" --delete-branch --match-head-commit "$sha" 2>&1 >/dev/null); then
  # A PR in a GitHub stack can only merge through the stack merge. Only the stack's bottom PR (based on
  # main), and only if its head is still the checked sha: a higher PR would merge unchecked parents.
  grep -q "part of a stack" <<<"$err" || { echo "MERGE FAILED #$pr (head moved from ${sha:0:7}, or GitHub refused)"; exit 1; }
  h=$(gh pr view "$pr" -R "$repo" --json headRefOid,baseRefName --jq '.headRefOid+" "+.baseRefName')
  [ "$h" = "$sha $base" ] || { echo "MERGE FAILED #$pr (stacked: head/base now $h; only the bottom PR at ${sha:0:7} lands)"; exit 1; }
  gh stack merge "$pr" --yes --squash >/dev/null 2>&1 || { echo "MERGE FAILED #$pr (stack merge refused)"; exit 1; }
fi
git worktree prune
[ -z "${2:-}" ] || herdr tab close "$2" >/dev/null 2>&1 || true
if [ "$(git branch --show-current)" = "$base" ]; then git fetch -q origin && git merge -q --ff-only "origin/$base"; fi
echo "MERGED #$pr (head ${sha:0:7})"
