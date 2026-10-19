# Issue tracker: GitHub Issues

Issues for this repo live in GitHub Issues on `ceffo/mrboard`, accessed through
`.claude/scripts/tracker.sh` (backend `github`, set in `.claude/fleet.env`). Run it without
arguments for the full usage. Ids are `#n`; the file-safe key for branches, briefs and reports is
`GH-n`. `.beads/` is the retired beads tracker, kept for history only: never run `br` or `bv`.

States, labels, hierarchy and the rule that only the supervisor and the scoper write are in the
"Tracker" section of [`docs/agent-workflow.md`](../agent-workflow.md).

## Reading issues

```bash
.claude/scripts/tracker.sh ready                    # Dispatchable queue: Todo, unblocked, not needs-info/deferred, not an epic
.claude/scripts/tracker.sh list                     # Open issues (TSV: id, state, priority, title)
.claude/scripts/tracker.sh list --label area:tui    # Filter by label (or --state, --query)
.claude/scripts/tracker.sh show '#12'               # Full issue: title, state, priority, labels, blockers, body, comments
.claude/scripts/tracker.sh questions                # Open needs-info / question items
```

## Writing issues

```bash
.claude/scripts/tracker.sh create --title "..." --body-file body.md --priority Low --label feature
.claude/scripts/tracker.sh create --title "..." --body-file body.md --parent '#41' --blocked-by '#42'
.claude/scripts/tracker.sh update '#12' --add-label risky
.claude/scripts/tracker.sh block '#12' '#11'        # #12 cannot start until #11 is done
.claude/scripts/tracker.sh comment '#12' "text"     # "-" reads the text from stdin
.claude/scripts/tracker.sh state '#12' "In Progress"
```

A merged PR's `Closes #n` line closes its issue; nobody closes a work item by hand. To drop an
issue, close it as not planned: `gh issue close 12 --reason "not planned"`.

## Priority values

`Urgent`, `High`, `Medium`, `Low`, `None` (labels `priority:urgent` … `priority:low`; no label is
None). Default when filing autonomously: `Low`.

## Issue types

Labels `feature`, `bug`, `chore`, `docs`, plus `epic` for a parent issue and `question` for a pending
question. Area labels: `area:domain`, `area:gitlab`, `area:ticket`, `area:config`, `area:cli`,
`area:tui`.

## Triage label mapping

Use `tracker.sh update <id> --add-label <label>` / `--remove-label <label>` to apply triage roles.
See `triage-labels.md` for the canonical role-to-label mapping.

## Wayfinding operations

- **Map**: one issue labelled `wayfinder:map` (create the label once with
  `gh label create wayfinder:map`). Its body is the map.
- **Tickets**: child issues of the map (`tracker.sh create --parent '#<map>'`), each labelled
  `wayfinder:<research|prototype|grilling|task>`.
- **Claim**: assign the ticket before any work: `gh issue edit <n> --add-assignee @me`.
- **Blocking**: GitHub's native "blocked by" relation, `tracker.sh block <ticket> <blocker>`.
- **Frontier**: open, unassigned children of the map with no open blocker:
  `tracker.sh list --query 'parent-issue:ceffo/mrboard#<map> no:assignee'`, then drop any whose
  `tracker.sh show` lists an open blocker.
- **Resolve**: `tracker.sh comment` with a pointer to the document holding the decision, then
  `gh issue close <n> --reason completed` (a decision ticket has no PR to close it).
