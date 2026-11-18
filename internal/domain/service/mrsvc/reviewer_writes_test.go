package mrsvc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

const (
	writeTestProjectID int64 = 42
	writeTestMRIID     int64 = 7
)

// fetchedMR builds the MR as FetchMR returns it: its flagged approvers are the
// members of the managed approval rule.
func fetchedMR(reviewers ...domain.ReviewerInfo) domain.MergeRequest {
	mr := domain.MergeRequest{ID: 1, IID: int(writeTestMRIID), ProjectID: int(writeTestProjectID), Reviewers: reviewers}
	var approvers []string
	for _, r := range reviewers {
		if r.IsApprover {
			approvers = append(approvers, r.Username)
		}
	}
	if len(approvers) > 0 {
		mr.SetManagedApprovers(approvers)
	}
	return mr
}

// applyChanges is a thin wrapper around mrsvc.ApplyReviewerChanges fixing the
// project/MR IDs and context, so test bodies only vary what's under test.
func applyChanges(
	src mrsvc.MergeRequestSource, change mrsvc.ReviewerChange, knownIDs map[string]int64,
) (domain.MergeRequest, bool, error) {
	return mrsvc.ApplyReviewerChanges(context.Background(), src, writeTestProjectID, writeTestMRIID, change, knownIDs)
}

// expectLiveMR stubs the re-read ApplyReviewerChanges does before writing.
func expectLiveMR(src *mocks.MockMergeRequestSource, reviewers ...domain.ReviewerInfo) {
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(fetchedMR(reviewers...), nil).Once()
}

// expectRefetch stubs the post-write refetch.
func expectRefetch(src *mocks.MockMergeRequestSource, reviewers ...domain.ReviewerInfo) {
	expectLiveMR(src, reviewers...)
}

func edit(staged ...mrsvc.ReviewerEdit) mrsvc.ReviewerChange {
	return mrsvc.ReviewerChange{Staged: staged}
}

func TestApplyReviewerChanges_ResolvesUnknownIDs(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	expectLiveMR(src)
	src.EXPECT().
		GetProjectMembers(mock.Anything, writeTestProjectID).
		Return([]domain.ProjectMember{{UserID: 101, Username: userAlice}}, nil).
		Once()
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	expectRefetch(src)

	_, _, err := applyChanges(src, edit(mrsvc.ReviewerEdit{Username: userAlice}), map[string]int64{})
	require.NoError(t, err)
}

func TestApplyReviewerChanges_SkipsResolutionWhenIDsKnown(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	// No GetProjectMembers EXPECT() — calling it fails the test (strict mockery mock).
	expectLiveMR(src)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	expectRefetch(src)

	_, _, err := applyChanges(src, edit(mrsvc.ReviewerEdit{Username: userAlice}), map[string]int64{userAlice: 101})
	require.NoError(t, err)
}

func TestApplyReviewerChanges_DedupesReviewerIDs(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	expectLiveMR(src)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	expectRefetch(src)

	staged := edit(
		mrsvc.ReviewerEdit{Username: userAlice, UserID: 101},
		mrsvc.ReviewerEdit{Username: "alice-alias", UserID: 101},
	)
	_, _, err := applyChanges(src, staged, map[string]int64{})
	require.NoError(t, err)
}

func TestApplyReviewerChanges_WritesNothingWhenNothingChanges(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	// Neither SetReviewers nor SaveApprovers is expected, and no refetch follows.
	expectLiveMR(src, domain.ReviewerInfo{Username: userAlice, IsApprover: true})

	staged := edit(mrsvc.ReviewerEdit{Username: userAlice, UserID: 101, IsApprover: true})
	staged.Baseline = staged.Staged
	mr, changed, err := applyChanges(src, staged, map[string]int64{userAlice: 101})

	require.NoError(t, err)
	assert.False(t, changed, "approversChanged should be false when the set is unchanged")
	assert.Len(t, mr.Reviewers, 1)
}

func TestApplyReviewerChanges_CallsSaveApproversWhenChanged(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	expectLiveMR(src)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	expectRefetch(src)

	staged := edit(mrsvc.ReviewerEdit{Username: userAlice, UserID: 101, IsApprover: true})
	_, changed, err := applyChanges(src, staged, map[string]int64{})
	require.NoError(t, err)
	assert.True(t, changed, "approversChanged should be true when the set grows from empty")
}

func TestApplyReviewerChanges_OverlaysStagedApproverFlagsAfterFetch(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	expectLiveMR(src)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	// The refetch returns stale data — GitLab's approval-rule read hasn't caught up yet.
	expectRefetch(src, domain.ReviewerInfo{Username: userAlice, IsApprover: false})

	staged := edit(mrsvc.ReviewerEdit{Username: userAlice, UserID: 101, IsApprover: true})
	mr, _, err := applyChanges(src, staged, map[string]int64{})
	require.NoError(t, err)
	assert.True(t, mr.Reviewers[0].IsApprover, "staged approver intent should overlay the stale fetch result")
}

func TestApplyReviewerChanges_EditKeepsReviewersAddedByOthers(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	// carol joined the MR after the editor opened on {alice}; the edit only adds bob.
	expectLiveMR(src, domain.ReviewerInfo{Username: userAlice}, domain.ReviewerInfo{Username: userCarol})
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 103, 102}).Return(nil).Once()
	expectRefetch(src)

	alice := mrsvc.ReviewerEdit{Username: userAlice, UserID: 101}
	change := mrsvc.ReviewerChange{
		Staged:   []mrsvc.ReviewerEdit{alice, {Username: userBob, UserID: 102}},
		Baseline: []mrsvc.ReviewerEdit{alice},
		Mode:     mrsvc.ReviewerWriteEdit,
	}
	_, _, err := applyChanges(src, change, map[string]int64{userCarol: 103})
	require.NoError(t, err)
}

func TestApplyReviewerChanges_EditAppliesRemoval(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	expectLiveMR(src, domain.ReviewerInfo{Username: userAlice}, domain.ReviewerInfo{Username: userBob})
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	expectRefetch(src)

	alice := mrsvc.ReviewerEdit{Username: userAlice, UserID: 101}
	change := mrsvc.ReviewerChange{
		Staged:   []mrsvc.ReviewerEdit{alice},
		Baseline: []mrsvc.ReviewerEdit{alice, {Username: userBob, UserID: 102}},
		Mode:     mrsvc.ReviewerWriteEdit,
	}
	_, _, err := applyChanges(src, change, map[string]int64{})
	require.NoError(t, err)
}

func TestApplyReviewerChanges_UnionNeverRemovesReviewersOrApprovers(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	expectLiveMR(src,
		domain.ReviewerInfo{Username: userAlice, IsApprover: true},
		domain.ReviewerInfo{Username: userCarol},
	)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 103, 102}).Return(nil).Once()
	// alice stays an approver although the staged edit doesn't flag her; bob becomes one.
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 102}).Return(nil).Once()
	expectRefetch(src)

	change := mrsvc.ReviewerChange{
		Staged: []mrsvc.ReviewerEdit{
			{Username: userAlice, UserID: 101},
			{Username: userBob, UserID: 102, IsApprover: true},
		},
		Mode: mrsvc.ReviewerWriteUnion,
	}
	_, changed, err := applyChanges(src, change, map[string]int64{userCarol: 103})
	require.NoError(t, err)
	assert.True(t, changed)
}

func TestApplyReviewerChanges_UnionOfSubsetWritesNothing(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t) // no write EXPECT() — nothing would be added
	expectLiveMR(src, domain.ReviewerInfo{Username: userAlice}, domain.ReviewerInfo{Username: userBob})

	change := mrsvc.ReviewerChange{
		Staged: []mrsvc.ReviewerEdit{{Username: userAlice, UserID: 101}},
		Mode:   mrsvc.ReviewerWriteUnion,
	}
	_, changed, err := applyChanges(src, change, map[string]int64{userBob: 102})
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestApplyReviewerChanges_UnresolvableExistingReviewerFailsWithoutWriting(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t) // no SetReviewers EXPECT() — dropping carol would strip her
	expectLiveMR(src, domain.ReviewerInfo{Username: userCarol})
	src.EXPECT().GetProjectMembers(mock.Anything, writeTestProjectID).Return(nil, nil).Once()

	change := mrsvc.ReviewerChange{
		Staged: []mrsvc.ReviewerEdit{{Username: userAlice, UserID: 101}},
		Mode:   mrsvc.ReviewerWriteUnion,
	}
	_, _, err := applyChanges(src, change, map[string]int64{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), userCarol)
}

func TestApplyReviewerChanges_PropagatesReadError(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t) // no write EXPECT() — an unread MR must not be written
	readErr := errors.New("boom")
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(domain.MergeRequest{}, readErr).Once()

	_, _, err := applyChanges(src, edit(mrsvc.ReviewerEdit{Username: userAlice, UserID: 101}), map[string]int64{})
	require.ErrorIs(t, err, readErr)
}

func TestApplyReviewerChanges_PropagatesResolveError(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	expectLiveMR(src)
	src.EXPECT().GetProjectMembers(mock.Anything, writeTestProjectID).Return(nil, errors.New("boom")).Once()

	_, _, err := applyChanges(src, edit(mrsvc.ReviewerEdit{Username: userAlice}), map[string]int64{})
	require.Error(t, err, "expected error to propagate")
}

func TestApplyReviewerChanges_PropagatesSetReviewersError(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	expectLiveMR(src)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).
		Return(errors.New("boom")).Once()

	_, _, err := applyChanges(src, edit(mrsvc.ReviewerEdit{Username: userAlice, UserID: 101}), map[string]int64{})
	require.Error(t, err, "expected error to propagate")
}

func TestResolveReviewerSet_EditDoesNotResurrectReviewerRemovedByOthers(t *testing.T) {
	alice := mrsvc.ReviewerEdit{Username: userAlice}
	bob := mrsvc.ReviewerEdit{Username: userBob}
	change := mrsvc.ReviewerChange{
		Staged:   []mrsvc.ReviewerEdit{alice, bob},
		Baseline: []mrsvc.ReviewerEdit{alice, bob},
		Mode:     mrsvc.ReviewerWriteEdit,
	}

	got := mrsvc.ResolveReviewerSet([]domain.ReviewerInfo{{Username: userAlice}}, change)

	assert.Equal(t, []mrsvc.ReviewerEdit{alice}, got, "bob was removed on GitLab after the editor opened")
}

func TestResolveReviewerSet_EditFlipsApproverFlagBothWays(t *testing.T) {
	change := mrsvc.ReviewerChange{
		Staged: []mrsvc.ReviewerEdit{
			{Username: userAlice, IsApprover: false},
			{Username: userBob, IsApprover: true},
		},
		Baseline: []mrsvc.ReviewerEdit{
			{Username: userAlice, IsApprover: true},
			{Username: userBob, IsApprover: false},
		},
		Mode: mrsvc.ReviewerWriteEdit,
	}
	live := []domain.ReviewerInfo{{Username: userAlice, IsApprover: true}, {Username: userBob}}

	got := mrsvc.ResolveReviewerSet(live, change)

	assert.Equal(t, []mrsvc.ReviewerEdit{
		{Username: userAlice, IsApprover: false},
		{Username: userBob, IsApprover: true},
	}, got)
}

// liveWithForeignRule is an MR whose managed rule holds bob, and whose
// project-sourced rule makes alice (a reviewer) and dave (not a reviewer)
// approvers.
func liveWithForeignRule() domain.MergeRequest {
	mr := fetchedMR(
		domain.ReviewerInfo{Username: userAlice},
		domain.ReviewerInfo{Username: userBob},
		domain.ReviewerInfo{Username: userCarol},
	)
	mr.ApproverRules = []domain.ApproverRule{
		{Name: "orekit-stelar", Approvers: []string{userAlice, userDave}, ApprovalsRequired: 2},
	}
	mr.SetManagedApprovers([]string{userBob})
	return mr
}

func TestApplyReviewerChanges_NeverCopiesLockedApproversIntoTheManagedRule(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(liveWithForeignRule(), nil).Once()
	// alice stays an approver through orekit-stelar; only bob and carol are mrboard's.
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{102, 103}).Return(nil).Once()
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(liveWithForeignRule(), nil).Once()

	before := []mrsvc.ReviewerEdit{
		{Username: userAlice, UserID: 101, IsApprover: true},
		{Username: userBob, UserID: 102, IsApprover: true},
		{Username: userCarol, UserID: 103},
	}
	after := append([]mrsvc.ReviewerEdit(nil), before...)
	after[2].IsApprover = true
	mr, changed, err := applyChanges(src,
		mrsvc.ReviewerChange{Staged: after, Baseline: before, Mode: mrsvc.ReviewerWriteEdit}, map[string]int64{})

	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, []string{userBob, userCarol}, mr.ManagedApprovers(), "the refetch is overlaid with the write")
	assert.Equal(t, []string{userAlice, userBob, userCarol, userDave}, mr.Approvers)
	for _, r := range mr.Reviewers {
		assert.True(t, r.IsApprover, "@%s", r.Username)
	}
}

// liveWithManagedRule is an MR with reviewers alice and bob whose managed
// rule, as read, lists members and needs required approvals.
// liveWithManagedRule is an MR reviewed by alice and bob whose managed rule has
// both as members and eligible as its eligible approvers.
func liveWithManagedRule(required int, eligible ...string) domain.MergeRequest {
	mr := fetchedMR(domain.ReviewerInfo{Username: userAlice}, domain.ReviewerInfo{Username: userBob})
	mr.ApplyApproverRules([]domain.ApproverRule{{
		Name: domain.ManagedApproverRuleName, Approvers: eligible, Members: []string{userAlice, userBob},
		ApprovalsRequired: required, Managed: true, UserIDs: map[string]int64{userAlice: 101, userBob: 102},
	}})
	return mr
}

func TestApplyReviewerChanges_KeepsManagedMembersWhoseRuleNeedsFewerApprovals(t *testing.T) {
	live := liveWithManagedRule(1, userAlice, userBob) // approvals required lowered by hand in GitLab
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(live, nil).Twice()
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 102, 103}).
		Return(nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 102, 103}).
		Return(nil).Once()

	before := []mrsvc.ReviewerEdit{
		{Username: userAlice, UserID: 101, IsApprover: true},
		{Username: userBob, UserID: 102, IsApprover: true},
	}
	after := append(append([]mrsvc.ReviewerEdit(nil), before...),
		mrsvc.ReviewerEdit{Username: userCarol, UserID: 103, IsApprover: true})
	mr, _, err := applyChanges(src,
		mrsvc.ReviewerChange{Staged: after, Baseline: before, Mode: mrsvc.ReviewerWriteEdit}, map[string]int64{})

	require.NoError(t, err)
	assert.Equal(t, []string{userAlice, userBob, userCarol}, mr.ManagedApprovers())
}

// A managed rule whose eligible approvers leave bob out because he cannot
// approve (he pushed a commit on a project preventing committer approval):
// he is still a member, and adding carol keeps him on the rule.
func TestApplyReviewerChanges_KeepsManagedMembersOfARuleReadShort(t *testing.T) {
	live := liveWithManagedRule(2, userAlice)
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(live, nil).Twice()
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 102, 103}).
		Return(nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 102, 103}).
		Return(nil).Once()

	before := []mrsvc.ReviewerEdit{
		{Username: userAlice, UserID: 101, IsApprover: true},
		{Username: userBob, UserID: 102, IsApprover: true},
	}
	after := append(append([]mrsvc.ReviewerEdit(nil), before...),
		mrsvc.ReviewerEdit{Username: userCarol, UserID: 103, IsApprover: true})
	mr, _, err := applyChanges(src,
		mrsvc.ReviewerChange{Staged: after, Baseline: before, Mode: mrsvc.ReviewerWriteEdit}, map[string]int64{})

	require.NoError(t, err)
	assert.Equal(t, []string{userAlice, userBob, userCarol}, mr.ManagedApprovers(), "alice and bob survive the save")
}

func TestApplyReviewerChanges_UnionLeavesALockedApproverOutOfTheManagedRule(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t) // no write EXPECT(): alice is an approver already
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(liveWithForeignRule(), nil).Once()

	change := mrsvc.ReviewerChange{
		Staged: []mrsvc.ReviewerEdit{{Username: userAlice, UserID: 101, IsApprover: true}},
		Mode:   mrsvc.ReviewerWriteUnion,
	}
	_, changed, err := applyChanges(src, change, map[string]int64{})
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestApplyReviewerChanges_UntickingKeepsTheOtherRulesApprovers(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(liveWithForeignRule(), nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{}).Return(nil).Once()
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(liveWithForeignRule(), nil).Once()

	bob := mrsvc.ReviewerEdit{Username: userBob, UserID: 102, IsApprover: true}
	unticked := bob
	unticked.IsApprover = false
	alice := mrsvc.ReviewerEdit{Username: userAlice, UserID: 101, IsApprover: true}
	carol := mrsvc.ReviewerEdit{Username: userCarol, UserID: 103}
	mr, changed, err := applyChanges(src, mrsvc.ReviewerChange{
		Staged:   []mrsvc.ReviewerEdit{alice, unticked, carol},
		Baseline: []mrsvc.ReviewerEdit{alice, bob, carol},
		Mode:     mrsvc.ReviewerWriteEdit,
	}, map[string]int64{})

	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, []string{userAlice, userDave}, mr.Approvers, "orekit-stelar's approvers remain")
}

func TestApplyReviewerChanges_KeepsManagedApproversWhoAreNotReviewers(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	live := fetchedMR(domain.ReviewerInfo{Username: userAlice})
	live.SetManagedApprovers([]string{userDave})
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(live, nil).Once()
	src.EXPECT().GetProjectMembers(mock.Anything, writeTestProjectID).
		Return([]domain.ProjectMember{{UserID: 104, Username: userDave}}, nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 104}).Return(nil).Once()
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(live, nil).Once()

	alice := mrsvc.ReviewerEdit{Username: userAlice, UserID: 101}
	ticked := alice
	ticked.IsApprover = true
	_, _, err := applyChanges(src, mrsvc.ReviewerChange{
		Staged: []mrsvc.ReviewerEdit{ticked}, Baseline: []mrsvc.ReviewerEdit{alice}, Mode: mrsvc.ReviewerWriteEdit,
	}, map[string]int64{})
	require.NoError(t, err)
}

func TestApplyReviewerChanges_ManagedMemberWhoLeftTheProjectResolvesFromTheRead(t *testing.T) {
	// dave is on the managed rule but no longer a project member, so
	// GetProjectMembers cannot resolve him. The read carries his ID: the save
	// keeps him without a members lookup instead of failing.
	src := mocks.NewMockMergeRequestSource(t) // no GetProjectMembers EXPECT()
	live := fetchedMR(domain.ReviewerInfo{Username: userAlice})
	live.SetManagedApprovers([]string{userDave})
	live.ApproverRules[0].UserIDs = map[string]int64{userDave: 104}
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(live, nil).Twice()
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 104}).Return(nil).Once()

	alice := mrsvc.ReviewerEdit{Username: userAlice, UserID: 101}
	ticked := alice
	ticked.IsApprover = true
	_, changed, err := applyChanges(src, mrsvc.ReviewerChange{
		Staged: []mrsvc.ReviewerEdit{ticked}, Baseline: []mrsvc.ReviewerEdit{alice}, Mode: mrsvc.ReviewerWriteEdit,
	}, map[string]int64{})

	require.NoError(t, err)
	assert.True(t, changed)
}

func TestApplyReviewerChanges_RemovingAManagedAndLockedReviewerKeepsThemApprover(t *testing.T) {
	// alice is on the managed rule and on orekit-stelar. Removing her as a
	// reviewer drops her from the managed rule; orekit-stelar keeps her approver.
	live := liveWithForeignRule()
	live.SetManagedApprovers([]string{userAlice, userBob})
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(live, nil).Twice()
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{102, 103}).Return(nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{102}).Return(nil).Once()

	alice := mrsvc.ReviewerEdit{Username: userAlice, UserID: 101, IsApprover: true}
	bob := mrsvc.ReviewerEdit{Username: userBob, UserID: 102, IsApprover: true}
	carol := mrsvc.ReviewerEdit{Username: userCarol, UserID: 103}
	mr, changed, err := applyChanges(src, mrsvc.ReviewerChange{
		Staged:   []mrsvc.ReviewerEdit{bob, carol},
		Baseline: []mrsvc.ReviewerEdit{alice, bob, carol},
		Mode:     mrsvc.ReviewerWriteEdit,
	}, map[string]int64{})

	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, []string{userBob}, mr.ManagedApprovers())
	assert.Contains(t, mr.Approvers, userAlice, "orekit-stelar keeps alice an approver")
}
