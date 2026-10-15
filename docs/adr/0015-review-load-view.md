# ADR-0015: Review Load View

**Status**: Proposed — wayfinder in progress; decisions accumulate below as tickets resolve

## Context

mrboard shows where each MR stands, but not how review effort is spread across the team. A team
lead needs to see, per team member, which MRs they are responsible to approve, how much effort
each one represents, and the total — to spot overloaded reviewers and rebalance during a sprint.

"Responsible to approve" means an assigned reviewer with `IsApprover` on the MR, in any reviewer
state (see `docs/domain-model.md`). The team is the roster from `config.TeamUsernames`
(`docs/adr/0009-auto-assign-reviewers.md`).

The view is sprint-centric:

- the board's header filters (sprint, author, …; `docs/adr/0012-header-filter-state.md`) scope it;
- MRs merged during the active sprint count alongside open ones, so finished reviews still show as
  load carried this sprint;
- a "remaining only" toggle narrows it to what is still left to approve;
- draft MRs are included and marked as drafts.

Each MR needs an effort metric: a size-derived proxy for how costly it is to review. The starting
point is lines added plus lines modified; the final metric is decided below.

Rows can be sorted by person, Jira ID and effort, and grouped by person and by Jira ID.

The destination of this effort: this ADR fully specifies the metric, the population, the output
and the fetch cost, so implementation can be cut into tickets. Delivery is CLI-first — an output
of the `mrboard` CLI to iterate the metric and layout against real data — then a TUI display mode
once the CLI output is settled.

## Non-goals

- Implementation. This ADR ends at a buildable spec; execution is cut into separate tickets.

## Decision

_No sub-decisions resolved yet._

## Consequences

_Filled in as decisions resolve._
