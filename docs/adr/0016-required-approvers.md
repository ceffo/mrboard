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
approvers. Every member of such a rule must approve, so each member's approval is required. Members of
any other rule are not approvers: their approval is optional and mrboard ignores it. The MR's author
is never an approver of their own MR, and is left out of a rule's members before they are counted.

| Rule (members counted without the MR's author) | Its members are approvers |
| --- | --- |
| needs N approvals, N members, N > 0 | yes |
| needs more approvals than it has members (GitLab dropped members who cannot approve) | yes |
| needs fewer approvals than it has members (e.g. 1 of 5) | no |
| needs no approval (0) | no |
| the managed rule (`approvers_mrboard`), whatever it needs | yes |

**Needing more than its members.** GitLab's GraphQL `approvalState.rules` are wrapped rules: their
`eligibleApprovers` drop the author and, when the project prevents it, committers, while
`approvalsRequired` stays as configured. GitLab treats such a rule as one every remaining member must
approve (its `invalid_rule?` case), so mrboard does too.

**The author never counts.** GitLab cannot count an author's own approval when the project prevents
author approval, its default. The REST read (`eligible_approvers` of the raw rule) keeps the author,
the GraphQL read drops them; dropping the author in the domain rule, both from the result and from the
count, makes the two reads give the same approvers.

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
- **The managed rule** (`approvers_mrboard`, ADR-0002) is written with approvals required equal to
  its member count, and its members are always required approvers, whatever count GitLab returns. A
  count lowered by hand in GitLab, or a read that drops a member, never demotes them, so the editor
  shows them ticked and a save never drops them from the rule.
- **Snapshots.** The board snapshot and merged snapshot formats move to version 2, so a file holding
  the old meaning is a cold cache (one full fetch) instead of being shown until the next refresh.

## Consequences

- An MR whose only rules are optional has no approvers: like an MR without rules, every reviewer
  counts and the MR never reaches the Approved column (phase rule 2 in `docs/domain-model.md`).
- **Known limit: committers.** When the project prevents committer approval, GraphQL drops the MR's
  committers from a rule's eligible approvers and REST does not, and REST cannot tell who they are. On
  such an MR, with a rule needing fewer approvals than its full membership, the board (GraphQL) and a
  single-MR refresh or the claim's live read (REST) can disagree on the approvers. This predates the
  required meaning and is not fixed here.
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
