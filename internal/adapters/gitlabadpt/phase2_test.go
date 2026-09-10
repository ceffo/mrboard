package gitlabadpt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

func gqlMRWithIID(iid string) pkggitlab.GQLMergeRequest {
	mr := pkggitlab.GQLMergeRequest{}
	mr.IID = iid
	return mr
}

func TestChunkGQLMRs(t *testing.T) {
	cases := []struct {
		name      string
		count     int
		size      int
		wantSizes []int
	}{
		{name: "empty", count: 0, size: 12, wantSizes: nil},
		{name: "under one chunk", count: 5, size: 12, wantSizes: []int{5}},
		{name: "exact multiple", count: 24, size: 12, wantSizes: []int{12, 12}},
		{name: "remainder", count: 16, size: 12, wantSizes: []int{12, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mrs := make([]pkggitlab.GQLMergeRequest, tc.count)
			for i := range mrs {
				mrs[i] = gqlMRWithIID(string(rune('a' + i)))
			}

			chunks := chunkGQLMRs(mrs, tc.size)

			require.Len(t, chunks, len(tc.wantSizes), "want chunk count")
			for i, wantSize := range tc.wantSizes {
				assert.Len(t, chunks[i], wantSize, "chunk %d size", i)
			}
		})
	}
}

func TestChunkGQLMRs_PreservesOrder(t *testing.T) {
	mrs := []pkggitlab.GQLMergeRequest{gqlMRWithIID("1"), gqlMRWithIID("2"), gqlMRWithIID("3")}

	chunks := chunkGQLMRs(mrs, 2)

	require.Len(t, chunks, 2, "want 2 chunks")
	assert.Equal(t, []string{"1", "2"}, []string{chunks[0][0].IID, chunks[0][1].IID}, "first chunk order")
	assert.Equal(t, "3", chunks[1][0].IID, "second chunk order")
}

func gqlMRWithApprovedBy(projectID, iid string, updatedAt time.Time, approvedBy ...string) pkggitlab.GQLMergeRequest {
	mr := pkggitlab.GQLMergeRequest{IID: iid, UpdatedAt: updatedAt.Format(time.RFC3339)}
	mr.Project.ID = "gid://gitlab/Project/" + projectID
	for _, u := range approvedBy {
		mr.ApprovedBy.Nodes = append(mr.ApprovedBy.Nodes, pkggitlab.GQLUser{Username: u})
	}
	return mr
}

func cachedMRWithReviewers(
	projectID, iid int, updatedAt time.Time, reviewers ...domain.ReviewerInfo,
) domain.MergeRequest {
	return domain.MergeRequest{ProjectID: projectID, IID: iid, UpdatedAt: updatedAt, Reviewers: reviewers}
}

func TestDiffGQLStage_MatchingUpdatedAtAndApprovedByIsUnchanged(t *testing.T) {
	updatedAt := time.Date(2026, 9, 10, 14, 54, 48, 0, time.UTC)
	mrs := []pkggitlab.GQLMergeRequest{gqlMRWithApprovedBy("1", "858", updatedAt, "mtherreault")}
	previous := []domain.MergeRequest{
		cachedMRWithReviewers(1, 858, updatedAt,
			domain.ReviewerInfo{Username: "mtherreault", State: domain.ReviewerApproved},
			domain.ReviewerInfo{Username: "moncef", State: domain.ReviewerNotStarted},
		),
	}

	unchanged, changed, _ := diffGQLStage(mrs, previous, nil)

	assert.Len(t, unchanged, 1, "same updatedAt and same approvedBy set: reuse cache")
	assert.Empty(t, changed)
}

func TestDiffGQLStage_ApprovalWithUnchangedUpdatedAtIsStillChanged(t *testing.T) {
	// Regression: GitLab does not bump an MR's updatedAt when someone approves
	// it, so a fresh approval can arrive with updatedAt still matching the
	// cache. approvedBy must independently force a refetch — see MR 858.
	updatedAt := time.Date(2026, 9, 10, 14, 54, 48, 0, time.UTC)
	mrs := []pkggitlab.GQLMergeRequest{gqlMRWithApprovedBy("1", "858", updatedAt, "mtherreault", "moncef")}
	previous := []domain.MergeRequest{
		cachedMRWithReviewers(1, 858, updatedAt,
			domain.ReviewerInfo{Username: "mtherreault", State: domain.ReviewerApproved},
			domain.ReviewerInfo{Username: "moncef", State: domain.ReviewerNotStarted},
		),
	}

	unchanged, changed, _ := diffGQLStage(mrs, previous, nil)

	assert.Empty(t, unchanged)
	require.Len(t, changed, 1, "approvedBy grew even though updatedAt didn't move: must refetch")
	assert.Equal(t, "858", changed[0].IID)
}
