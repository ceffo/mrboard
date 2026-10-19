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

- it always covers one sprint, the active one, and the board's header filters narrow it
  (`docs/adr/0012-header-filter-state.md`);
- MRs merged during the sprint count alongside open ones, so finished reviews still show as
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
- Navigating to previous sprints. The view covers the active sprint only; exploring history is a
  later effort.

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

### Load population (resolved 2026-10-15)

Evidence: `docs/research/sprint-merged-mrs.md` on branch `research/sprint-merged-mrs`.

**The view is always scoped to one sprint: the active one.** Sprint scoping is what the view
*is*, not a filter on it, so the board's `S` sprint toggle does not apply here.

**An MR is in the sprint's load** when its ticket key — read from the title by the same
`TicketKeyMatcher` the board uses — is one of the sprint's issue keys, and:

- it is open; or
- it was merged on or after the sprint's start date. The date bound keeps MRs of a ticket carried
  over from an earlier sprint from counting again.

**Unticketed MRs are included in a "no ticket" group**: open MRs whose title has no ticket key,
and unticketed MRs merged on or after the sprint start. They cost review time even without a
ticket. MRs closed without merging are excluded.

Merged MRs come from the same sources as the board's open MRs, listed with GraphQL
`mergedAfter` (available since GitLab 13.3; the REST equivalent needs 19.3).

**Header filters.** Every board filter except sprint applies to the view as it does to the board:
the "me" scope, issue and assignee exclusions narrow the MR set; phase columns narrow open MRs only
(a merged MR has no phase); an excluded reviewer loses their person row. ADR-0012's denominator
invariant holds: the header count states the population the filters ran against.

**Approvers.** An MR's load goes to its assigned reviewers flagged `IsApprover`, open or merged —
GitLab keeps reviewers on a merged MR, so the same rule serves both. Each team-roster member gets
a person row; approvers outside the roster are aggregated into one "others" row so no load
silently disappears.

**Remaining only.** The toggle hides a person's row for an MR once that person has approved it or
the MR is merged. Drafts stay, marked as drafts.

### CLI output layout (resolved 2026-10-19)

Prototype: `prototypes/review-load-cli/` on branch `prototype/review-load-cli` — three layouts
(tree, flat table, ticket × person matrix) over the same fake sprint.

**The output is a tree: group headings carrying their total, MR lines beneath.** It answers the
standup question "who carries what" at a glance. A flat approver × MR table was rejected because
it repeats every shared MR; a ticket × person matrix was rejected because it hides the MRs.

A header line states the sprint (name and dates), the MR count and the scope (all / remaining
only).

**Grouped by person** (the default), each block is headed by the approver's review load. One line
per MR they approve: `repo!iid`, ticket, review size, size bucket, co-approvers (`+dave
+others`), state (draft, merged, ✓ approved) and title. The co-approvers column makes the
full-credit double counting visible; a footer states that person loads exceed the team total.

**Grouped by ticket**, each block is headed by the ticket's total, counting each MR once. One line
per MR — not per approver — listing its approvers, with ✓ on those who approved, so the lines
match the heading.

**Sorting** by person, ticket or size orders the groups (by key, or by group total for size) and
the lines within each group. The catch-all groups — "others" for persons, "(no ticket)" for
tickets — always come last, whatever the sort, so aggregated load never outranks a named person
or ticket.

## Consequences

- The ticket port carries the active sprint's start and end dates alongside its issue keys, under
  a vendor-neutral name (rule 7 in `CLAUDE.md`), cached under the existing `SprintCacheTTL`.
- The GitLab adapter gains a merged-since listing over the board's sources; `mrboard fetch` keeps
  parity.
- A new `review_size` config section (exclude globs, default-excludes switch, bucket thresholds).
- The GitLab adapter gains a per-project `.gitattributes` read and cache, and per-MR `diffStats`.
