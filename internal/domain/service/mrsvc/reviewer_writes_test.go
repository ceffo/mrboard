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
	src mrsvc.MergeRequestSource,
	staged []mrsvc.ReviewerEdit,
	knownIDs map[string]int64,
	current map[string]bool,
) (domain.MergeRequest, bool, error) {
	return mrsvc.ApplyReviewerChanges(
		context.Background(), src, writeTestProjectID, writeTestMRIID, staged, knownIDs, current,
	)
}

func TestApplyReviewerChanges_ResolvesUnknownIDs(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().
		GetProjectMembers(mock.Anything, writeTestProjectID).
		Return([]domain.ProjectMember{{UserID: 101, Username: userAlice}}, nil).
		Once()
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(fetchedMR(), nil).Once()

	staged := []mrsvc.ReviewerEdit{{Username: userAlice}} // UserID unresolved
	_, _, err := applyChanges(src, staged, map[string]int64{}, nil)
	require.NoError(t, err)
}

func TestApplyReviewerChanges_SkipsResolutionWhenIDsKnown(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	// No GetProjectMembers EXPECT() — calling it fails the test (strict mockery mock).
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(fetchedMR(), nil).Once()

	staged := []mrsvc.ReviewerEdit{{Username: userAlice}} // UserID unresolved but already in knownIDs
	knownIDs := map[string]int64{userAlice: 101}
	_, _, err := applyChanges(src, staged, knownIDs, nil)
	require.NoError(t, err)
}

func TestApplyReviewerChanges_DedupesReviewerIDs(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(fetchedMR(), nil).Once()

	staged := []mrsvc.ReviewerEdit{{Username: userAlice, UserID: 101}, {Username: "alice-alias", UserID: 101}}
	_, _, err := applyChanges(src, staged, map[string]int64{}, nil)
	require.NoError(t, err)
}

func TestApplyReviewerChanges_SkipsSaveApproversWhenUnchanged(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	// No SaveApprovers EXPECT() — must not be called when the set is unchanged.
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(fetchedMR(), nil).Once()

	staged := []mrsvc.ReviewerEdit{{Username: userAlice, UserID: 101, IsApprover: true}}
	current := map[string]bool{userAlice: true}
	_, changed, err := applyChanges(src, staged, map[string]int64{}, current)
	require.NoError(t, err)
	assert.False(t, changed, "approversChanged should be false when the set is unchanged")
}

func TestApplyReviewerChanges_CallsSaveApproversWhenChanged(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(fetchedMR(), nil).Once()

	staged := []mrsvc.ReviewerEdit{{Username: userAlice, UserID: 101, IsApprover: true}}
	_, changed, err := applyChanges(src, staged, map[string]int64{}, nil)
	require.NoError(t, err)
	assert.True(t, changed, "approversChanged should be true when the set grows from empty")
}

func TestApplyReviewerChanges_OverlaysStagedApproverFlagsAfterFetch(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(nil).Once()
	// FetchMR returns stale data — GitLab's approval-rule read hasn't caught up yet.
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).
		Return(fetchedMR(domain.ReviewerInfo{Username: userAlice, IsApprover: false}), nil).Once()

	staged := []mrsvc.ReviewerEdit{{Username: userAlice, UserID: 101, IsApprover: true}}
	mr, _, err := applyChanges(src, staged, map[string]int64{}, nil)
	require.NoError(t, err)
	assert.True(t, mr.Reviewers[0].IsApprover, "staged approver intent should overlay the stale fetch result")
}

func TestApplyReviewerChanges_PropagatesResolveError(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().GetProjectMembers(mock.Anything, writeTestProjectID).Return(nil, errors.New("boom")).Once()

	staged := []mrsvc.ReviewerEdit{{Username: userAlice}}
	_, _, err := applyChanges(src, staged, map[string]int64{}, nil)
	require.Error(t, err, "expected error to propagate")
}

func TestApplyReviewerChanges_PropagatesSetReviewersError(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).
		Return(errors.New("boom")).Once()

	staged := []mrsvc.ReviewerEdit{{Username: userAlice, UserID: 101}}
	_, _, err := applyChanges(src, staged, map[string]int64{}, nil)
	require.Error(t, err, "expected error to propagate")
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
