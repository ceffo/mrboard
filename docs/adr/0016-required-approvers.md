# ADR-0016: An Approver Is a Member of a Rule That Needs Every Member's Approval

**Status**: Accepted. Supersedes the "eligible under any approval rule" meaning of `IsApprover` and
`MergeRequest.Approvers` used by `docs/domain-model.md`, ADR-0001, ADR-0013, ADR-0014 and ADR-0015.

## Context

mrboard called a reviewer an **approver** when they were eligible to approve under any regular
approval rule on the MR, whatever that rule needed. A rule needing one approval out of five people
made all five approvers, though none of them was individually required. Every view built on that
flag over-counted: the Approved column waited on approvals the MR did not need, the load view
charged the MR to all five, and `needs_my_review` asked all five to review.

## Decision

A reviewer is a **required approver** of an MR when they are an eligible approver of a regular
approval rule whose approvals required equals its number of eligible approvers, and is above zero.
Every member of such a rule must approve, so each member's approval is required. Members of any other
rule are not approvers: their approval is optional and mrboard ignores it.

| Rule | Its members are approvers |
| --- | --- |
| needs N approvals, N eligible approvers, N > 0 | yes |
| needs fewer approvals than it has eligible approvers (e.g. 1 of 5) | no |
| needs more approvals than it has eligible approvers | no |
| needs no approval (0) | no |

A user in a required rule and an optional rule is an approver (the required rule decides).
`MergeRequest.Approvers` is the union of the members of every required rule; `ReviewerInfo.IsApprover`
flags the reviewers among them. The names stay; only their meaning changes, everywhere at once: board
phase and card markers, detail view, load view, `RolesOf`/`NeedsAttention`/`NeedsReviewBy`,
`mrboard fetch` (`is_approver`, `approvals_required`), `mrboard load`, and the approver set the
announcement ledger records and compares against the live MR (ADR-0014).

- **One owner.** `domain.ApproverRule.RequiresEveryMember`, `domain.RequiredApprovers` and
  `MergeRequest.ApplyApproverRules` hold the rule. The REST, GraphQL (fat, thin, cached-merge and
  merged-MR) mappers, the announcement claim's live read, the demo adapter and
  `SetManagedApprovers` all derive through them; none re-implements the test.
- **Data.** `ApproverRule.ApprovalsRequired` carries the rule's count: REST `approvals_required`,
  GraphQL `approvalsRequired` on `approvalState.rules` (selected by every MR query).
- **The managed rule** (`approvers_mrboard`, ADR-0002) is written with approvals required equal to
  its member count, so its members are always required approvers.
- **Snapshots.** The board snapshot and merged snapshot formats move to version 2, so a file holding
  the old meaning is a cold cache (one full fetch) instead of being shown until the next refresh.

## Consequences

- An MR whose only rules are optional has no approvers: like an MR without rules, every reviewer
  counts and the MR never reaches the Approved column (phase rule 2 in `docs/domain-model.md`).
- A rule GitLab lists with fewer eligible approvers than it needs (for example because the author
  is excluded from approving) makes no one required, even if the project meant it to.
- **Locked approvers follow the same meaning.** The approver editor locks (`🔒 <rule>`) only the
  members of an unmanaged rule that requires every member (`LockedApprovers`). A member of an
  optional rule is an ordinary row: ticking them adds them to `approvers_mrboard`, which makes them
  a required approver.
- An MR whose announcement ledger recorded an old-meaning set that included optional members sees a
  different set after the upgrade; the announcement rules of ADR-0014 decide what that produces.
