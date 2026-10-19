#!/usr/bin/env bash
# tracker-github.sh: the GitHub Issues backend of tracker.sh (see tracker.sh for the interface).
# How the fleet's model maps onto GitHub:
#   Todo         open, no status label          In Progress  label status:in-progress
#   In Review    label status:in-review         Backlog      label status:backlog
#   Done         closed (the PR's "Closes #n" does it on merge)
#   priority     label priority:urgent|high|medium|low (no label = None)
#   type/process labels  needs-info, risky, epic, deferred, question, bug, feature, chore, docs
#   blocked by   GitHub's native issue dependencies;  parent  GitHub's native sub-issues
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
repo="$(fleet_repo)"
GH() { gh "$@" -R "$repo"; }

PROCESS_LABELS=(needs-info risky epic deferred question bug feature chore docs)
STATUS_LABELS=(status:in-progress status:in-review status:backlog)
PRIORITY_LABELS=(priority:urgent priority:high priority:medium priority:low)

die() { echo "tracker(github): $*" >&2; exit 1; }
num() { local n="${1#\#}"; n="${n#GH-}"; [[ "$n" =~ ^[0-9]+$ ]] || die "not an issue id: $1"; echo "$n"; }
api() { gh api "repos/$repo/$1" "${@:2}"; }
prio_label() { case "$(tr 'A-Z' 'a-z' <<<"$1")" in urgent) echo priority:urgent;; high) echo priority:high;; medium) echo priority:medium;;
  low) echo priority:low;; none|"") echo "";; *) die "unknown priority '$1' (Urgent|High|Medium|Low|None)";; esac; }
state_label() { case "$1" in "Todo") echo "";; "In Progress") echo status:in-progress;; "In Review") echo status:in-review;;
  "Backlog") echo status:backlog;; "Done") echo "";; *) die "unknown state '$1' (Todo|In Progress|In Review|Backlog|Done)";; esac; }

# jq prelude: the fleet's view of one issue as returned by `gh issue list/view --json`.
JQ_VIEW='
def lbls: [.labels[].name];
def prio: (lbls | map(select(startswith("priority:"))) | .[0] // "" | sub("priority:";"") | if .=="" then "None" else (.[0:1]|ascii_upcase)+.[1:] end);
def st: if .state=="CLOSED" or .state=="closed" then "Done"
        elif (lbls|index("status:in-progress")) then "In Progress"
        elif (lbls|index("status:in-review")) then "In Review"
        elif (lbls|index("status:backlog")) then "Backlog"
        else "Todo" end;'

cmd_show() {
  local n; n="$(num "${1:?usage: show <id>}")"
  GH issue view "$n" --json number,title,state,labels,body,comments,url | jq -r "$JQ_VIEW"'
    "#\(.number) \(.title)\nState: \(st)   Priority: \(prio)\nLabels: \(lbls|join(", "))\n\(.url)\n\n\(.body)\n"
    + (if (.comments|length)>0 then "\n--- comments ---\n" + ([.comments[]|"[\(.author.login), \(.createdAt)]\n\(.body)"]|join("\n\n")) else "" end)'
  local b; b="$(api "issues/$n/dependencies/blocked_by" --jq '[.[]|select(.state=="open")|"#"+(.number|tostring)]|join(", ")' 2>/dev/null || true)"
  [ -z "$b" ] || echo "Blocked by (open): $b"
}

cmd_list() {
  local state=open label="" query=""
  while [ $# -gt 0 ]; do case "$1" in
    --state) state="$2"; shift 2;; --label) label="$2"; shift 2;; --query) query="$2"; shift 2;;
    *) die "list: unknown option $1";; esac; done
  local args=(--limit 500 --json number,title,state,labels)
  case "$state" in all|Done) args+=(--state all);; *) args+=(--state open);; esac
  [ -z "$label" ] || args+=(--label "$label")
  [ -z "$query" ] || args+=(--search "$query")
  GH issue list "${args[@]}" | jq -r --arg want "$state" "$JQ_VIEW"'
    .[] | select($want=="open" and st!="Done" or $want=="all" or st==$want)
    | ["#"+(.number|tostring), st, prio, .title] | @tsv'
}

cmd_ready() {
  local json=0; [ "${1:-}" != --json ] || json=1
  local rows ready=""
  rows="$(GH issue list --state open --limit 500 --json number,title,labels,createdAt)"
  local cand
  cand="$(jq -c "$JQ_VIEW"'[.[] | select(st=="Todo")
      | select((lbls|any(.=="needs-info" or .=="deferred" or .=="epic"))|not)
      | {n: .number, id: ("#"+(.number|tostring)), priority: prio, title: .title, created: .createdAt}]' <<<"$rows")"
  local out='[]' n
  while IFS= read -r n; do
    [ -n "$n" ] || continue
    local open parent
    open="$(api "issues/$n/dependencies/blocked_by" --jq '[.[]|select(.state=="open")]|length' 2>/dev/null || echo 0)"
    [ "$open" = 0 ] || continue
    parent="$(api "issues/$n/parent" --jq '"#"+(.number|tostring)' 2>/dev/null || true)"
    out="$(jq -c --argjson n "$n" --arg p "$parent" --argjson c "$cand" \
      '. + [$c[]|select(.n==$n)|{id,priority,title,created,parent:(if $p=="" then null else $p end)}]' <<<"$out")"
  done < <(jq -r '.[].n' <<<"$cand")
  out="$(jq -c 'sort_by([({"Urgent":1,"High":2,"Medium":3,"Low":4,"None":5}[.priority]), .created])' <<<"$out")"
  if [ "$json" = 1 ]; then echo "$out"
  else jq -r '.[]|[.id,.priority,.title,(.parent // "")]|@tsv' <<<"$out"; fi
}

cmd_questions() {
  GH issue list --state open --limit 500 --json number,title,body,labels \
    | jq -c '.[] | select(.labels|any(.name=="needs-info" or .name=="question"))
             | {id:("#"+(.number|tostring)), title, description:(.body // "")}'
}

cmd_labels() { GH label list --limit 200 --json name --jq '.[].name'; }

ensure_label() { # ensure_label <name> : create it when the repo has none of that name
  # grep reads the whole list: an early `grep -q` exit would SIGPIPE gh and fail the pipeline under pipefail.
  GH label list --limit 300 --json name --jq '.[].name' | grep -xF "$1" >/dev/null ||
    GH label create "$1" --color "${2:-ededed}" >/dev/null
}

cmd_create() {
  local title="" body="" prio="" parent="" labels=() blockers=()
  while [ $# -gt 0 ]; do case "$1" in
    --title) title="$2"; shift 2;; --body-file) body="$2"; shift 2;; --priority) prio="$2"; shift 2;;
    --label) labels+=("$2"); shift 2;; --parent) parent="$2"; shift 2;; --blocked-by) blockers+=("$2"); shift 2;;
    *) die "create: unknown option $1";; esac; done
  [ -n "$title" ] || die "create: --title is required"
  local pl; pl="$(prio_label "${prio:-None}")"
  [ -z "$pl" ] || labels+=("$pl")
  local l args=(--title "$title" --body-file "${body:-/dev/null}")
  for l in "${labels[@]+"${labels[@]}"}"; do ensure_label "$l"; args+=(--label "$l"); done
  local url n; url="$(GH issue create "${args[@]}" | tail -1)"; n="${url##*/}"
  [ -z "$parent" ] || set_parent "$n" "$parent"
  for l in "${blockers[@]+"${blockers[@]}"}"; do cmd_block "$n" "$l"; done
  echo "#$n"
}

set_parent() { # set_parent <child> <parent>
  local cid; cid="$(api "issues/$(num "$1")" --jq .id)"
  api "issues/$(num "$2")/sub_issues" -X POST -F sub_issue_id="$cid" >/dev/null || die "cannot make $1 a sub-issue of $2"
}

cmd_block() { # block <id> <blocker>: id is blocked by blocker
  local bid; bid="$(api "issues/$(num "${2:?usage: block <id> <blocker-id>}")" --jq .id)"
  api "issues/$(num "${1:?usage: block <id> <blocker-id>}")/dependencies/blocked_by" -X POST -F issue_id="$bid" >/dev/null \
    || die "cannot record $1 blocked by $2"
}

cmd_update() {
  local n; n="$(num "${1:?usage: update <id> …}")"; shift
  local args=() prio="" add=() rm=()
  while [ $# -gt 0 ]; do case "$1" in
    --title) args+=(--title "$2"); shift 2;; --body-file) args+=(--body-file "$2"); shift 2;;
    --priority) prio="$2"; shift 2;; --add-label) add+=("$2"); shift 2;; --remove-label) rm+=("$2"); shift 2;;
    *) die "update: unknown option $1";; esac; done
  if [ -n "$prio" ]; then
    local pl; pl="$(prio_label "$prio")"
    local p; for p in "${PRIORITY_LABELS[@]}"; do [ "$p" = "$pl" ] || rm+=("$p"); done
    [ -z "$pl" ] || add+=("$pl")
  fi
  local l; for l in "${add[@]+"${add[@]}"}"; do ensure_label "$l"; args+=(--add-label "$l"); done
  local have; have="$(GH issue view "$n" --json labels --jq '.labels[].name')"
  for l in "${rm[@]+"${rm[@]}"}"; do grep -qxF "$l" <<<"$have" && args+=(--remove-label "$l"); done
  [ ${#args[@]} -eq 0 ] || GH issue edit "$n" "${args[@]}" >/dev/null
}

cmd_comment() {
  local n; n="$(num "${1:?usage: comment <id> <text>}")"; local t="${2:?usage: comment <id> <text>}"
  if [ "$t" = - ]; then GH issue comment "$n" --body-file - >/dev/null; else GH issue comment "$n" --body "$t" >/dev/null; fi
}

cmd_state() {
  local n; n="$(num "${1:?usage: state <id> <State> [comment]}")"; local st="${2:?usage: state <id> <State> [comment]}" c="${3:-}"
  local want; want="$(state_label "$st")"
  local have; have="$(GH issue view "$n" --json labels,state --jq '([.labels[].name]|join("\n")), "STATE="+.state')"
  local args=() l
  for l in "${STATUS_LABELS[@]}"; do [ "$l" = "$want" ] || { grep -qxF "$l" <<<"$have" && args+=(--remove-label "$l"); }; done
  if [ -n "$want" ] && ! grep -qxF "$want" <<<"$have"; then ensure_label "$want"; args+=(--add-label "$want"); fi
  [ ${#args[@]} -eq 0 ] || GH issue edit "$n" "${args[@]}" >/dev/null
  if [ "$st" = Done ]; then grep -q 'STATE=CLOSED' <<<"$have" || GH issue close "$n" >/dev/null 2>&1
  else grep -q 'STATE=CLOSED' <<<"$have" && GH issue reopen "$n" >/dev/null 2>&1 || true; fi
  [ -z "$c" ] || GH issue comment "$n" --body "$c" >/dev/null
  echo "#$n -> $st"
}

cmd_setup() {
  local l
  for l in "${PROCESS_LABELS[@]}" "${STATUS_LABELS[@]}" "${PRIORITY_LABELS[@]}"; do ensure_label "$l"; done
  echo "labels ready in $repo"
}

verb="${1:-}"; [ $# -eq 0 ] || shift
case "$verb" in
  show) cmd_show "$@";; list) cmd_list "$@";; ready) cmd_ready "$@";; questions) cmd_questions;; labels) cmd_labels;;
  create) cmd_create "$@";; update) cmd_update "$@";; block) cmd_block "$@";; comment) cmd_comment "$@";;
  state) cmd_state "$@";; setup) cmd_setup;;
  *) sed -n '2,24p' "$here/tracker.sh" >&2; exit 2;;
esac
