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

// ReviewerWriteMode says how a ReviewerChange is applied to an MR.
type ReviewerWriteMode int

const (
	// ReviewerWriteEdit applies the user's own edit of the MR it was made on:
	// what they changed between Baseline and Staged — reviewers added or removed,
	// approver flags flipped — and nothing else. Reviewers who joined the MR
	// since the editor opened stay.
	ReviewerWriteEdit ReviewerWriteMode = iota
	// ReviewerWriteUnion only ever adds: every staged reviewer joins the MR and
	// every staged approver becomes an approver. Nobody is removed and no
	// approver flag is cleared. It is how an edit made elsewhere is propagated
	// to other MRs.
	ReviewerWriteUnion
)

// ReviewerChange is a staged reviewer/approver edit and the way to apply it.
// Baseline is the reviewer list the edit started from; ReviewerWriteEdit
// consults it to tell what the user changed, and ReviewerWriteUnion ignores it.
type ReviewerChange struct {
	Staged   []ReviewerEdit
	Baseline []ReviewerEdit
	Mode     ReviewerWriteMode
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

// ResolveReviewerSet returns the reviewer set a write leaves on an MR whose
// reviewers on GitLab right now are live: live reviewers first, in order, then
// newly added ones. A reviewer in live is only dropped by a ReviewerWriteEdit
// that removes them, and an approver flag is only cleared by one that flips it.
func ResolveReviewerSet(live []domain.ReviewerInfo, change ReviewerChange) []ReviewerEdit {
	result := make([]ReviewerEdit, 0, len(live)+len(change.Staged))
	index := make(map[string]int, len(live)+len(change.Staged))
	for _, r := range live {
		index[r.Username] = len(result)
		result = append(result, ReviewerEdit{Username: r.Username, IsApprover: r.IsApprover})
	}

	baseline := make(map[string]ReviewerEdit, len(change.Baseline))
	if change.Mode == ReviewerWriteEdit {
		for _, b := range change.Baseline {
			baseline[b.Username] = b
		}
	}
	staged := make(map[string]bool, len(change.Staged))
	for _, s := range change.Staged {
		staged[s.Username] = true
		i, isLive := index[s.Username]
		if !isLive {
			if _, wasInBaseline := baseline[s.Username]; wasInBaseline {
				continue // someone else removed them since the editor opened; don't resurrect
			}
			index[s.Username] = len(result)
			result = append(result, s)
			continue
		}
		if s.UserID != 0 {
			result[i].UserID = s.UserID
		}
		if b, wasInBaseline := baseline[s.Username]; wasInBaseline {
			if s.IsApprover != b.IsApprover {
				result[i].IsApprover = s.IsApprover
			}
			continue
		}
		result[i].IsApprover = result[i].IsApprover || s.IsApprover
	}

	if change.Mode != ReviewerWriteEdit {
		return result
	}
	kept := result[:0]
	for _, r := range result {
		if _, wasInBaseline := baseline[r.Username]; wasInBaseline && !staged[r.Username] {
			continue // the user removed this reviewer
		}
		kept = append(kept, r)
	}
	return kept
}

// ApplyReviewerChanges writes a staged reviewer/approver edit to a single MR.
//
// The MR is re-read first and the change is applied to what is on GitLab now
// (see ResolveReviewerSet), never to the possibly stale board snapshot the edit
// was staged against — SetReviewers replaces the whole set, so anything the
// result omits would be stripped from the MR. Nothing is written for a part
// that would not change: the reviewer set via SetReviewers, the "Approvers"
// rule via SaveApprovers. It then refetches the MR and overlays the
// just-written approver flags onto it, since GitLab's approval-rule read is
// eventually consistent and a fetch fired immediately after SaveApprovers can
// return stale IsApprover flags.
//
// Unresolved user IDs (UserID == 0 and not present in knownIDs) are resolved via
// one GetProjectMembers call. A reviewer already on the MR whose ID cannot be
// resolved fails the write rather than being silently left out of the set.
//
// knownIDs may be pre-populated with already-resolved usernames to skip a redundant
// GetProjectMembers call; pass an empty map when none are known yet. The caller owns
// the map and must not read it concurrently — ApplyReviewerChanges mutates it in place
// with any IDs it resolves.
func ApplyReviewerChanges(
	ctx context.Context,
	src reviewerStore,
	projectID, mrIID int64,
	change ReviewerChange,
	knownIDs map[string]int64,
) (mr domain.MergeRequest, approversChanged bool, err error) {
	live, err := src.FetchMR(ctx, projectID, mrIID)
	if err != nil {
		return domain.MergeRequest{}, false, fmt.Errorf("re-read MR before writing reviewers: %w", err)
	}
	desired := ResolveReviewerSet(live.Reviewers, change)

	liveNames := make(map[string]bool, len(live.Reviewers))
	liveApprovers := make(map[string]bool)
	for _, r := range live.Reviewers {
		liveNames[r.Username] = true
		if r.IsApprover {
			liveApprovers[r.Username] = true
		}
	}
	desiredNames := make(map[string]bool, len(desired))
	nowApprovers := make(map[string]bool)
	for _, d := range desired {
		desiredNames[d.Username] = true
		if d.IsApprover {
			nowApprovers[d.Username] = true
		}
	}
	reviewersChanged := !sameSet(desiredNames, liveNames)
	approversChanged = !sameSet(nowApprovers, liveApprovers)
	if !reviewersChanged && !approversChanged {
		return live, false, nil
	}

	if err := resolveMissingIDs(ctx, src, projectID, desired, knownIDs); err != nil {
		return domain.MergeRequest{}, false, err
	}
	seen := make(map[int64]bool)
	var reviewerIDs, approverIDs []int64
	for _, d := range desired {
		id := d.UserID
		if id == 0 {
			id = knownIDs[d.Username]
		}
		if id == 0 {
			if liveNames[d.Username] {
				return domain.MergeRequest{}, false, fmt.Errorf(
					"cannot resolve a user ID for existing reviewer @%s; leaving the reviewers unchanged", d.Username)
			}
			continue
		}
		if d.IsApprover {
			approverIDs = append(approverIDs, id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		reviewerIDs = append(reviewerIDs, id)
	}

	if reviewersChanged {
		if sErr := src.SetReviewers(ctx, projectID, mrIID, reviewerIDs); sErr != nil {
			return domain.MergeRequest{}, false, sErr
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

// sameSet reports whether a and b hold the same keys.
func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// resolveMissingIDs fills knownIDs for every entry of desired that has neither a
// UserID nor a knownIDs entry, with a single GetProjectMembers call.
func resolveMissingIDs(
	ctx context.Context, src reviewerStore, projectID int64, desired []ReviewerEdit, knownIDs map[string]int64,
) error {
	needFetch := false
	for _, d := range desired {
		if d.UserID == 0 {
			if _, ok := knownIDs[d.Username]; !ok {
				needFetch = true
				break
			}
		}
	}
	if !needFetch {
		return nil
	}
	members, err := src.GetProjectMembers(ctx, projectID)
	if err != nil {
		return fmt.Errorf("resolve reviewer IDs: %w", err)
	}
	for _, m := range members {
		knownIDs[m.Username] = m.UserID
	}
	return nil
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
