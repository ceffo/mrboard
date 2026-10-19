#!/usr/bin/env bash
# tracker-linear.sh: the Linear backend of tracker.sh (see tracker.sh for the interface).
# Needs LINEAR_TEAM (the team key, e.g. ENG) and an API key (see linear-api.sh). Uses the team's
# built-in states Todo / In Progress / In Review / Backlog / Done and labels by name. Linear's GitHub
# integration moves an issue to Done when a PR whose body has "Closes <KEY-n>" merges.
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
team="${LINEAR_TEAM:-}"
[ -n "$team" ] || { echo "tracker(linear): set LINEAR_TEAM (the team key) in .claude/fleet.env" >&2; exit 1; }
Q() { "$here/linear-api.sh" "$@"; }
die() { echo "tracker(linear): $*" >&2; exit 1; }
vars() { jq -nc "$@"; }

PROCESS_LABELS=(needs-info risky epic deferred question bug feature chore docs)

# priority name <-> Linear number
prio_num() { case "$(tr 'A-Z' 'a-z' <<<"$1")" in urgent) echo 1;; high) echo 2;; medium) echo 3;; low) echo 4;; none|"") echo 0;;
  *) die "unknown priority '$1' (Urgent|High|Medium|Low|None)";; esac; }
JQ_PRIO='def pname: {"0":"None","1":"Urgent","2":"High","3":"Medium","4":"Low"}[tostring];'

issue_uuid() { # issue_uuid <KEY-n>
  local u; u="$(Q 'query($id: String!) { issue(id: $id) { id } }' "$(vars --arg id "$1" '{id:$id}')" | jq -r '.data.issue.id // empty')"
  [ -n "$u" ] || die "no such issue $1"; echo "$u"
}
team_id() { Q 'query($k: String!) { teams(filter: {key: {eq: $k}}) { nodes { id } } }' "$(vars --arg k "$team" '{k:$k}')" \
  | jq -r '.data.teams.nodes[0].id // empty'; }
label_ids() { # label_ids <name>... -> JSON array of label ids (creates none; unknown label is an error)
  local all; all="$(Q 'query($k: String!) { issueLabels(first: 250, filter: {or: [{team: {key: {eq: $k}}}, {team: {null: true}}]}) { nodes { id name } } }' \
    "$(vars --arg k "$team" '{k:$k}')")"
  local out='[]' n id
  for n in "$@"; do
    id="$(jq -r --arg n "$n" '[.data.issueLabels.nodes[]|select(.name==$n)|.id][0] // empty' <<<"$all")"
    [ -n "$id" ] || die "no label '$n' in team $team (run: tracker.sh setup)"
    out="$(jq -c --arg i "$id" '. + [$i]' <<<"$out")"
  done; echo "$out"
}

cmd_show() {
  local id="${1:?usage: show <id>}"
  Q 'query($id: String!) { issue(id: $id) { identifier title description url priority state { name } parent { identifier }
      labels { nodes { name } } inverseRelations { nodes { type issue { identifier state { type } } } }
      comments { nodes { body createdAt user { name } } } } }' "$(vars --arg id "$id" '{id:$id}')" \
  | jq -r "$JQ_PRIO"'.data.issue // error("no such issue")
    | "\(.identifier) \(.title)\nState: \(.state.name)   Priority: \(.priority|pname)\nLabels: \([.labels.nodes[].name]|join(", "))\(.parent|if . then "\nParent: \(.identifier)" else "" end)\n\(.url)\n\n\(.description // "")\n"
      + ([.inverseRelations.nodes[]|select(.type=="blocks" and (.issue.state.type|IN("completed","canceled")|not))|.issue.identifier]|if length>0 then "Blocked by (open): \(join(", "))\n" else "" end)
      + (if (.comments.nodes|length)>0 then "\n--- comments ---\n" + ([.comments.nodes[]|"[\(.user.name // "?"), \(.createdAt)]\n\(.body)"]|join("\n\n")) else "" end)'
}

cmd_list() {
  local state=open label="" query=""
  while [ $# -gt 0 ]; do case "$1" in
    --state) state="$2"; shift 2;; --label) label="$2"; shift 2;; --query) query="$2"; shift 2;;
    *) die "list: unknown option $1";; esac; done
  local resp
  resp="$(Q 'query($t: String!, $q: String) { issues(first: 250, filter: {team: {key: {eq: $t}}, searchableContent: {contains: $q}}) {
      nodes { identifier title priority state { name type } labels { nodes { name } } } } }' \
    "$(vars --arg t "$team" --arg q "$query" '{t:$t, q:(if $q=="" then null else $q end)}')")" || exit 1
  jq -r "$JQ_PRIO"' --arg want "'"$state"'" --arg lab "'"$label"'" '"'"'
    .data.issues.nodes[]
    | select($lab=="" or ([.labels.nodes[].name]|index($lab)))
    | select($want=="all" or ($want=="open" and (.state.type|IN("completed","canceled")|not)) or .state.name==$want)
    | [.identifier, .state.name, (.priority|pname), .title] | @tsv'"'" <<<"$resp"
}

cmd_ready() {
  local json=0; [ "${1:-}" != --json ] || json=1
  local query='query($team: String!, $after: String) {
    issues(first: 100, after: $after, filter: {team: {key: {eq: $team}}, state: {name: {eq: "Todo"}}}) {
      nodes { identifier title priority createdAt parent { identifier } labels { nodes { name } }
              inverseRelations { nodes { type issue { identifier state { type } } } } }
      pageInfo { hasNextPage endCursor } } }'
  local nodes='[]' after=null page
  while :; do
    page="$(Q "$query" "$(vars --arg t "$team" --argjson a "$after" '{team:$t, after:$a}')")" || exit 1
    nodes="$(jq -c --argjson more "$(jq -c '.data.issues.nodes' <<<"$page")" '. + $more' <<<"$nodes")"
    [ "$(jq -r '.data.issues.pageInfo.hasNextPage' <<<"$page")" = true ] || break
    after="$(jq -c '.data.issues.pageInfo.endCursor' <<<"$page")"
  done
  # Linear priority: 0 none, 1 urgent .. 4 low; none sorts last. A blocker is an inverse relation of type
  # "blocks" whose issue is not completed or canceled.
  local ready
  ready="$(jq -c "$JQ_PRIO"'
    [ .[] | select(([.labels.nodes[].name]|any(. == "needs-info" or . == "deferred" or . == "epic"))|not)
      | select([.inverseRelations.nodes[]|select(.type=="blocks")|select(.issue.state.type|IN("completed","canceled")|not)]|length==0)
      | {id: .identifier, priority: (.priority|pname), title: .title, parent: (.parent.identifier // null), created: .createdAt, p: .priority} ]
    | sort_by([(if .p==0 then 5 else .p end), .created]) | map(del(.p))' <<<"$nodes")"
  if [ "$json" = 1 ]; then echo "$ready"; else jq -r '.[]|[.id,.priority,.title,(.parent // "")]|@tsv' <<<"$ready"; fi
}

cmd_questions() {
  Q 'query($team: String!) { issues(first: 250, filter: {team: {key: {eq: $team}}, state: {type: {nin: ["completed", "canceled"]}},
       or: [{labels: {name: {eq: "needs-info"}}}, {labels: {name: {eq: "question"}}}]}) { nodes { identifier title description } } }' \
    "$(vars --arg t "$team" '{team:$t}')" | jq -c '.data.issues.nodes[]|{id:.identifier, title, description:(.description // "")}'
}

cmd_labels() {
  Q 'query($k: String!) { issueLabels(first: 250, filter: {team: {key: {eq: $k}}}) { nodes { name } } }' "$(vars --arg k "$team" '{k:$k}')" \
    | jq -r '.data.issueLabels.nodes[].name'
}

cmd_create() {
  local title="" body="" prio="" parent="" labels=() blockers=()
  while [ $# -gt 0 ]; do case "$1" in
    --title) title="$2"; shift 2;; --body-file) body="$2"; shift 2;; --priority) prio="$2"; shift 2;;
    --label) labels+=("$2"); shift 2;; --parent) parent="$2"; shift 2;; --blocked-by) blockers+=("$2"); shift 2;;
    *) die "create: unknown option $1";; esac; done
  [ -n "$title" ] || die "create: --title is required"
  local tid; tid="$(team_id)"; [ -n "$tid" ] || die "no team with key $team"
  local lids='[]'; [ ${#labels[@]} -eq 0 ] || lids="$(label_ids "${labels[@]}")"
  local pid=null
  if [ -n "${LINEAR_PROJECT:-}" ]; then
    pid="$(Q 'query($n: String!) { projects(filter: {name: {eq: $n}}) { nodes { id } } }' "$(vars --arg n "$LINEAR_PROJECT" '{n:$n}')" \
      | jq -c '.data.projects.nodes[0].id // null')"
  fi
  local parent_uuid=null; [ -z "$parent" ] || parent_uuid="$(jq -Rc . <<<"$(issue_uuid "$parent")")"
  local input
  input="$(jq -nc --arg t "$tid" --arg title "$title" --arg d "$( [ -z "$body" ] || cat "$body")" --argjson p "$(prio_num "${prio:-None}")" \
    --argjson l "$lids" --argjson pj "$pid" --argjson par "$parent_uuid" \
    '{teamId:$t, title:$title, description:$d, priority:$p, labelIds:$l, projectId:$pj, parentId:$par}')"
  local resp id
  resp="$(Q 'mutation($i: IssueCreateInput!) { issueCreate(input: $i) { success issue { identifier } } }' "$(jq -nc --argjson i "$input" '{i:$i}')")" || exit 1
  id="$(jq -r '.data.issueCreate.issue.identifier // empty' <<<"$resp")"; [ -n "$id" ] || die "create refused"
  local b; for b in "${blockers[@]+"${blockers[@]}"}"; do cmd_block "$id" "$b"; done
  echo "$id"
}

cmd_update() {
  local id="${1:?usage: update <id> …}"; shift
  local title="" body="" prio="" add=() rm=()
  while [ $# -gt 0 ]; do case "$1" in
    --title) title="$2"; shift 2;; --body-file) body="$2"; shift 2;; --priority) prio="$2"; shift 2;;
    --add-label) add+=("$2"); shift 2;; --remove-label) rm+=("$2"); shift 2;; *) die "update: unknown option $1";; esac; done
  local uuid; uuid="$(issue_uuid "$id")"
  local input='{}'
  [ -z "$title" ] || input="$(jq -c --arg v "$title" '.title=$v' <<<"$input")"
  [ -z "$body" ] || input="$(jq -c --arg v "$(cat "$body")" '.description=$v' <<<"$input")"
  [ -z "$prio" ] || input="$(jq -c --argjson v "$(prio_num "$prio")" '.priority=$v' <<<"$input")"
  if [ ${#add[@]} -gt 0 ] || [ ${#rm[@]} -gt 0 ]; then
    local cur names
    cur="$(Q 'query($id: String!) { issue(id: $id) { labels { nodes { id name } } } }' "$(vars --arg id "$id" '{id:$id}')" | jq -c '.data.issue.labels.nodes')"
    names="$(jq -c --argjson add "$(printf '%s\n' "${add[@]+"${add[@]}"}" | jq -R . | jq -sc 'map(select(.!=""))')" \
      --argjson rm "$(printf '%s\n' "${rm[@]+"${rm[@]}"}" | jq -R . | jq -sc 'map(select(.!=""))')" \
      '[.[].name] + $add | unique | map(select(. as $n | $rm | index($n) | not))' <<<"$cur")"
    local final=(); mapfile -t final < <(jq -r '.[]' <<<"$names")
    input="$(jq -c --argjson ids "$(label_ids "${final[@]+"${final[@]}"}")" '.labelIds=$ids' <<<"$input")"
  fi
  [ "$input" != '{}' ] || return 0
  Q 'mutation($id: String!, $i: IssueUpdateInput!) { issueUpdate(id: $id, input: $i) { success } }' \
    "$(jq -nc --arg id "$uuid" --argjson i "$input" '{id:$id, i:$i}')" >/dev/null
}

cmd_block() { # block <id> <blocker>
  local id="${1:?usage: block <id> <blocker-id>}" by="${2:?usage: block <id> <blocker-id>}"
  Q 'mutation($a: String!, $b: String!) { issueRelationCreate(input: {issueId: $a, relatedIssueId: $b, type: blocks}) { success } }' \
    "$(vars --arg a "$(issue_uuid "$by")" --arg b "$(issue_uuid "$id")" '{a:$a, b:$b}')" >/dev/null
}

cmd_comment() {
  local id="${1:?usage: comment <id> <text>}" t="${2:?usage: comment <id> <text>}"
  [ "$t" != - ] || t="$(cat)"
  Q 'mutation($id: String!, $b: String!) { commentCreate(input: {issueId: $id, body: $b}) { success } }' \
    "$(vars --arg id "$(issue_uuid "$id")" --arg b "$t" '{id:$id, b:$b}')" >/dev/null
}

cmd_state() {
  [ $# -ge 2 ] || die "usage: state <id> <State> [comment]"
  local id=$1 state=$2 comment=${3:-}
  local resp uuid sid out
  resp="$(Q 'query($id: String!) { issue(id: $id) { id identifier team { states { nodes { id name } } } } }' "$(vars --arg id "$id" '{id:$id}')")" || exit 1
  uuid="$(jq -r '.data.issue.id // empty' <<<"$resp")"; [ -n "$uuid" ] || die "no such issue $id"
  sid="$(jq -r --arg s "$state" '[.data.issue.team.states.nodes[]|select(.name==$s)|.id][0] // empty' <<<"$resp")"
  [ -n "$sid" ] || die "$id's team has no state \"$state\""
  out="$(Q 'mutation($id: String!, $s: String!) { issueUpdate(id: $id, input: {stateId: $s}) { success } }' "$(vars --arg id "$uuid" --arg s "$sid" '{id:$id, s:$s}')")" || exit 1
  [ "$(jq -r '.data.issueUpdate.success' <<<"$out")" = true ] || die "update of $id refused"
  [ -z "$comment" ] || cmd_comment "$id" "$comment"
  echo "$id -> $state"
}

cmd_setup() {
  local tid have l; tid="$(team_id)"; [ -n "$tid" ] || die "no team with key $team"
  have="$(cmd_labels)"
  for l in "${PROCESS_LABELS[@]}"; do
    grep -qxF "$l" <<<"$have" && continue
    Q 'mutation($t: String!, $n: String!) { issueLabelCreate(input: {teamId: $t, name: $n}) { success } }' "$(vars --arg t "$tid" --arg n "$l" '{t:$t, n:$n}')" >/dev/null
    echo "created label $l"
  done
  echo "labels ready in team $team"
}

verb="${1:-}"; [ $# -eq 0 ] || shift
case "$verb" in
  show) cmd_show "$@";; list) cmd_list "$@";; ready) cmd_ready "$@";; questions) cmd_questions;; labels) cmd_labels;;
  create) cmd_create "$@";; update) cmd_update "$@";; block) cmd_block "$@";; comment) cmd_comment "$@";;
  state) cmd_state "$@";; setup) cmd_setup;;
  *) sed -n '2,24p' "$here/tracker.sh" >&2; exit 2;;
esac
