# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

### gocyclo budget management in coreUpdate
`coreUpdate` sits at exactly gocyclo 30 (the limit) after mrr-qtk.2. To add new message cases without exceeding the limit:
1. Extract any inline case handler that has internal `if/&&/||` branching into a `handleXxx(msg)` method — saves as many complexity points as the branches inside it.
2. Merge semantically-identical "close overlay" messages into a multi-type case (`case A, B, C:`).
3. Delegate new cases to handler functions so each case is worth only +1 in coreUpdate.

In mrr-qtk.3: extracted `DetailFetchResultMsg` to `handleDetailFetchResult` (saved 5 points), then added 3 new cases, landing at 28.

---

## 2026-06-30 - mrr-qtk.4
- Implemented idempotent write dispatch in `handleBatchPreviewConfirmed`
  - 0-targets path: closes overlay silently (no write, no toast)
  - N-targets path: fires `makeBatchWriteCmd` per target, each returns `ReviewersSavedMsg`
- Added `makeBatchWriteCmd` package-level function in `model.go`
  - Always fetches `GetProjectMembers` (batch editor never pre-populates `stagedReviewer.UserID`)
  - Calls `SetReviewers` unconditionally for each target
  - Calls `SaveApprovers` only when approver set differs from current `target.Reviewers` state
  - Snapshots `staged` and `origApprovers` at call time to avoid data races in the closure
  - Returns `ReviewersSavedMsg` reusing the existing handler (in-place MR update, toast, optional notify)
- Files changed: `model.go`
- **Learnings:**
  - `stagedReviewer.UserID` is always 0 in the batch editor — the batch editor pre-fills from `domain.ReviewerInfo` which has no UserID. Always `GetProjectMembers` to resolve.
  - `origApprovers` baseline for change detection in the batch case = current `target.Reviewers[*].IsApprover` (no persistent editor state to read from).
  - `makeBatchWriteCmd` mirrors `reviewerEditorWidget.saveCmd()` logic exactly; keeping them in sync is the maintenance burden.
  - Reusing `ReviewersSavedMsg` + `handleReviewersSaved` means each batch target gets its own "Reviewers saved" toast and optional Teams notification — same as single-MR flow.
---

## 2026-06-30 - mrr-qtk.3
- Implemented `batchPreviewWidget` in `internal/tui/batch_preview.go`
  - `previewMRRow` struct: `mr`, `included` (user toggle), `hasChange` (computed at construction)
  - `stagedDiffersFromMR()` compares staged username×IsApprover map vs current `MR.Reviewers`
  - Renders `[x]/[ ]` checkboxes + `✎`/`─` change indicator per row; title shows "N to write"
  - Keys: ↑/↓ nav, Space toggle include, Enter → `BatchPreviewConfirmedMsg`, Esc → `BatchPreviewBackMsg`
- Added `Confirm` key to `BatchReviewerEditorKeyMap` (Enter → emits `BatchReviewerEditorPreviewMsg`)
- Added `overlayKindBatchPreview` to `overlay_router.go`
- Updated `model.go`:
  - New field: `batchPreview *batchPreviewWidget`, `batchPreviewKeys BatchPreviewKeyMap`
  - `handleBatchEditorPreview` — creates preview widget, opens overlay
  - `handleBatchPreviewConfirmed` — closes overlay (dispatch wired in mrr-qtk.4)
  - `BatchPreviewBackMsg` reopens `overlayKindBatchReviewerEditor`
  - Extracted `DetailFetchResultMsg` inline code to `handleDetailFetchResult` to stay within gocyclo=30
- Files changed: `keys.go`, `overlay_router.go`, `batch_reviewer_editor.go`, `batch_preview.go` (new), `model.go`
- **Learnings:**
  - The `included && hasChange` intersection for `collectTargets()` is the right contract: excluded MRs skip write, "no change" MRs skip write. mrr-qtk.4 consumes `BatchPreviewConfirmedMsg.Targets` directly.
  - gocyclo budget: extracting one dense inline case frees enough headroom for multiple new cases. Track the budget explicitly in the patterns section.
---

