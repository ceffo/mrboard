# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

*Add reusable patterns discovered during development here.*

---

## 2026-06-29 - mrr-0bw.1
- Implementation was already complete from a prior session (matched the stop condition)
- `GetActiveSprintIssueKeys` in `internal/adapters/jiraadpt/jiraadpt.go:92-116`: two-step call — `GetActiveSprint(boardID)` → `GetSprintIssueKeys(sprintID)` — result cached under `sprint_board_<boardID>.json`
- Port `jirasvc.JiraEnricher` in `internal/domain/service/jirasvc/jirasvc.go` already declares the method
- Tests in `jiraadpt_test.go`: `TestGetActiveSprintIssueKeys_LiveAndCached` and `TestGetActiveSprintIssueKeys_NoActiveSprint`
- `just check` passes: 0 lint issues, clean build, all tests green
- **Learnings:**
  - Board-scoped cache key (not sprint-scoped) avoids needing to know the sprint ID before the cache lookup — the two API calls are bundled under one cache entry
  - `nil` sprint (no active sprint) returns `(nil, nil)` — not an error, callers must handle the nil slice explicitly
  - `fakeClient` in tests implements all three `jiraClient` interface methods with a `calls` counter for verifying cache hit/miss behavior

---

## 2026-06-29 - mrr-0bw.2
- Added `SprintIssueKeysMsg` message type to `internal/tui/model.go`
- Added `sprintIssueKeys map[string]bool` and `sprintFilterActive bool` fields to `Model` struct
- Added `makeSprintFetchCmd` (fires `GetActiveSprintIssueKeys`; gated by `jiraEnricher != nil && cfg.Jira.BoardID != 0`)
- Fired sprint fetch in `Init()` alongside existing cmds; `nil` cmd in `tea.Batch` is safe
- Added `handleSprintIssueKeys` handler: stores keys as `map[string]bool`, calls `applyMRFilter`
- Dispatches `SprintIssueKeysMsg` in `coreUpdate`
- Extended `mrsvc.FilterOptions` with `SprintFilter bool` + `SprintKeys map[string]bool`
- `FilterAndSort` applies sprint filter after reviewers filter; guarded by `SprintFilter && len(SprintKeys) > 0`
- `isFilterActive` now includes `sprintFilterActive`
- 3 new tests in `filter_test.go` covering: filter on, filter off, nil keys passthrough
- **Learnings:**
  - `goconst` counts struct literal field values across the package — avoid `"repo/a"` / `"repo/b"` in new sprint tests (already at threshold from existing tests using the `mr()` helper)
  - Sprint key set uses `nil` (not empty map) as sentinel for "no active sprint"; `len(nil map) == 0` so the filter guard is naturally inert
  - `tea.Batch(nil)` is valid — no need to conditionally omit from Init when board_id is 0

---

## 2026-06-29 - mrr-qtk.1
- Added `jiraIndex map[string][]domain.MergeRequest` field to `Model` struct
- Added `buildJiraIndex()` private method — iterates `allMRs`, calls `domain.ExtractJiraID`, populates map; MRs with no JIRA key are skipped
- Added `SiblingMRs(issueKey string) []domain.MergeRequest` public method — returns nil for empty key
- Wired `buildJiraIndex()` as the first call in `applyMRFilter()` — ensures index is always consistent with `allMRs` since applyMRFilter is called after every mutation
- Added 3 tests in `model_test.go`: index builds on fetch (siblings grouped), empty key returns nil, no JIRA keys produces empty index
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`
- **Learnings:**
  - Index is built from `allMRs` (not the filtered view) so siblings hidden by sprint/reviewer-source filters are still findable by the batch editor
  - `applyMRFilter()` is the canonical "allMRs changed, recompute" hook — no need to call `buildJiraIndex` separately after individual `allMRs[i]` mutations since `applyMRFilter` follows each one
  - `goconst` string literal threshold: used package-level `const jiraKeyAlpha/jiraKeyBeta` in tests to stay safe (JIRA keys like "OD-100" appear twice in test table data)

---

## 2026-06-29 - mrr-0bw.3
- Added `Sprint key.Binding` to `KeyMap` struct and `DefaultKeyMap` (uppercase `S`); added to `ShortHelp()` and wrapped to stay within 120-char lint limit
- Disabled Sprint key at construction time in `New()` when `cfg.Jira.BoardID == 0` — same pattern as `Notify`/`Jira` keys
- Added `case key.Matches(msg, m.keys.Sprint):` in `handleKeyBoard` — toggles `sprintFilterActive`, calls `applyMRFilter` (no `saveState` needed; sprint state is ephemeral)
- Added `sprintFilterActive bool` field and `SetSprintFilterActive(v bool)` setter to `headerWidget`; `render()` appends `[sprint]` badge using `FilterActive` style when active
- Wired `header.SetSprintFilterActive(m.sprintFilterActive)` in `applyMRFilter` so badge always reflects current state
- Files changed: `internal/tui/keys.go`, `internal/tui/model.go`, `internal/tui/header.go`
- **Learnings:**
  - `ShortHelp()` line-length: adding one more binding to an already-long return pushed the line over 120 chars (`lll` lint) — split across two lines grouped conceptually (navigation vs actions)
  - Sprint toggle state is ephemeral (not persisted via `saveState`) — no sprint ID is available client-side, and the sprint keys are already re-fetched on every startup
  - The `[sprint]` badge renders independently of `[filtered]`; both can appear simultaneously if other filters are also active — clean separation of concerns

---

## 2026-06-29 - mrr-qtk.2
- Created `internal/tui/batch_reviewer_editor.go`: `batchReviewerEditorWidget` with `batchReviewerEditorPanel` (reviewers/siblings), pre-fill from focused MR, Tab panel switching, ↑/↓ navigation, space toggle approver, d remove, Esc close
- Added `BatchEdit key.Binding` (key `E`) to `KeyMap` + `ShortHelp`; added `BatchReviewerEditorKeyMap` + `DefaultBatchReviewerEditorKeyMap` to `keys.go`
- Added `overlayKindBatchReviewerEditor` to `overlay_router.go`
- Wired in `model.go`: new fields (`batchReviewerEditor`, `batchReviewerEditorKeys`), E key handler in `handleKeyBoard`, overlay dispatch in `handleKey`, `overlayKindBatchReviewerEditor` render case, `applyTheme` propagation
- Merged `ReviewerEditorClosedMsg` and `BatchReviewerEditorClosedMsg` into a single type-switch case to keep `gocyclo` complexity ≤ 30
- Files changed: `internal/tui/batch_reviewer_editor.go` (new), `internal/tui/keys.go`, `internal/tui/overlay_router.go`, `internal/tui/model.go`
- **Learnings:**
  - Adding a new overlay case to `coreUpdate` would have pushed `gocyclo` from 30→31 — merging the two "closed" messages into one `case T1, T2:` is the right fix when semantics are identical
  - Formatter re-aligns struct field columns; adding `batchReviewerEditorKeys` widened the longest field name, pushing an adjacent comment over 120 chars — shortening the comment is the correct fix
  - `SiblingMRs("")` returns nil when the focused MR has no JIRA key; this propagates naturally to `len(siblings) == 0`, rendering `(no sibling MRs)` without any special-casing at the call site

---
