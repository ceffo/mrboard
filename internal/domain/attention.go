package domain

// Role is how a user is connected to a merge request.
type Role string

// Role values.
const (
	RoleAuthor   Role = "author"
	RoleAssignee Role = "assignee"
	RoleReviewer Role = "reviewer"
)

// RolesOf returns every way username is connected to the MR, in the fixed order
// author, assignee, reviewer. A reviewer is someone in the formal reviewer list,
// in any state; when the MR designates approvers, only an approver counts, since
// a plain reviewer is not held to a review obligation. An empty username has no
// roles.
func (mr MergeRequest) RolesOf(username string) []Role {
	if username == "" {
		return nil
	}
	var roles []Role
	if mr.Author == username {
		roles = append(roles, RoleAuthor)
	}
	if mr.Assignee == username {
		roles = append(roles, RoleAssignee)
	}
	if mr.reviewerRole(username) != nil {
		roles = append(roles, RoleReviewer)
	}
	return roles
}

// Concerns reports whether the MR belongs in username's own view: the user is
// its author, assignee, or a counting reviewer (see RolesOf). Visibility is
// deliberately independent of NeedsAttention, so an MR the user owns never
// disappears while it waits on others.
func (mr MergeRequest) Concerns(username string) bool {
	return len(mr.RolesOf(username)) > 0
}

// NeedsAttention reports whether the ball is in username's court.
//
//   - As author or assignee: reviewers have commented (PhaseNeedsAuthorAction)
//     or the MR is approved and awaiting merge (PhaseReadyToMerge).
//   - As reviewer: their own state is NotStarted or ReReviewRequested.
//
// Drafts never need attention: the author has not asked for review yet.
func (mr MergeRequest) NeedsAttention(username string) bool {
	if username == "" || mr.Phase == PhaseDraft {
		return false
	}
	if mr.Author == username || mr.Assignee == username {
		if mr.Phase == PhaseNeedsAuthorAction || mr.Phase == PhaseReadyToMerge {
			return true
		}
	}
	r := mr.reviewerRole(username)
	if r == nil {
		return false
	}
	return r.State == ReviewerNotStarted || r.State == ReviewerReReviewRequested
}

// reviewerRole returns username's reviewer entry, or nil when they have none or
// the MR designates approvers and they are not one of them.
func (mr MergeRequest) reviewerRole(username string) *ReviewerInfo {
	for i := range mr.Reviewers {
		r := &mr.Reviewers[i]
		if r.Username != username {
			continue
		}
		if mr.hasApprovers() && !r.IsApprover {
			return nil
		}
		return r
	}
	return nil
}

func (mr MergeRequest) hasApprovers() bool {
	for _, r := range mr.Reviewers {
		if r.IsApprover {
			return true
		}
	}
	return false
}
