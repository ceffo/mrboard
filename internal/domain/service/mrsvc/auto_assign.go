package mrsvc

import (
	"context"

	"github.com/ceffo/mrboard/internal/domain"
)

// autoAssignReviewerStore is the narrow slice of MergeRequestSource that
// AutoAssignReviewers actually calls, declared at this use-case's own site
// per docs/clean_architecture.md §7.3.
type autoAssignReviewerStore interface {
	SetReviewers(ctx context.Context, projectID int64, mrIID int64, userIDs []int64) error
}

// AutoAssignReviewers writes reviewers to a single MR, given the team members
// domain.AutoAssignCandidates has already selected (docs/adr/0009). This is
// the one place, shared by the TUI and mrboard update, that turns an
// eligibility decision into the actual GitLab write — src only ever executes
// the plain write it's told to make.
//
// A nil or empty reviewers list is a no-op: SetReviewers treats an empty ID
// slice as "clear all reviewers," so calling through with nothing to assign
// would silently strip any reviewers already on the MR.
func AutoAssignReviewers(
	ctx context.Context, src autoAssignReviewerStore, projectID, mrIID int64, reviewers []domain.User,
) error {
	if len(reviewers) == 0 {
		return nil
	}
	userIDs := make([]int64, len(reviewers))
	for i, u := range reviewers {
		userIDs[i] = u.ID
	}
	return src.SetReviewers(ctx, projectID, mrIID, userIDs)
}
