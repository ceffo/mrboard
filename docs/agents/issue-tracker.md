# Issue tracker: Beads

Issues for this repo live in `.beads/beads.jsonl`, managed by the `br` CLI (beads-rust).

## Reading issues

```bash
bv --robot-triage | toon              # Triage: top picks + health
bv --robot-next | toon                # Single top pick
br list --status=open --format toon   # All open issues
br show <id> --format toon            # Full issue details
br ready --format toon                # Unblocked (ready to work)
```

## Writing issues

```bash
br create --title="..." --type=task --priority=2
br create --title="..." --type=task --priority=2 --parent <epic-id>
br update <id> --status=in_progress         # Claim
br update <id> --add-label needs-triage     # Apply a triage label
br close <id> --reason="Completed"
br sync --flush-only                        # Export DB to JSONL (run before every commit)
```

## Priority values

P0=critical, P1=high, P2=medium, P3=low, P4=backlog (use numbers 0–4, not words).

## Issue types

`task`, `bug`, `feature`, `epic`, `chore`, `docs`, `question`

## Triage label mapping

Use `br update <id> --add-label <label>` / `--remove-label <label>` to apply triage roles.
See `triage-labels.md` for the canonical role-to-label mapping.

Additional state transitions:
- Claiming work: `br update <id> --claim` (sets assignee + status=in_progress atomically)
- Closing as wontfix: `br close <id> --reason="wontfix"`
