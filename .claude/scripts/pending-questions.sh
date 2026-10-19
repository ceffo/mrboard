#!/usr/bin/env bash
# pending-questions.sh: every question waiting on the user, from the two places they live:
#   1. the tracker: open items labelled needs-info, and open items of type question;
#   2. the ledger's "## Pending questions to user" section (questions not tied to an item).
# The supervisor runs it before every reply that ends with the pending-questions list, and
# restates each line in plain words with its default. "No questions pending" only on exit 0
# with nothing printed. Exit 3 when there is at least one question.
# The tracker part comes from tracker.sh questions (the "Questions" section is read from the issue body). A missing
# key or an API error is a message on stderr and exit 1, never exit 3 and never a silent "no questions".
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
ledger="${FLEET_LEDGER:-$FLEET_SUPERVISOR_DIR/ledger.md}"
n=0
# the item's own questions: lines of its "Questions" / "Open questions" section (stdin: the item's text)
questions() { awk '/^## .*[Qq]uestions/{f=1;next} /^## |^[A-Z][A-Za-z ]*:$/{f=0} f&&NF' | head -6; }
resp="$("$here/tracker.sh" questions)" || exit 1
while IFS= read -r row; do
  [ -n "$row" ] || continue
  n=$((n + 1))
  id="$(jq -r .id <<<"$row")"; title="$(jq -r .title <<<"$row")"
  q="$(jq -r '.description // ""' <<<"$row" | questions)"
  printf '%d. [%s] %s\n%s\n' "$n" "$id" "$title" "${q:-   (no Questions section: write one with a default, then ask)}"
done <<<"$resp"
if [ -f "$ledger" ]; then
  while IFS= read -r line; do
    n=$((n + 1)); printf '%d. [ledger] %s\n' "$n" "$line"
  done < <(awk '/^## Pending questions to user/{f=1;next} /^## /{f=0} f' "$ledger" \
           | grep -E '^[0-9]+\.' || true)
fi
[ "$n" -eq 0 ] || exit 3
