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
approval rule that needs at least one approval and at least as many approvals as it has eligible
approvers. Every eligible approver of such a rule must approve, so each one's approval is required.
Eligible approvers of any other rule are not approvers: their approval is optional and mrboard ignores
it. Rules are counted from their eligible approvers exactly as GitLab returns them.

| Rule (counted on its eligible approvers) | Its eligible approvers are approvers |
| --- | --- |
| needs N approvals, N eligible approvers, N > 0 | yes |
| needs more approvals than it has eligible approvers (GitLab left out members who cannot approve) | yes |
| needs fewer approvals than it has eligible approvers (e.g. 1 of 5) | no |
| needs no approval (0) | no |
| the managed rule (`approvers_mrboard`), whatever it needs | yes |

**Eligible approvers, in both reads.** REST `GET …/approval_state` and GraphQL `approvalState.rules`
both return GitLab's wrapped rules (`ApprovalWrappedRule`). Their eligible approvers
(`eligible_approvers`, `eligibleApprovers`) leave out the MR's author when the project prevents author
approval (its default) and the MR's committers when it prevents committer approval, while
`approvals_required` stays as configured. Both reads therefore give the same approvers. GitLab treats a
rule needing more approvals than it has eligible approvers as one every remaining one must approve (its
`invalid_rule?` case), so mrboard does too. An author the project lets approve is eligible like anyone
else and counts: a rule of the author, alice and bob needing 2 makes no one required.

A user in a required rule and an optional rule is an approver (the required rule decides).
`MergeRequest.Approvers` is the union of the members of every required rule; `ReviewerInfo.IsApprover`
flags the reviewers among them. The names stay; only their meaning changes, everywhere at once: board
phase and card markers, detail view, load view, `RolesOf`/`NeedsAttention`/`NeedsReviewBy`,
`mrboard fetch` (`is_approver`, `approvals_required`), `mrboard load`, and the approver set the
announcement ledger records and compares against the live MR (ADR-0014).

- **One owner.** `domain.ApproverRule.RequiredMembers`, `domain.RequiredApprovers` and
  `MergeRequest.ApplyApproverRules` hold the rule. The REST, GraphQL (fat, thin, cached-merge and
  merged-MR) mappers, the announcement claim's live read, the demo adapter and
  `SetManagedApprovers` all derive through them; none re-implements the test.
- **Data.** `ApproverRule.ApprovalsRequired` carries the rule's count: REST `approvals_required`,
  GraphQL `approvalsRequired` on `approvalState.rules` (selected by every MR query).
  `ApproverRule.Approvers` holds the eligible approvers, `ApproverRule.Members` the users added to the
  rule (REST `users`, GraphQL `users`, selected by the open-MR queries), with their IDs when REST read
  them.
- **The managed rule** (`approvers_mrboard`, ADR-0002) is written with approvals required equal to
  its member count. Its eligible approvers are always required approvers, whatever count GitLab
  returns, so a count lowered by hand in GitLab does not demote them. Its members, eligible or not,
  are what `ManagedApprovers` returns: a member GitLab does not let approve this MR (for example a
  member who pushed a commit) is not an approver on the board, but the editor starts them ticked and
  a save writes them back to the rule. Only unticking a member removes them.
- **Snapshots.** The board snapshot and merged snapshot formats move to version 2, so a file holding
  the old meaning is a cold cache (one full fetch) instead of being shown until the next refresh.

## Consequences

- An MR whose only rules are optional has no approvers: like an MR without rules, every reviewer
  counts and the MR never reaches the Approved column (phase rule 2 in `docs/domain-model.md`).
- **Locked approvers follow the same meaning.** The approver editor locks (`🔒 <rule>`) only the
  members of an unmanaged rule that requires every member (`LockedApprovers`). A member of an
  optional rule is an ordinary row: ticking them adds them to `approvers_mrboard`, which makes them
  a required approver.
- **Announcement ledger upgrade.** A ledger written under the old meaning may hold members of optional
  rules, so the first look at it after the upgrade would see a different set and announce the meaning
  change as a removal. Ledger entries are now written as `v2`; `v1` entries are read as legacy
  (`ApproverClaim.Legacy`). A set the board finds on its first look at an MR (`ClaimRequest.Discovered`)
  is not claimed against a ledger whose latest entry is legacy: no note, no announcement. A change the
  board sees happen is claimed and announced as usual, and its entry is written as `v2`. A change made
  while no instance runs, on an MR whose ledger is still legacy, is therefore not announced. mrboard
  versions that read only `v1` do not see `v2` entries, so a fleet mixing versions keeps two ledger views
  until every instance is upgraded.
