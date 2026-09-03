package mrsvc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

func TestAutoAssignReviewers_WritesResolvedUserIDs(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101, 102}).Return(nil).Once()

	reviewers := []domain.User{{ID: 101, Username: userAlice}, {ID: 102, Username: userBob}}
	err := mrsvc.AutoAssignReviewers(context.Background(), src, writeTestProjectID, writeTestMRIID, reviewers)

	require.NoError(t, err)
}

func TestAutoAssignReviewers_NoReviewersIsNoOp(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t) // no EXPECT() — calling SetReviewers fails the test

	err := mrsvc.AutoAssignReviewers(context.Background(), src, writeTestProjectID, writeTestMRIID, nil)

	require.NoError(t, err)
}

func TestAutoAssignReviewers_PropagatesWriteError(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	writeErr := errors.New("gitlab: forbidden")
	src.EXPECT().SetReviewers(mock.Anything, writeTestProjectID, writeTestMRIID, []int64{101}).Return(writeErr).Once()

	reviewers := []domain.User{{ID: 101, Username: userAlice}}
	err := mrsvc.AutoAssignReviewers(context.Background(), src, writeTestProjectID, writeTestMRIID, reviewers)

	require.ErrorIs(t, err, writeErr)
}
