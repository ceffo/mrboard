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

### Review size metric (resolved 2026-10-15)

Evidence: `docs/research/review-effort-metric.md` on branch `research/review-effort-metric`.

**Vocabulary.** An MR's **review size** is the number of counted lines it changes. An approver's
**review load** is the sum of the review sizes of the MRs they are responsible to approve. Neither
term claims to measure review *time*: line count correlates weakly with time (Baysal, r = 0.09)
and more strongly with revision rounds, so the view reports size, not effort.

**Review size = additions + deletions over counted files.** A modified line counts twice (one
deletion, one addition) — standard churn, the most parsimonious proxy in the literature (Rigby).
Files touched is not an input: it is largely collinear with churn, and a second axis would leave
no single number to sort by.

**Counted files** are every file in the MR diff except those matching an exclusion pattern:

- built-in globs mirroring GitLab's collapsed-by-default set (lock files, `vendor/`,
  `node_modules/`, minified JS/CSS, source maps, generated Go/protobuf, …);
- `review_size.exclude:` globs from `mrboard.yaml`, which extend the built-ins;
  `review_size.default_excludes: false` drops the built-ins instead;
- paths the project's root `.gitattributes` marks `gitlab-generated` or `linguist-generated`, read
  from the MR's target branch and cached per project. These always apply.

Test files are counted fully: the evidence says tests warrant review (Spadini 2018).

**Source.** Per-file line counts come from GraphQL `MergeRequest.diffStats { path additions
deletions }`, which batches across MRs. GraphQL exposes no generated-file flag, so the exclusion
patterns above are matched by mrboard. This approximates GitLab's own detection (go-enry
heuristics plus `.gitattributes`) rather than reproducing it; an MR's review size can differ from
what GitLab collapses in its Changes tab.

**Presentation.** Each MR shows its raw review size and a size bucket: S ≤ 100, M ≤ 400,
L ≤ 1000, XL > 1000 counted lines (defaults overridable in `mrboard.yaml`). Thresholds follow the
~400 LOC quality drop (SmartBear/Cisco) and Google's ~1250-line mark. Sorting by size and every
total use the raw number; a bucket is a label, never summed.

**Unknown size.** An MR whose size cannot be measured (stats not yet fetched, diff overflow) shows
`?`. Totals sum the known sizes and are marked partial (e.g. `1240+`), so missing data is never
read as zero and no responsibility is hidden.

### Crediting review size to approvers (resolved 2026-10-15)

**Every assigned approver is credited the MR's full review size.** In mrboard's domain the
approvals an MR requires are its assigned approvers (`docs/domain-model.md`: required =
reviewers with `IsApprover`), so each of them must read the whole diff; splitting the size would
model a shared reading that does not happen.

Consequently, totals differ by axis. A person's review load sums the full size of each MR they
approve. A per-Jira total and the team total count each MR once. Person loads therefore do not
add up to the team total when MRs share approvers — by design.

**Re-review rounds add nothing.** Review size is a property of the MR, not of the number of
passes over it; a re-review reads a delta. Rounds remain visible through the approver's reviewer
state.

**Approval does not remove load.** An MR an approver has already approved stays in their load
at full size, marked approved: the view reports load carried this sprint. Only the "remaining
only" toggle hides it.

## Consequences

- A new `review_size` config section (exclude globs, default-excludes switch, bucket thresholds).
- The GitLab adapter gains a per-project `.gitattributes` read and cache, and per-MR `diffStats`.
