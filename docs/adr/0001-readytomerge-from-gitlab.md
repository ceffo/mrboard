# ADR-0001: PhaseReadyToMerge driven by GitLab's detailed_merge_status

**Status**: Superseded (see [Decision: reverting to approver-based gating](#decision-reverting-to-approver-based-gating-2026-05-27) below)

## Context

`ClassifyPhase` previously computed `PhaseReadyToMerge` locally: `openThreads == 0 && approvalCount >= requiredApprovals`. This works for simple repos but misses project-level rules that mrboard can't know about: CI requirements, branch protection, external status checks, additional approval rules.

## Decision

Map `detailed_merge_status == "mergeable"` from the GitLab API to `PhaseReadyToMerge`. All other phases (Draft, NeedsAuthorAction, NeedsReview) remain locally computed from discussion events.

`ClassifyPhase` signature changes from `(draft bool, openThreads, approvalCount, requiredApprovals int, reviewers []ReviewerInfo)` to `(draft bool, mergeable bool, reviewers []ReviewerInfo)`.

The `detailed_merge_status` field must be present in both the REST response and the GQL query.

## Consequences

- `PhaseReadyToMerge` is now authoritative for all project configurations, not just the common case.
- `ApprovalCount` and `RequiredApprovals` are removed from `MergeRequest` — they were only needed to drive this computation.
- The GQL query must add `detailedMergeStatus` to stay in sync with the REST path.

## Decision: reverting to approver-based gating (2026-05-27)

`detailed_merge_status` proved too coarse: a project's merge status can depend on rules mrboard
has no way to distinguish from "still needs review" — CI still running, an unrelated branch
conflict — so a card could sit outside the Approved column for reasons that have nothing to do
with review state, the thing this board exists to track.

`ClassifyPhase` (`internal/domain/mr.go`) now enters `PhaseReadyToMerge` — renamed "Approved" in
the TUI — when every reviewer flagged `IsApprover` has approved. An MR with no designated
approvers stays in `NeedsReview` regardless of plain-reviewer approvals. See
`docs/domain-model.md`'s phase classification rules for the current logic.

`detailed_merge_status` is kept on `MergeRequest` (`DetailedMergeStatus`) but no longer drives
phase assignment — it only tints an Approved card's title green (mergeable) or red (blocked),
so the distinction this ADR originally cared about (CI, branch protection, other project rules)
is still visible, just as a signal layered on top of the approver-based column rather than the
gate for it.
