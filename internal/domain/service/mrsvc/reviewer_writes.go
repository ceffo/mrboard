package mrsvc

import (
	"context"
	"fmt"

	"github.com/ceffo/mrboard/internal/domain"
)

// ReviewerEdit is one entry in a staged reviewer/approver edit, as prepared by a
// caller (e.g. the TUI's reviewer editor). UserID is 0 when not yet resolved.
type ReviewerEdit struct {
	Username   string
	IsApprover bool
	UserID     int64
}

// reviewerStore is the narrow slice of MergeRequestSource that
// ApplyReviewerChanges actually calls. Declared at this use-case's own site
// per docs/clean_architecture.md §7.3 — callers pass their MergeRequestSource
// (or any narrower interface satisfying these four methods) directly.
type reviewerStore interface {
	FetchMR(ctx context.Context, projectID int64, mrIID int64) (domain.MergeRequest, error)
	GetProjectMembers(ctx context.Context, projectID int64) ([]domain.ProjectMember, error)
	SaveApprovers(ctx context.Context, projectID int64, mrIID int64, userIDs []int64) error
	SetReviewers(ctx context.Context, projectID int64, mrIID int64, userIDs []int64) error
}

// ApplyReviewerChanges writes a staged reviewer/approver edit to a single MR.
//
// It resolves any unresolved user IDs (UserID == 0 and not present in knownIDs) via
// one GetProjectMembers call, replaces the MR's reviewer set unconditionally via
// SetReviewers, and writes the "Approvers" rule via SaveApprovers only when the
// resulting approver set differs from currentApprovers. It then refetches the MR and
// overlays the just-written approver flags onto it, since GitLab's approval-rule read
// is eventually consistent and a fetch fired immediately after SaveApprovers can
// return stale IsApprover flags.
//
// knownIDs may be pre-populated with already-resolved usernames to skip a redundant
// GetProjectMembers call; pass an empty map when none are known yet. The caller owns
// the map and must not read it concurrently — ApplyReviewerChanges mutates it in place
// with any IDs it resolves.
func ApplyReviewerChanges(
	ctx context.Context,
	src reviewerStore,
	projectID, mrIID int64,
	staged []ReviewerEdit,
	knownIDs map[string]int64,
	currentApprovers map[string]bool,
) (mr domain.MergeRequest, approversChanged bool, err error) {
	needFetch := false
	for _, s := range staged {
		if s.UserID == 0 {
			if _, ok := knownIDs[s.Username]; !ok {
				needFetch = true
				break
			}
		}
	}
	if needFetch {
		members, mErr := src.GetProjectMembers(ctx, projectID)
		if mErr != nil {
			return domain.MergeRequest{}, false, fmt.Errorf("resolve reviewer IDs: %w", mErr)
		}
		for _, m := range members {
			knownIDs[m.Username] = m.UserID
		}
	}

	seen := make(map[int64]bool)
	var reviewerIDs []int64
	for _, s := range staged {
		id := s.UserID
		if id == 0 {
			id = knownIDs[s.Username]
		}
		if id == 0 || seen[id] {
			continue
		}
		reviewerIDs = append(reviewerIDs, id)
		seen[id] = true
	}

	if sErr := src.SetReviewers(ctx, projectID, mrIID, reviewerIDs); sErr != nil {
		return domain.MergeRequest{}, false, sErr
	}

	nowApprovers := make(map[string]bool)
	var approverIDs []int64
	for _, s := range staged {
		if s.IsApprover {
			nowApprovers[s.Username] = true
			id := s.UserID
			if id == 0 {
				id = knownIDs[s.Username]
			}
			if id != 0 {
				approverIDs = append(approverIDs, id)
			}
		}
	}
	approversChanged = len(nowApprovers) != len(currentApprovers)
	if !approversChanged {
		for u := range nowApprovers {
			if !currentApprovers[u] {
				approversChanged = true
				break
			}
		}
	}
	if approversChanged {
		if aErr := src.SaveApprovers(ctx, projectID, mrIID, approverIDs); aErr != nil {
			return domain.MergeRequest{}, approversChanged, aErr
		}
	}

	mr, err = src.FetchMR(ctx, projectID, mrIID)
	if err == nil {
		ApplyStagedApproverFlags(&mr, nowApprovers)
	}
	return mr, approversChanged, err
}

// ApplyStagedApproverFlags overlays the just-written approver set onto the MR's
// reviewers. GitLab's approval-rule read is eventually consistent, so a fetch fired
// immediately after SaveApprovers can return stale EligibleApprovers and drop the
// IsApprover flag. Trusting the staged intent instead keeps callers correct until the
// next full refresh.
func ApplyStagedApproverFlags(mr *domain.MergeRequest, approvers map[string]bool) {
	for i := range mr.Reviewers {
		mr.Reviewers[i].IsApprover = approvers[mr.Reviewers[i].Username]
	}
}
