# ADR-0008: One Reviewer-Write Use Case for Single-Edit and Batch Apply

**Status**: Accepted

## Context

The reviewer editor writes a staged edit to GitLab through `mrsvc.ApplyReviewerChanges` from two
call sites: `reviewerEditorWidget.saveCmd` (single MR, no siblings) and, after the batch-preview
screen, one call per target sibling MR. Both rebuilt the same three inputs —
the `[]stagedReviewer` → `[]mrsvc.ReviewerEdit` conversion, the `knownIDs` seed, and
`origApprovers` — independently, and they diverged: `saveCmd` seeded `knownIDs` from the editor's
`userIDByName` (already resolved via the earlier project-members fetch), while the batch path
always started from an empty map. A staged reviewer whose `UserID` wasn't already resolved on the
edit itself (in practice: an existing reviewer the editor's async member fetch hadn't caught up
with yet) forced a redundant `GetProjectMembers` call per batch target, even when the focused
editor had already resolved that exact username moments earlier.

## Decision

Introduce a single function, `makeReviewerWriteCmd(base, src, target, staged, knownIDs)` in
`internal/tui/model.go`, as the one place that converts `staged`, copies `knownIDs`, derives
`origApprovers` from `target.Reviewers`, and calls `mrsvc.ApplyReviewerChanges`. Both callers
route through it:

- `saveCmd` calls it with `target: w.mr`, `knownIDs: w.userIDByName`.
- `handleBatchPreviewConfirmed` calls it once per target, threading the *same* `knownIDs` map the
  editor resolved — carried from `reviewerEditorWidget.confirm` through
  `BatchReviewerEditorPreviewMsg.KnownIDs` → `batchPreviewWidget` →
  `BatchPreviewConfirmedMsg.KnownIDs`.

Reusing the editor's resolved IDs across targets is correct, not just convenient: GitLab user IDs
are global to the instance, so an ID resolved against one project's membership is valid when
writing to another target's project. `origApprovers` is derived fresh from `target.Reviewers`
inside the shared function rather than cached on the widget — `reviewerEditorWidget.origApprovers`
was redundant with `w.mr.Reviewers` (never mutated post-construction) and was deleted.

## Consequences

- `knownIDs` divergence between the two write paths is now structurally impossible — there is one
  place that builds it, not two that can drift.
- A batch apply across siblings sharing the *same* project reuses the editor's own member
  resolution instead of re-fetching per target.
- Any future reviewer-write caller (a new keybinding, say) gets correct-by-construction setup by
  calling `makeReviewerWriteCmd` — no setup to duplicate or diverge.
