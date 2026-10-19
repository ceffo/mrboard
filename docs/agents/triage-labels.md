# Triage Labels

The skills speak in terms of five canonical triage roles. This file maps those roles to the labels
and states this repo's GitHub Issues tracker uses (see the "Tracker" section of
[`docs/agent-workflow.md`](../agent-workflow.md)).

| Label in mattpocock/skills | In our tracker                                   | Meaning                                  |
| -------------------------- | ------------------------------------------------ | ---------------------------------------- |
| `needs-triage`             | label `question`                                 | Maintainer needs to evaluate this issue  |
| `needs-info`               | label `needs-info`                               | Waiting on reporter for more information |
| `ready-for-agent`          | no label: open, unblocked, no `needs-info`/`deferred` | Fully specified, ready for an AFK agent  |
| `ready-for-human`          | label `deferred`                                 | Requires human implementation; no agent dispatches it |
| `wontfix`                  | closed as not planned, label `wontfix`           | Will not be actioned                     |

When a skill mentions a role (e.g. "apply the AFK-ready triage label"), apply the corresponding
label from this table with `.claude/scripts/tracker.sh update <id> --add-label <label>`. "Ready for
agent" is the absence of a blocking label: `tracker.sh ready` lists exactly those issues.
