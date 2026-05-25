# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

*Add reusable patterns discovered during development here.*

---


## 2026-05-25 - mrr-28q.1
- Extracted `DiscussionEvent{AuthorUsername, Timestamp, Kind}` normalized type with `KindComment`, `KindReReviewRequest`, `KindApproval` constants
- Added `normalizeDiscussionEventsREST` and `normalizeDiscussionEventsGQL` normalizers (both skip resolved threads)
- Added pure `deriveReviewerState(approved bool, events []DiscussionEvent) ReviewerState` — directly testable, no API types
- Added shared `buildReviewerInfos` — single place where reviewer state + timestamps + WaitingSince/ApprovedAt are derived
- Added `countRoundTripsFromEvents` — counts KindReReviewRequest events
- Added `threadState` + `countOpenThreads([]threadState)` + `restThreadStates`/`gqlThreadStates` normalizers for unified open thread counting
- Removed `DeriveReviewerStatesFromGQL`, `countOpenThreadsGQL`, `countRoundTripsGQL`
- Added 9 table-driven tests for `deriveReviewerState`; updated `TestCountRoundTripsFromEvents` to use new event-based API
- **Files changed**: `internal/adapters/gitlabadpt/mapper.go`, `internal/adapters/gitlabadpt/mapper_test.go`
- **Learnings:**
  - `countRoundTrips` previously skipped resolved-thread skipping (iterated all notes), while `normalizeDiscussionEventsREST` skips resolved threads. In practice, re-review system notes never appear inside resolved comment threads, so the behavior is identical. Accepted the minor theoretical behavioral alignment.
  - `threadState` slice approach cleanly unifies open-thread counting without needing a generic type parameter or interface — just two tiny normalizer functions.
  - Pre-filtering events per reviewer is not needed in `buildReviewerInfos` — the shared map accumulation is O(N+M) and cleaner than O(N*M) per-reviewer filtering.
---

## 2026-05-25 - mrr-28q.3
- Extracted `MRDeduplicator{ExcludedAuthors []string}` to `internal/adapters/gitlabadpt/dedup.go`
- `Deduplicate([]domain.MergeRequest) []domain.MergeRequest` — pure function: excludes by author, deduplicates by project+IID, preserves first-occurrence order
- Moved `mrKey` struct from `gitlabadpt.go` to `dedup.go`
- Refactored `FetchAll` into three labelled phases: **fetch** (listAllMRs), **deduplicate** (combine mappedMRs + raw stubs → Deduplicate → separate by source), **enrich** (enrich unique raw survivors)
- Exclusion logic now fires exactly once, inside `Deduplicate`; all scattered `excluded[...]` checks removed from `FetchAll`
- Added 9 table-driven tests covering: empty input, no-op, cross-source dedup, author exclusion, multiple exclusions, same-IID different project, order preservation
- **Files changed**: `internal/adapters/gitlabadpt/gitlabadpt.go`, `internal/adapters/gitlabadpt/dedup.go` (NEW), `internal/adapters/gitlabadpt/dedup_test.go` (NEW)
- **Learnings:**
  - To combine `mappedMRs ([]domain.MergeRequest)` with `rawMRs ([]*gl.BasicMergeRequest)` for a single Deduplicate pass, build minimal domain.MergeRequest stubs from rawMRs (just ProjectID, IID, Author), append after mappedMRs (so mapped wins on collision), then after dedup separate survivors by checking a `mappedKeys` set and `rawByKey` lookup map.
  - `goconst` linter fires on test string literals used ≥3 times — define a package-level test constant instead.
---

## 2026-05-25 - mrr-28q.4
- Created `internal/tui/filter.go` with `FilterCriteria{Phases map[domain.MRPhase]bool; Authors []string; Reviewers []string}`
- Replaced `filterPhases`/`filterAuthors`/`filterReviewers` fields in `Model` with single `filter FilterCriteria`
- `newFilterPopupWidget` now takes `current FilterCriteria` instead of three separate params
- `FilterAppliedMsg` carries `Criteria FilterCriteria` instead of separate fields
- `applyMRFilter()` and `isFilterActive()` read from `m.filter` only
- **Files changed**: `internal/tui/filter.go` (NEW), `internal/tui/filter_popup.go`, `internal/tui/model.go`
- **Learnings:**
  - The `domain` import in `filter_popup.go` remained valid because `FilterCriteria.Phases` uses `domain.MRPhase` — no import churn needed.
  - Consolidating into a value type with zero-value semantics (nil map/slice = no filter) keeps `isFilterActive()` clean with no special cases.
---

## 2026-05-25 - mrr-28q.2
- Created `internal/domain/state.go` with `AppState{SortField, SortDesc, ViewMode, ThemeName, ThemeMode}`, `DefaultAppState()`, `StateStore` interface, and `ViewMode`/`ViewAll`/`ViewMine` constants (moved from tui)
- Updated `internal/adapters/statestore/statestore.go` to import `domain` instead of `tui`, implementing `domain.StateStore`
- Updated `internal/tui/state.go` — emptied (all types moved to domain)
- Updated `internal/tui/model.go` — all `ViewMode`/`ViewAll`/`ViewMine`/`StateStore`/`State{}`/`DefaultState()` references now use `domain.` prefix
- Updated `internal/tui/model_test.go` — `noopStore` now implements `domain.StateStore`
- Updated `internal/tui/export_test.go` — `ViewMine` → `domain.ViewMine`
- Updated `internal/core/core.go` — `StateStore domain.StateStore`, removed `tui` import
- **Files changed**: `internal/domain/state.go` (NEW), `internal/adapters/statestore/statestore.go`, `internal/tui/state.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/tui/export_test.go`, `internal/core/core.go`
- **Learnings:**
  - `tui.State` was already purely a DTO for persistence (model used individual fields, not the struct). Moving it to `domain.AppState` was a clean rename + relocation with no behavior change.
  - `ViewMode`/`ViewAll`/`ViewMine` needed to move too since they're stored in `AppState` — they're a domain concept, not a UI concern.
  - `core.go` was incorrectly importing `tui` just for the `StateStore` interface type — fixing the dependency direction also cleaned up core's import graph.
---

## 2026-05-25 - mrr-28q.5
- Added `measureHeight(w int) int` to `cardWidget` in `internal/tui/card.go`
- Updated `SetWidth()` and `SetCards()` in `internal/tui/column.go` to call `measureHeight` instead of `render()`
- Removed `lineCount` helper (no longer used)
- **Files changed**: `internal/tui/card.go`, `internal/tui/column.go`
- **Learnings:**
  - `measureHeight` counts: `5 + nTitleLines + nPillLines` where 5 = line1 + blank + blank + border-top + border-bottom
  - `wrapPills` was kept as the counting helper (it builds pill strings but avoids the expensive full `style.Render()` on the whole card)
  - `lineCount` was only used in layout paths; once both callers switched to `measureHeight`, the linter caught it as unused — safe to delete entirely
---
