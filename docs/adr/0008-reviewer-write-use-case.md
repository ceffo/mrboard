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

Introduce a single function, `makeReviewerWriteCmd(base, src, target, staged, baseline, mode,
knownIDs)` in `internal/tui/model.go`, as the one place that converts `staged` and `baseline`,
copies `knownIDs`, and calls `mrsvc.ApplyReviewerChanges`. Both callers route through it:

- `saveCmd` calls it with `target: w.mr`, `knownIDs: w.userIDByName`.
- `handleBatchPreviewConfirmed` calls it once per target, threading the *same* `knownIDs` map the
  editor resolved — carried from `reviewerEditorWidget.confirm` through
  `BatchReviewerEditorPreviewMsg.KnownIDs` → `batchPreviewWidget` →
  `BatchPreviewConfirmedMsg.KnownIDs`.

Reusing the editor's resolved IDs across targets is correct, not just convenient: GitLab user IDs
are global to the instance, so an ID resolved against one project's membership is valid when
writing to another target's project. The target's current approvers are read from the MR as it is
on GitLab at write time, not cached on the widget — see the amendment below.

## Consequences

- `knownIDs` divergence between the two write paths is now structurally impossible — there is one
  place that builds it, not two that can drift.
- A batch apply across siblings sharing the *same* project reuses the editor's own member
  resolution instead of re-fetching per target.
- Any future reviewer-write caller (a new keybinding, say) gets correct-by-construction setup by
  calling `makeReviewerWriteCmd` — no setup to duplicate or diverge.

## Amendment (2026-10-05): a reviewer write is a delta on live state, never a replace

Users reported reviewers changing on MRs that already had them. `SetReviewers` replaces the whole
set, and every writer decided what to write from the board snapshot — which can be a refresh
interval old, or older when the board booted from the cache. Anything the snapshot did not list
was stripped from the MR on the next write.

`mrsvc.ApplyReviewerChanges` now takes a `ReviewerChange{Staged, Baseline, Mode}` and **re-reads the
MR before writing**, applying the change to what GitLab reports (`mrsvc.ResolveReviewerSet`):

- `ReviewerWriteEdit`, for the MR the edit was made on, applies only what the user changed between
  `Baseline` (the list the editor opened on) and `Staged`: reviewers added or removed, approver
  flags flipped. Reviewers who joined the MR since the editor opened stay, and one removed
  meanwhile is not resurrected. `T` (set team) and search only ever add to `Staged`, so on the
  focused MR they can never drop anyone.
- `ReviewerWriteUnion`, for every other target of a batch apply, only adds: staged reviewers join,
  staged approvers become approvers, and nobody is removed and no approver flag is cleared.
  `handleBatchPreviewConfirmed` picks the mode by comparing each target with
  `BatchPreviewConfirmedMsg.FocusedMR`.

A part that would not change is not written: `SetReviewers` only when the reviewer set differs,
`SaveApprovers` only when the approver set does, and when neither does the live MR is returned
without a second fetch. A reviewer already on the MR whose user ID cannot be resolved fails the
write instead of being silently left out of the replacement set. This replaces `origApprovers`,
which the caller used to pass in from its snapshot.

Applying to related MRs is **opt-in**: siblings start unchecked in the batch preview, and their
diffs show only additions (`reviewerWriteDiff(..., union=true)`), since a union removes nothing.
The "conflict" warning and `domain.ApproversConflict`/`ApproversDiff` are gone with the replace
semantics they described.

Not closed: there is still a window between the re-read and the write, a few milliseconds wide.
GitLab's API offers no compare-and-set on reviewers, so a teammate adding a reviewer inside it
would still be overwritten.
