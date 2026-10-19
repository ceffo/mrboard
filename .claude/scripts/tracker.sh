#!/usr/bin/env bash
# tracker.sh: the one tracker interface the agents use, whatever the backend (FLEET_TRACKER=github|linear).
# Every verb prints plain text or TSV; ids are what the backend uses (GitHub "#12", Linear "ABC-12").
#
#   tracker.sh show <id>                       the item in full: title, state, priority, labels, blockers, body, comments
#   tracker.sh list [--state S] [--label L] [--query Q]
#                                              TSV: id, state, priority, title. S = Todo|In Progress|In Review|Backlog|Done|open|all (default open)
#   tracker.sh ready [--json]                  the dispatchable queue: Todo, no open blocker, no needs-info/deferred label, not an epic;
#                                              Urgent first, then oldest. TSV: id, priority, title, parent
#   tracker.sh questions                       JSON lines {id,title,description} of open needs-info / question items
#   tracker.sh labels                          the labels that exist
#   WRITES (supervisor and scoper only; developers, reviewers and testers never call these):
#   tracker.sh create --title T --body-file F [--priority Urgent|High|Medium|Low|None] [--label L]... [--parent ID] [--blocked-by ID]...
#                                              prints the new id
#   tracker.sh update <id> [--title T] [--body-file F] [--priority P] [--add-label L]... [--remove-label L]...
#   tracker.sh block <id> <blocker-id>         <id> cannot start until <blocker-id> is done
#   tracker.sh comment <id> <text>             (text "-" reads stdin)
#   tracker.sh state <id> <State> [comment]    Todo | In Progress | In Review | Backlog | Done
#   tracker.sh setup                           create the process labels the fleet uses (idempotent)
#
# States: Todo (open) -> In Progress (claimed at dispatch) -> In Review (PR open) -> Done (the PR merge closes it).
# Process labels: needs-info, risky, epic, deferred, question, bug, feature, chore, docs.
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
case "$FLEET_TRACKER" in
  github|linear) exec "$here/tracker-$FLEET_TRACKER.sh" "$@" ;;
  *) echo "tracker: unknown FLEET_TRACKER '$FLEET_TRACKER' (github or linear)" >&2; exit 2 ;;
esac
