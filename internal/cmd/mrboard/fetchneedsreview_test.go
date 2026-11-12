package mrboardcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
)

var (
	mrToReview = domain.MergeRequest{
		ProjectID: 1, IID: 10, Author: userBob, Phase: domain.PhaseNeedsReview,
		Reviewers: []domain.ReviewerInfo{{Username: userAlice, IsApprover: true}},
	}
	mrNotToReview = domain.MergeRequest{
		ProjectID: 1, IID: 11, Author: userBob, Phase: domain.PhaseNeedsReview,
		Reviewers: []domain.ReviewerInfo{{Username: userAlice, State: domain.ReviewerApproved}},
	}
	mrDraftToReview = domain.MergeRequest{
		ProjectID: 1, IID: 12, Author: userBob, Phase: domain.PhaseDraft,
		Reviewers: []domain.ReviewerInfo{{Username: userAlice, State: domain.ReviewerReReviewRequested}},
	}
)

func decodeIIDs(t *testing.T, stdout string) []int {
	t.Helper()
	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	iids := make([]int, 0, len(got))
	for _, obj := range got {
		iids = append(iids, int(obj["iid"].(float64)))
	}
	return iids
}

func TestToMRJSON_NeedsMyReviewFollowsCurrentUser(t *testing.T) {
	assert.JSONEq(t, `true`, string(encodeMR(t, mrToReview, userAlice)["needs_my_review"]))
	assert.JSONEq(t, `false`, string(encodeMR(t, mrToReview, userBob)["needs_my_review"]))
	assert.JSONEq(t, `false`, string(encodeMR(t, mrToReview, "")["needs_my_review"]))
}

func TestExecFetch_WithoutFlagKeepsEveryMR(t *testing.T) {
	stdout, _, err := runExecFetchWith(t, &config.AppConfig{CurrentUser: userAlice}, fetchCmdOptions{},
		[]domain.MergeRequest{mrToReview, mrNotToReview, mrDraftToReview}, nil)

	require.NoError(t, err)
	assert.Equal(t, []int{10, 11, 12}, decodeIIDs(t, stdout))
}

func TestExecFetch_NeedsMyReviewKeepsOnlyMatchingMRs(t *testing.T) {
	stdout, stderr, err := runExecFetchWith(t, &config.AppConfig{CurrentUser: userAlice},
		fetchCmdOptions{needsMyReview: true},
		[]domain.MergeRequest{mrToReview, mrNotToReview, mrDraftToReview}, nil)

	require.NoError(t, err)
	assert.Equal(t, []int{10}, decodeIIDs(t, stdout))
	assert.Empty(t, stderr)
}

func TestExecFetch_NeedsMyReviewCombinesWithViewAll(t *testing.T) {
	all := domain.ViewAll
	stdout, _, err := runExecFetchWith(t, &config.AppConfig{CurrentUser: userAlice},
		fetchCmdOptions{needsMyReview: true, viewOverride: &all},
		[]domain.MergeRequest{mrNotToReview, mrToReview}, nil)

	require.NoError(t, err)
	assert.Equal(t, []int{10}, decodeIIDs(t, stdout))
}

func TestExecFetch_NeedsMyReviewWithNoMatchPrintsEmptyArray(t *testing.T) {
	stdout, _, err := runExecFetchWith(t, &config.AppConfig{CurrentUser: userAlice},
		fetchCmdOptions{needsMyReview: true}, []domain.MergeRequest{mrNotToReview}, nil)

	require.NoError(t, err)
	assert.JSONEq(t, "[]", stdout)
}

func TestExecFetch_NeedsMyReviewWithoutCurrentUserFailsBeforeFetching(t *testing.T) {
	m := newFetchMocks(t)
	m.core.Config = &config.AppConfig{}
	var stdout, stderr bytes.Buffer
	ctx := context.WithValue(context.Background(), coreKey{}, m.core)

	err := execFetch(ctx, fetchCmdOptions{needsMyReview: true}, &stdout, &stderr)

	require.ErrorIs(t, err, errNoCurrentUser)
	assert.Contains(t, err.Error(), "current_user")
	assert.Empty(t, stdout.String())
}
