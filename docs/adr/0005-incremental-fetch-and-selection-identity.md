# ADR-0005: Incremental Fetch, Background Refresh, and Selection Identity

**Status**: Implemented 2026-07-31 — all decisions resolved 2026-07-30; both verification items
(phase-1 findings, phase-2 timing) settled during implementation, see below

## Context

Three defects in mrboard's data path share one root: the TUI treats a fetch as a modal,
all-or-nothing event that rebuilds the board from scratch.

**Latency.** Measured over 96 fetches in a real session log: an authored-only fetch takes 2s;
enabling `include_reviewer_mrs` takes 3–9s. `enrichment done` is always `0s` — the current
configuration is entirely on the GraphQL path, so all of the time is in `listStage`. The per-source
GraphQL queries already run in parallel goroutines (`gitlabadpt.go` `listAllMRs`/`listReviewerMRs`),
so wall-clock is `max(query)`, not `sum(query)`. Batching them into one aliased request would make
GitLab process them serially and would likely be *worse*. Only two levers move `max(query)`: make
each query cheaper, or stop issuing the expensive ones.

Two structural costs dominate. First, `gqlUserMRsQuery`/`gqlReviewerMRsQuery` request
`discussions(first: 100) { nodes { notes(first: 100) { ... body ... } } }` — up to 10,000 note
bodies per MR — while `mapper.go`'s `normalizeDiscussionEventsGQL` only reads `body` on *system*
notes, discarding it for human notes. Second, dedup runs *after* the expensive work: the log shows
`mapped: 29` collapsing to roughly a dozen unique MRs, meaning the same MR's full discussion payload
is fetched once per teammate who is a reviewer on it, then thrown away in `dedupStage`.

**Blocking.** `isRefreshing` gates `baseStack()` down to `BaseCtx` only, makes `handleKey` return
early, and draws a full-board spinner overlay. The board is not merely visually busy during a
refresh — it is keyboard-dead for 2–9 seconds. There is also no persisted data, so every launch
shows an empty loading screen until the first fetch completes.

**Selection loss.** Every call to `applyMRFilter()` ends in `board.SetMRs()` → `setInitialFocus()`,
which jumps to the first card of the first non-empty column. There are eight call sites — sort,
sprint toggle, view toggle, settings applied, reviewers saved, two ticket-result handlers, and fetch
result. Only the manual-`r` path attempts restoration, via `prevFocusMR`, and
`TryRestoreFocus(colIdx, mrIID)` matches on **IID alone** with no `ProjectID`, so MRs from different
projects with the same IID collide.

## Non-goals

- **Streaming partial results into the board.** Considered and rejected: cards would visibly pop in
  and reorder, header phase counts would lie until the last source landed, and selection restoration
  would run repeatedly per fetch. With a warm boot cache the perceived cold-start latency is already
  near zero, so streaming buys little and costs flicker and selection churn.
- **A generic caching decorator around `mrsvc.MergeRequestSource`.** A decorator can only cache
  whole `FetchAll` calls. The two-phase split is inherently GitLab-query-shaped — a thin GraphQL
  selection set versus a fat one — so the phase logic belongs inside `gitlabadpt`, which is exactly
  the vendor-specific translation an adapter exists to do.
- **Replacing per-user queries with group-scoped ones.** Potentially the largest win, but it changes
  which MRs the board shows and depends on GitLab topology the current config does not describe (it
  uses zero group sources). Out of scope here.
- **Aligning the duration-string tick to real minute boundaries.** Real but unrelated; tracked
  separately. See "Consequences" for why it yields accuracy rather than fewer wakeups.

## Decision

### Snapshot semantics: atomic swap, never partial (resolved 2026-07-30)

The board always displays one coherent set of MRs. A refresh computes a complete snapshot
off-screen and replaces the previous one in a single `Update`. Cards never appear, disappear, or
reorder mid-fetch; header counts are never transiently wrong; selection restoration runs exactly
once per swap.

This is the decision every other one below depends on. It removes any need for a channel-based
subscription `tea.Cmd` and keeps the fetch path a plain request/response `tea.Cmd` as it is today.

### Two-phase conditional fetch (resolved 2026-07-30)

`FetchAll` in `gitlabadpt` splits into two phases:

1. **Phase 1 — thin listing.** Per-source GraphQL queries with a reduced selection set: scalars
   (`iid`, `updatedAt`, `title`, `draft`, `createdAt`, `webUrl`, `detailedMergeStatus`,
   `sourceBranch`, `targetBranch`), `author`, `assignees`, `reviewers`, `approvedBy`,
   `approvalState`, `project` — and **no `discussions`**. Sources run in parallel exactly as today.
   Dedup to unique `(projectID, iid)` happens here, *before* any expensive work, rather than after
   it as `dedupStage` does today.
2. **Phase 2 — targeted enrichment.** A single aliased GraphQL request
   (`mr0: project(fullPath:){mergeRequest(iid:){discussions{…}}} mr1: …`) fetching `discussions`
   only for MRs whose `updatedAt` differs from the previous snapshot's.

Expected steady state: phase 1 ≈ 200ms, phase 2 for the 1–3 MRs that typically change during a
standup ≈ 400ms, total ≈ 0.6s against today's 2–9s. A cold cache degenerates to phase 2 over every
unique MR — still strictly better than today, because it fetches each MR once instead of up to five
times.

`updated_at` is a faithful version marker for note-derived data: GitLab bumps it on every note,
approval, reviewer change, and title/draft edit. Two known gaps are handled explicitly below.

### What the cache is allowed to answer for (resolved 2026-07-30)

`detailedMergeStatus` is **not** covered by `updated_at`: an MR can go from `mergeable` to
`conflict` (someone merged into the target branch) or from `ci_still_running` to `mergeable` with no
`updated_at` movement. That field drives the Ready to Merge column via `ClassifyPhase`. It is also a
cheap scalar that phase 1 returns for free.

Therefore, on an `updatedAt` match the cached MR answers **only** for the discussion-derived fields
— `Reviewers`, `OpenThreads`, `RoundTripCount` — and phase 1's freshly-fetched values always
overwrite `Title`, `Draft`, `DetailedMergeStatus`, `Assignee`/`AssigneeName`, the reviewer
reference list, `approvedBy`, and `UpdatedAt`. `ClassifyPhase`, `DeriveWaitingSince`, and
`deriveReadyToMergeSince` then re-run against the merged result. A card can never sit in the wrong
column because of a stale merge status.

`approvalState.rules` stays in phase 1 rather than moving to the cached phase 2, deliberately:
`SaveApprovers` writes a *separate* GitLab resource (the MR approval rule), which almost certainly
does not bump the MR's `updated_at`. Local writes are covered by the dirty-set rule below, but a
teammate editing approvers in the GitLab web UI would otherwise be invisible to a cache keyed on
`updated_at`. Keeping the field always-fresh closes that hole. If measurement shows this resolver
dominates phase 1's latency, the trade is worth revisiting — as a deliberate decision, not silently.

### Phase-1 thin query: implementation findings (resolved 2026-07-31)

Two open questions from the phase-1 ticket (`mrr-incremental-fetch-3sl.3`) are now answered against
the live `gl.nsesi.io` schema and traffic, not left as assumptions:

- **`resolvedDiscussionsCount` / `resolvableDiscussionsCount` exist as scalars on `MergeRequest`**,
  confirmed via a live GraphQL introspection query and a live data query returning real non-zero
  counts (e.g. a merged MR with `resolvedDiscussionsCount: 3, resolvableDiscussionsCount: 3`). Both
  fields are non-deprecated. They are now requested in the thin query alongside `approvalState`.
  This makes the "does resolving a thread bump `updated_at`" question moot for a future phase-2: a
  changed resolved/resolvable count is itself a cache-invalidation signal independent of
  `updated_at`, and `resolvableDiscussionsCount - resolvedDiscussionsCount` gives `OpenThreads`
  directly without a discussions fetch when the two snapshots' counts already agree.
- **`approvalState` does not dominate phase-1 latency.** Three timed runs of the thin listing query
  (5 users x authored + reviewer-requested, run in parallel — the same source shape the 2–9s fat-query
  baseline used) against `gl.nsesi.io`, with and without the `approvalState { rules { ... } }` block,
  measured 584–964ms with the block and 579–1320ms without it. The two conditions are within each
  other's noise band; there is no systematic difference attributable to `approvalState`. No action
  needed — the field stays in phase 1 as designed above.

Phase-1 wall-clock for this same 5-user authored+reviewer shape: **~0.6–1.3s**, down from the fat
query's measured 2–9s baseline, even before phase 2's per-MR diffing removes the enrichment fetch
entirely on a cache hit. The remaining phase-1 cost is now dominated by network round-trip count
(10 parallel requests) rather than payload size, since `discussions` was the only field requesting
note bodies.

### Phase-2 aliased query: measured timing (resolved 2026-07-31)

End-to-end `GitLabAdapter.FetchAll` against the live `gl.nsesi.io` config (5 user sources, 11 MRs,
`IncludeReviewerMRs: true`), measured twice back-to-back — cold with a nil `Previous` (full fetch,
phase 2 runs for every MR), then warm with `Previous` set to the cold result (steady state, nothing
changed between the two calls so every MR's `updatedAt` matches):

- **Cold**: 3.80s and 4.10s across two runs — inside the pre-epic 2–9s baseline, since a cold fetch
  still pays for phase 1 plus a phase-2 discussions fetch for every MR (no cache to skip against).
- **Warm**: 667ms and 651ms — matches the ADR's predicted ~0.6s steady state almost exactly. Phase 2
  issued zero requests in both warm runs (all 11 MRs unchanged), so this is effectively phase-1's
  wall-clock alone, consistent with the ~0.6–1.3s phase-1 measurement above.

This confirms the epic's central claim end-to-end, not just per-phase: a warm cache turns a 2–9s fetch
into ~0.6s, a 6–14x improvement, with the two-phase split fetching every MR's discussions exactly once
per real change rather than once per fetch.

### Cache ownership: passed in, not held (resolved 2026-07-30)

`GitLabAdapter` is stateless today — `New(client, cfg)`, with every method a pure function of its
arguments, which is what lets `gitlabadpt_test.go` drive it with mockery fakes. Giving it a
persistent cache would make it stateful and couple a vendor adapter to disk persistence.

Instead, `mrsvc.FetchOptions` gains `Previous []domain.MergeRequest`. The adapter runs phase 1,
diffs against `Previous`, runs phase 2 for changed keys only, merges, and returns a complete
snapshot. The adapter stays pure and the two-phase logic is unit-testable by passing a `Previous`
slice — no state accumulated across calls.

Persistence becomes a new `domain.SnapshotStore` port, sibling to the existing `domain.StateStore`,
implemented by a new adapter writing `~/.cache/mrboard/snapshot.json` (mode 0600) via a new
`config.XDGCacheDir()` helper mirroring the existing `XDGConfigDir()`/`XDGDataDir()`. `$XDG_CACHE_HOME`
is correct rather than the data dir: losing this file must cost nothing but one slow fetch, which is
precisely its contract. `domain.MergeRequest` gains `UpdatedAt time.Time`.

The file carries a `version` int; on mismatch it is ignored and refetched, which is how domain-schema
changes invalidate it. Config changes need **no** invalidation mechanism: the snapshot is only ever a
lookup table keyed by MRs that phase 1 returned, so removing a source means its MRs are never looked
up and disappear naturally.

`mrboard fetch` (the CLI JSON dump) mirrors the TUI's own fetch by default — same saved
`IncludeReviewerMRs` setting, same on-disk `Previous` snapshot — so discussion-derived bugs
(reviewer state, round trips) can be reproduced from the CLI without driving the interactive board.
`--cold` forces a nil `Previous` for a full unconditional recompute of every MR; `--reviewer-mrs`
overrides the saved setting. It never writes the snapshot itself, so it cannot corrupt the TUI's
cache no matter which flags are used.

### Non-blocking refresh (resolved 2026-07-30)

`isRefreshing` stops gating `baseStack()` and `handleKey`, and the full-board spinner overlay is
removed from `View`. Every key works at every moment; a refresh is signalled only by the header,
which carries the snapshot's age and an in-progress indicator (`⠿ 14m ago` → `just now`).
`stateLoading` survives solely for a genuinely cold cache — no snapshot file, nothing to draw.

At boot the cached board renders immediately, fully interactive, **at any age**, with the age stated
in the header. There is no staleness ceiling: a three-day-old board is more useful than an empty
one, the label makes it impossible to mistake for live, and the first fetch is running the whole
time anyway.

### The write race that ungating creates (resolved 2026-07-30)

Writes cannot race a refresh today because the keyboard is dead during one. Ungating makes this
reachable: press `R` at T+0, save reviewers at T+2 (`handleReviewersSaved` updates `m.allMRs` and
calls `applyMRFilter`), and the snapshot that started at T+0 lands at T+2.5 carrying the *old*
reviewer list, visibly reverting the edit.

`Model` keeps `dirty map[domain.MRKey]time.Time`, recording when each locally-mutated MR was
written. A landing snapshot applies to every clean key; for dirty keys whose `fetchStartedAt` is
earlier than `writeAt` it skips the entry, leaving the local version in place, and issues a
**targeted phase-2 refetch for just those MRs**. A dirty entry clears when a fetch that started
after `writeAt` returns that MR.

Refetching only the dirty MRs — rather than discarding the whole snapshot — costs one aliased query
for one or two MRs, and requires no new machinery: phase 2 already fetches individual MRs by
`(projectID, iid)`, so this is a phase-2 call with a forced-stale set. It also needs no shadow
write-log or per-field merge rules, because `handleReviewersSaved` already replaces `m.allMRs[i]`
wholesale with a complete `updatedMR` from the write path — the local entry is already authoritative.

### Refresh cadence (resolved 2026-07-30)

Auto-refresh every 60s, via a new `refresh_interval` config key (default `60s`, `0` disables).
Manual `R` still works and resets the timer. A tick is skipped while a fetch is already in flight —
`fetchTimeout` is 60s, so overlap is otherwise reachable.

This is only viable because of the two-phase work: the common outcome is phase 1 finding nothing
changed, which means no phase 2, no snapshot swap, and therefore no selection churn — the timer is
invisible by default. The board is left open during standups; requiring someone to remember to press
`R` is the reason it goes stale.

Auto-refresh is **not** suppressed while an overlay is open. The detail panel and diff view hold
their own copy of the MR they were opened on, so they simply do not update — acceptable, and
preferable to a board that can be arbitrarily stale whenever someone leaves a modal open.

### Selection identity (resolved 2026-07-30)

Selection is keyed by a new exported `domain.MRKey{ProjectID, IID}` — a promoted version of the
private `mrKey` already in `gitlabadpt` — which fixes the IID-only collision in today's
`TryRestoreFocus`.

`Model.selected domain.MRKey` is the single source of truth. `board.SetMRs` takes the key and
resolves `focusedCol`/`focusIdx` from it on every call, so all eight `applyMRFilter()` call sites are
fixed **by construction** rather than one at a time; arrow-key movement writes back to `m.selected`.
`prevFocusMR` and `TryRestoreFocus` are deleted.

Focus follows the MR across columns when its phase changes. The card visibly relocated; leaving the
highlight behind on an unrelated card would be the more jarring outcome.

When the selected MR is absent from the new snapshot — merged, closed, or filtered out — focus stays
in the column it was in and keeps the same row index, clamped to the new length; if that column is
now empty, it falls back to the first non-empty column. `m.selected` is reassigned to whatever card
it lands on. This matches the common case: an MR merges while you are working down a column, and
focus lands on the card that took its place.

## Consequences

- `mrsvc.FetchOptions` gains `Previous []domain.MergeRequest`, and `domain.MergeRequest` gains
  `UpdatedAt`. The `MergeRequestSource` interface signature is unchanged, so existing mocks are
  unaffected; the new `domain.SnapshotStore` port needs a mockery entry.
- `gitlabadpt` grows a second GraphQL query shape (thin) alongside the existing fat one, plus an
  aliased multi-MR discussions query. The REST fallback path (`fetchSourceViaREST`, `enrichMR`) is
  untouched and remains the unconditional-full-fetch escape hatch.
- The snapshot file is a new on-disk compatibility surface. It is versioned and disposable, but every
  future change to `domain.MergeRequest` must bump the version or accept zero-valued fields on the
  first fetch after upgrade.
- A teammate's approver edit made in the GitLab web UI is visible because `approvalState` is
  always-fresh in phase 1. Should that resolver later be moved to phase 2 for latency reasons, that
  staleness hole reopens and must be reconsidered explicitly.
- The board is never keyboard-dead, which permanently removes `isRefreshing` as a guard. Any future
  action added to the board must be safe to invoke while a fetch is in flight, or must register its
  MR in the dirty set.
- Auto-refresh means the board mutates without user input. Any future feature that assumes the MR
  set is stable between keypresses is now wrong by default.
- Selection is a `domain.MRKey` rather than a position, so any future widget that shows a per-MR
  selection (batch preview, reviewer editor) should key off the same identity rather than an index.
- Separately tracked: the duration tick fires every 60s from app start, so a string that should flip
  at the top of the minute flips up to 59s late. Computing the next real boundary makes flips
  *accurate*; it does not make them *fewer*, because `FormatDuration` renders `Xh Ym` — minute grain
  — for everything under 24 hours, so any MR younger than a day forces minute-grain ticking anyway.
