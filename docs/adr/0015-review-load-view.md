# ADR-0015: Review Load View

**Status**: Proposed — wayfinder complete; ready to cut into implementation tickets

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

### CLI command shape (resolved 2026-10-19)

**`mrboard load` is its own subcommand**, named after the domain term. `mrboard fetch` is a
reproduction tool whose defaults are raw and unfiltered; a load report's defaults are the
opposite, so it is not a `fetch` mode.

| Flag | Values | Default | Meaning |
|---|---|---|---|
| `--group` | `person`, `jira` | `person` | Grouping of the tree |
| `--sort` | `person`, `jira`, `size` | `size` | Order of groups and lines; heaviest load first by default |
| `--remaining` | bool | off | Only what is left to approve |
| `--json` | bool | off | Emit the computed rows as JSON instead of the tree |
| `--color` | `auto`, `always`, `never` | `auto` | `auto` colours only on a TTY with `NO_COLOR` unset |
| `--no-filters` | bool | off | Ignore the TUI's saved header filters |
| `--view` | `mine`, `all` | saved | Override the saved view scope, as in `fetch` |
| `--reviewer-mrs` | bool | saved | Override the saved reviewer-MRs setting, as in `fetch` |
| `--cold` | bool | off | Bypass the snapshot cache, as in `fetch` |

The TUI's saved header filters apply by default — every one except sprint, per "Load population".
`--group`, `--sort` and `--remaining` are flags only and never persisted: the same command gives
the same output.

**One fetch path.** `mrboard load` calls the same core fetch as `mrboard fetch` — snapshot cache,
saved reviewer-MRs setting — and adds the sprint population (sprint dates, merged-since listing)
on top. The two commands stay at parity by construction.

**Layering.**

- The review load computation — population, crediting, totals and partial markers, grouping and
  sorting — is business logic and lives in `internal/domain` (or a service under
  `internal/domain/service`), so the CLI and the later TUI mode share it.
- The coloured tree is rendered by a plain function in `internal/tui` (no Bubble Tea model),
  using `styles.go` and the configured theme: charmbracelet stays inside `internal/tui`, and the
  TUI mode reuses the same styles.
- `--json` serialises the computed rows in `internal/cmd`.

### Fetch placement and caching (resolved 2026-10-19)

**Review size is an always-on stage of the board fetch.** Every `FetchAll` runs it, not only
`mrboard load`, so the snapshot always carries sizes: the TUI view opens with them ready and
`mrboard fetch` stays at parity without a flag. The stage is incremental:

- **Invalidation is the MR's diff head SHA.** The thin phase-1 query gains `diffHeadSha` (REST
  stubs carry `sha`). Stats are refetched only when the head SHA moves — a push. Comments, labels
  and approvals bump `updatedAt` but leave the diff, so they cost no size fetch.
- **Query shape.** One aliased GraphQL `diffStats { path additions deletions }` query per batch
  of at most 40 MRs whose SHA moved, for REST- and GraphQL-sourced MRs alike. The constant
  (`gqlDiffStatsBatchSize`) is 30, not yet measured against a real complexity score: GitLab scores
  each selected field 1 and a Gitaly-backed field 1 more (the discussions batch measured exactly its
  field count, 16 per alias), so a `diffStats` alias is estimated at 7 and 30 of them at ~213,
  leaving margin under the 250 ceiling for an error in the estimate. Each query also requests
  `queryComplexity { score limit }`, logged at debug level; a real run's score may raise the cap.
- **Timing.** The size stage runs concurrently with phase-2 discussions and `FetchAll` returns
  once both finish: one result, one snapshot save.
- **The snapshot caches per-file stats**, not the computed size. Review size is derived at read
  time, so editing `review_size.exclude` or a project's `.gitattributes` re-applies without a
  refetch.
- **Failure.** An MR whose SHA moved and whose stats fetch failed has an unknown size (`?`, totals
  partial) until a fetch succeeds — stats of an older SHA describe a diff that no longer exists.
  An MR whose SHA did not move keeps its cached stats. The error joins `FetchAll`'s partial-result
  errors.

**`.gitattributes`** is cached on disk per (project, target branch) with a fixed 24h TTL, beside
the ticket cache in the XDG cache dir; a missing file is cached as "no rules". Expired projects are
read together in one aliased GraphQL `repository.blobs` query. The TTL is not configurable:
generated-file markings change rarely, and `--cold` is the escape hatch.

**The merged-since listing is gated.** `mrsvc.FetchOptions.MergedSince` (zero means skip) adds
merged-after listings over the board's sources to the list stage, through the same dedup and
exclusions. Merged MRs need only the thin query — reviewers, `approvedBy` and approval rules give
everything the load needs — so they skip phase-2 discussions; they do get the size stage. The
board passes zero and is unchanged; `mrboard load`, and later the TUI view, pass the sprint start.

**Merged MRs persist in a separate merged snapshot**, tagged with the sprint start it was listed
for and dropped when that start changes. A board-only fetch never overwrites it, so a merged MR's
stats are fetched once per sprint, and the board snapshot stays open MRs only. Each load still
re-lists merged MRs with the thin query, so approvals recorded after merge are current.

**`mrboard fetch` parity.** Fetch output shows each MR's review size (or `?`), and its counted and
excluded files in verbose mode. `--merged-since YYYY-MM-DD` drives the merged listing without the
ticket port, keeping the reproduction tool independent of the sprint source. `--cold` bypasses the
snapshot, merged snapshot and `.gitattributes` caches alike.

### TUI display mode (resolved 2026-10-19)

**`L` toggles the load view**, a full-screen mode that replaces the board; `L` or Esc returns to
it. My View (`tab`) stays an independent scope and narrows the load view as any header filter
does, per "Load population". Launch always opens the board: whether the view was open is not
persisted.

**The widget** is a self-contained `internal/tui` widget over the same domain rows and the same
tree renderer as the CLI. Selection moves over MR lines only; group headings are not selectable.
Detail (`↵`), open, copy and diff reuse the board's actions on the selected MR.

**Keys** live in a new `LoadCtx` in `internal/tui/keys.go` (`docs/keybindings.md`):

| Key | Action |
|---|---|
| `g` | Toggle grouping: person ↔ jira |
| `s` | Cycle sort: person → jira → size |
| `R` | Toggle remaining only |

**Persistence.** Group, sort and remaining-only are saved in `state.yaml` as `load_group`,
`load_sort` and `load_remaining`, like the board's `sort_field`. The CLI ignores them: its flags
stay the only input, so the same command gives the same output.

**Merged listing timing.** The TUI passes `FetchOptions.MergedSince` only while the view is open.
Opening it renders the cached merged snapshot at once and starts a fetch with `MergedSince`;
refreshes keep passing it until the view closes. Board-only use costs no extra request.

## Consequences

- The ticket port carries the active sprint's start and end dates alongside its issue keys, under
  a vendor-neutral name (rule 7 in `CLAUDE.md`), cached under the existing `SprintCacheTTL`.
- The GitLab adapter gains a merged-since listing over the board's sources; `mrboard fetch` keeps
  parity.
- A new `review_size` config section (exclude globs, default-excludes switch, bucket thresholds).
- The GitLab adapter gains a per-project `.gitattributes` read and 24h disk cache, and a
  SHA-keyed `diffStats` stage in `FetchAll`; the thin phase-1 query gains `diffHeadSha`.
- `domain.MergeRequest` carries its head SHA and per-file diff stats, persisted by the snapshot.
- `mrsvc.FetchOptions` gains `MergedSince`; a second snapshot store holds merged MRs per sprint.
- `mrboard fetch` gains `--merged-since`, and `--cold` covers every new cache.
- `domain.AppState` gains `load_group`, `load_sort` and `load_remaining`; `keys.go` gains `LoadCtx`
  and the board's `L` action.
