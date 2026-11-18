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
// that would not change: the reviewer set via SetReviewers, the approval
// rules via SaveApprovers. It then refetches the MR and overlays the
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
	if knownIDs == nil {
		knownIDs = make(map[string]int64)
	}
	// The read carries the managed rule's member IDs, so a member who is no
	// longer a project member (and so absent from GetProjectMembers) still
	// resolves and is kept or dropped like anyone else.
	for u, id := range live.ManagedApproverIDs() {
		if _, ok := knownIDs[u]; !ok && id != 0 {
			knownIDs[u] = id
		}
	}

	liveNames := make(map[string]bool, len(live.Reviewers))
	for _, r := range live.Reviewers {
		liveNames[r.Username] = true
	}
	desiredNames := make(map[string]bool, len(desired))
	for _, d := range desired {
		desiredNames[d.Username] = true
	}
	liveManaged := setOf(live.ManagedApprovers())
	managed := resolveManagedApprovers(live, desired)
	reviewersChanged := !sameSet(desiredNames, liveNames)
	approversChanged = !sameSet(managed, liveManaged)
	if !reviewersChanged && !approversChanged {
		return live, false, nil
	}

	managedNames := sortedKeys(managed)
	if err := resolveMissingIDs(ctx, src, projectID, desired, managedNames, knownIDs); err != nil {
		return domain.MergeRequest{}, false, err
	}
	reviewerIDs, err := reviewerIDsOf(desired, liveNames, knownIDs)
	if err != nil {
		return domain.MergeRequest{}, false, err
	}
	approverIDs, err := managedIDsOf(managedNames, liveManaged, desired, knownIDs)
	if err != nil {
		return domain.MergeRequest{}, false, err
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
		mr.SetManagedApprovers(managedNames)
	}
	return mr, approversChanged, err
}

// resolveManagedApprovers returns the members the managed approval rule should
// have once desired is written: the live members, plus every desired reviewer
// flagged approver, minus every desired reviewer not flagged approver and every
// live reviewer the write removes. A reviewer an unmanaged rule makes a
// required approver is never added to the managed rule: they are an approver already.
// Members of the managed rule who are not reviewers stay.
func resolveManagedApprovers(live domain.MergeRequest, desired []ReviewerEdit) map[string]bool {
	managed := setOf(live.ManagedApprovers())
	locked := live.LockedApprovers()
	desiredNames := make(map[string]bool, len(desired))
	for _, d := range desired {
		desiredNames[d.Username] = true
		_, isLocked := locked[d.Username]
		switch {
		case !d.IsApprover:
			delete(managed, d.Username)
		case !isLocked:
			managed[d.Username] = true
		}
	}
	for _, r := range live.Reviewers {
		if !desiredNames[r.Username] {
			delete(managed, r.Username)
		}
	}
	return managed
}

// reviewerIDsOf returns the user IDs of desired, deduplicated, in order. An
// entry without an ID is left out, except a reviewer already on the MR, which
// fails the write rather than being stripped from it.
func reviewerIDsOf(desired []ReviewerEdit, liveNames map[string]bool, knownIDs map[string]int64) ([]int64, error) {
	seen := make(map[int64]bool)
	var ids []int64
	for _, d := range desired {
		id := idOf(d.Username, d.UserID, knownIDs)
		if id == 0 {
			if liveNames[d.Username] {
				return nil, fmt.Errorf(
					"cannot resolve a user ID for existing reviewer @%s; leaving the reviewers unchanged", d.Username)
			}
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

// managedIDsOf returns the user IDs of names, the managed rule's members to
// write. A member without an ID is left out, except one already on the rule,
// which fails the write rather than being dropped from it.
func managedIDsOf(
	names []string, liveManaged map[string]bool, desired []ReviewerEdit, knownIDs map[string]int64,
) ([]int64, error) {
	staged := make(map[string]int64, len(desired))
	for _, d := range desired {
		staged[d.Username] = d.UserID
	}
	ids := make([]int64, 0, len(names))
	for _, u := range names {
		id := idOf(u, staged[u], knownIDs)
		if id == 0 {
			if liveManaged[u] {
				return nil, fmt.Errorf(
					"cannot resolve a user ID for existing approver @%s; leaving the approvers unchanged", u)
			}
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func idOf(username string, id int64, knownIDs map[string]int64) int64 {
	if id != 0 {
		return id
	}
	return knownIDs[username]
}

func setOf(usernames []string) map[string]bool {
	set := make(map[string]bool, len(usernames))
	for _, u := range usernames {
		set[u] = true
	}
	return set
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	return domain.NormalizeApprovers(keys)
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
// UserID nor a knownIDs entry, and every such username in extra, with a single
// GetProjectMembers call.
func resolveMissingIDs(
	ctx context.Context, src reviewerStore, projectID int64, desired []ReviewerEdit, extra []string,
	knownIDs map[string]int64,
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
	staged := make(map[string]bool, len(desired))
	for _, d := range desired {
		staged[d.Username] = true
	}
	for _, u := range extra {
		if _, ok := knownIDs[u]; !ok && !staged[u] {
			needFetch = true
			break
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
