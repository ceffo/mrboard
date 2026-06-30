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

