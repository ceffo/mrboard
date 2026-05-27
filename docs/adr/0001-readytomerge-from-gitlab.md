# ADR-0001: PhaseReadyToMerge driven by GitLab's detailed_merge_status

**Status**: Accepted

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
