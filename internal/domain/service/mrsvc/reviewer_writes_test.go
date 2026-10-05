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

func fetchedMR(reviewers ...domain.ReviewerInfo) domain.MergeRequest {
	return domain.MergeRequest{ID: 1, IID: int(writeTestMRIID), ProjectID: int(writeTestProjectID), Reviewers: reviewers}
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

func TestApplyStagedApproverFlags_OverlaysIntent(t *testing.T) {
	mr := domain.MergeRequest{
		Reviewers: []domain.ReviewerInfo{
			{Username: "doc"},
			{Username: "biff"},
		},
	}
	mrsvc.ApplyStagedApproverFlags(&mr, map[string]bool{"doc": true})

	assert.True(t, mr.Reviewers[0].IsApprover, "doc should be flagged as approver from staged intent")
	assert.False(t, mr.Reviewers[1].IsApprover, "biff was not staged as approver and must stay false")
}
