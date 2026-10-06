# ADR-0013: "My View" Shows Everything That Concerns the User; Attention Is a Separate Highlight

**Status**: Accepted — supersedes the reviewer-gating rule of the earlier My View predicate

## Context

My View answered two different questions with one predicate: *does this MR belong in my view?* and
*is the ball in my court?* The predicate admitted an MR only when the user was the assignee, the
author with no approvers assigned, or a reviewer still owing a review (and, once approvers existed,
only as an approver).

The conflation hid the user's own work. An MR the user authored, with approvers assigned and no
explicit assignee, vanished from the board while it waited on those approvers — exactly when the
author most wants to see it. Fixing it inside the predicate would have meant widening "relevant"
until it no longer meant "needs me", and the board would have lost the signal it was built for.

## Decision

Split the two questions into two pure domain rules in `internal/domain/attention.go`.

**Visibility — `MergeRequest.Concerns(user)`.** The MR is shown if the user is its author, its
assignee, or in its reviewer list in any state, approver or not. Visibility never depends on phase
or reviewer state, so an MR the user owns cannot disappear while it waits on others.

**Attention — `MergeRequest.NeedsAttention(user)`.** The ball is in the user's court:

| Role | Needs attention when |
|---|---|
| Author or assignee | phase is `NeedsAuthorAction` (a reviewer commented) or `ReadyToMerge` |
| Reviewer | their own state is `NotStarted` or `ReReviewRequested` |

Two gates apply to both roles:

- A **draft** never needs attention — review has not been asked for yet.
- When the MR designates approvers, only an approver reviewer is prompted. A plain reviewer still
  sees the MR but is not highlighted, consistent with basic reviewers not being held to a review
  SLA (see `docs/domain-model.md`).

`MergeRequest.RolesOf(user)` returns the user's roles on the MR (`author`, `assignee`, `reviewer`)
and backs `Concerns`.

**Rendering.** An attention card gets a warning-colored border (`Styles.CardAttention`), applied
only while the card is unfocused so focus styling keeps priority. The highlight is independent of
the view mode: it also appears in the all-MRs view. The board computes it from `current_user`;
without one nothing is highlighted.

**`mrboard fetch`.** The JSON gains `roles` and `needs_attention`, relative to `current_user`, and
`--filtered` / `--view mine|all` apply the saved board filters, so both rules can be checked
against real data without the TUI.

## Non-goals

- **No reason tag on the card.** The column already says why the ball is where it is (Needs
  Review, Needs Author Action, Approved); a tag would repeat it.
- **No header counter yet.** ADR-0012 reserves the header's left zone for filter state, and
  attention is not a filter. A count belongs there only if a clear place for it is decided first.
- **Sprint filtering is not part of `fetch --filtered`.** It needs Jira data the command does not
  load.
