package mrsvc

import (
	"context"
	"fmt"

	"github.com/ceffo/mrboard/internal/domain"
)

// autoAssignReviewerStore is the narrow slice of MergeRequestSource that
// AutoAssignReviewers actually calls, declared at this use-case's own site
// per docs/clean_architecture.md §7.3.
type autoAssignReviewerStore interface {
	FetchMR(ctx context.Context, projectID int64, mrIID int64) (domain.MergeRequest, error)
	SetReviewers(ctx context.Context, projectID int64, mrIID int64, userIDs []int64) error
}

// AutoAssignReviewers writes reviewers to a single MR, given the team members
// domain.AutoAssignCandidates has already selected (docs/adr/0009).
//
// Candidates are chosen from a board snapshot that can be stale, and
// SetReviewers replaces the whole reviewer set. So the MR is re-read from the
// source right before writing, and the write only happens when it still has no
// reviewers; assigned reports whether it did. An MR that gained reviewers in
// the meantime is left untouched.
//
// A nil or empty reviewers list is a no-op: SetReviewers treats an empty ID
// slice as "clear all reviewers," so calling through with nothing to assign
// would silently strip any reviewers already on the MR.
func AutoAssignReviewers(
	ctx context.Context, src autoAssignReviewerStore, projectID, mrIID int64, reviewers []domain.User,
) (assigned bool, err error) {
	if len(reviewers) == 0 {
		return false, nil
	}
	live, err := src.FetchMR(ctx, projectID, mrIID)
	if err != nil {
		return false, fmt.Errorf("auto-assign: re-reading MR: %w", err)
	}
	if len(live.Reviewers) != 0 {
		return false, nil
	}
	userIDs := make([]int64, len(reviewers))
	for i, u := range reviewers {
		userIDs[i] = u.ID
	}
	if err := src.SetReviewers(ctx, projectID, mrIID, userIDs); err != nil {
		return false, err
	}
	return true, nil
}
