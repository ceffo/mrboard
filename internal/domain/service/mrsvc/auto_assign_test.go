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

func TestAutoAssignReviewers_WritesResolvedUserIDs(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(fetchedMR(), nil).Once()
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 102}).Return(nil).Once()

	reviewers := []domain.User{{ID: 101, Username: userAlice}, {ID: 102, Username: userBob}}
	assigned, err := mrsvc.AutoAssignReviewers(context.Background(), src, writeTestProjectID, writeTestMRIID, reviewers)

	require.NoError(t, err)
	assert.True(t, assigned)
}

func TestAutoAssignReviewers_NoReviewersIsNoOp(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t) // no EXPECT() — any call fails the test

	assigned, err := mrsvc.AutoAssignReviewers(context.Background(), src, writeTestProjectID, writeTestMRIID, nil)

	require.NoError(t, err)
	assert.False(t, assigned)
}

func TestAutoAssignReviewers_SkipsMRThatGainedReviewers(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t) // no SetReviewers EXPECT() — writing would replace the live reviewers
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).
		Return(fetchedMR(domain.ReviewerInfo{Username: userBob}), nil).Once()

	reviewers := []domain.User{{ID: 101, Username: userAlice}}
	assigned, err := mrsvc.AutoAssignReviewers(context.Background(), src, writeTestProjectID, writeTestMRIID, reviewers)

	require.NoError(t, err)
	assert.False(t, assigned)
}

func TestAutoAssignReviewers_PropagatesReadError(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t) // no SetReviewers EXPECT() — an unverified MR must not be written
	readErr := errors.New("gitlab: timeout")
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(domain.MergeRequest{}, readErr).Once()

	reviewers := []domain.User{{ID: 101, Username: userAlice}}
	assigned, err := mrsvc.AutoAssignReviewers(context.Background(), src, writeTestProjectID, writeTestMRIID, reviewers)

	require.ErrorIs(t, err, readErr)
	assert.False(t, assigned)
}

func TestAutoAssignReviewers_PropagatesWriteError(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	writeErr := errors.New("gitlab: forbidden")
	src.EXPECT().FetchMR(mock.Anything, writeTestProjectID, writeTestMRIID).Return(fetchedMR(), nil).Once()
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(writeErr).Once()

	reviewers := []domain.User{{ID: 101, Username: userAlice}}
	assigned, err := mrsvc.AutoAssignReviewers(context.Background(), src, writeTestProjectID, writeTestMRIID, reviewers)

	require.ErrorIs(t, err, writeErr)
	assert.False(t, assigned)
}
