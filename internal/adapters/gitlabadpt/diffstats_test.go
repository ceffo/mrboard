package gitlabadpt

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

const (
	testSHAOld = "1111111111111111111111111111111111111111"
	testSHANew = "2222222222222222222222222222222222222222"
)

func gqlMRAtSHA(projectGID, iid, fullPath, sha string) pkggitlab.GQLMergeRequest {
	mr := gqlMR(projectGID, iid, fullPath)
	mr.DiffHeadSHA = sha
	return mr
}

func userSourceAdapter(client *fakeFetchClient) *GitLabAdapter {
	return New(client, Config{Sources: []mrsvc.Source{{Type: mrsvc.SourceTypeUser, IDs: []string{testUserPriya}}}})
}

func TestFetchAll_DiffStats_FetchedForNewMRs(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMRAtSHA("gid://gitlab/Project/1", "7", "group/one", testSHANew),
	}
	client.diffStats = map[string][]pkggitlab.GQLDiffStat{
		"group/one!7": {{Path: "main.go", Additions: 10, Deletions: 2}, {Path: "go.sum", Additions: 40}},
	}

	mrs, errs := userSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{})

	assert.Empty(t, errs)
	require.Len(t, mrs, 1)
	assert.Equal(t, testSHANew, mrs[0].HeadSHA)
	assert.Equal(t, []domain.FileDiffStat{
		{Path: "main.go", Additions: 10, Deletions: 2},
		{Path: "go.sum", Additions: 40},
	}, mrs[0].DiffStats)
	assert.Equal(t, []pkggitlab.MRDiffStatsRequest{{ProjectFullPath: "group/one", IID: "7"}}, client.diffStatsReqs)
}

func TestFetchAll_DiffStats_OnlyMovedSHAsRefetched(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMRAtSHA("gid://gitlab/Project/1", "1", "group/one", testSHAOld),
		gqlMRAtSHA("gid://gitlab/Project/2", "2", "group/two", testSHANew),
	}
	cachedStats := []domain.FileDiffStat{{Path: "a.go", Additions: 5}}
	previous := []domain.MergeRequest{
		{ProjectID: 1, IID: 1, HeadSHA: testSHAOld, DiffStats: cachedStats},
		{ProjectID: 2, IID: 2, HeadSHA: testSHAOld, DiffStats: []domain.FileDiffStat{{Path: "old.go", Additions: 1}}},
	}

	mrs, errs := userSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{Previous: previous})

	assert.Empty(t, errs)
	require.Len(t, mrs, 2)
	byKey := map[domain.MRKey]domain.MergeRequest{mrs[0].Key(): mrs[0], mrs[1].Key(): mrs[1]}
	assert.Equal(t, cachedStats, byKey[domain.MRKey{ProjectID: 1, IID: 1}].DiffStats,
		"an MR whose head SHA did not move keeps its cached stats")
	assert.Equal(t, []domain.FileDiffStat{}, byKey[domain.MRKey{ProjectID: 2, IID: 2}].DiffStats,
		"an MR whose head SHA moved gets freshly fetched stats")
	assert.Equal(t, []pkggitlab.MRDiffStatsRequest{{ProjectFullPath: "group/two", IID: "2"}}, client.diffStatsReqs,
		"only the MR whose head SHA moved is refetched")
}

func TestFetchAll_DiffStats_NothingMovedIssuesNoRequest(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMRAtSHA("gid://gitlab/Project/1", "1", "group/one", testSHAOld),
	}
	previous := []domain.MergeRequest{{ProjectID: 1, IID: 1, HeadSHA: testSHAOld, DiffStats: []domain.FileDiffStat{}}}

	mrs, errs := userSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{Previous: previous})

	assert.Empty(t, errs)
	require.Len(t, mrs, 1)
	assert.Equal(t, 0, client.diffStatsCalls)
	assert.NotNil(t, mrs[0].DiffStats, "a cached known-empty diff stays known")
}

func TestFetchAll_DiffStats_BatchedAtMostBatchSize(t *testing.T) {
	client := newFakeFetchClient()
	n := gqlDiffStatsBatchSize + 1
	for i := 1; i <= n; i++ {
		client.userMRs[testUserPriya] = append(client.userMRs[testUserPriya], gqlMRAtSHA(
			fmt.Sprintf("gid://gitlab/Project/%d", i), "1", fmt.Sprintf("group/p%d", i), testSHANew))
	}

	mrs, errs := userSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{})

	assert.Empty(t, errs)
	assert.Len(t, mrs, n)
	assert.Equal(t, 2, client.diffStatsCalls)
	assert.Len(t, client.diffStatsReqs, n)
	for _, size := range client.diffStatsBatch {
		assert.LessOrEqual(t, size, gqlDiffStatsBatchSize)
	}
	assert.LessOrEqual(t, gqlDiffStatsBatchSize, 40, "docs/adr/0015 caps a diffStats batch at 40 MRs")
}

func TestFetchAll_DiffStats_FailedBatchLeavesMovedMRsUnknown(t *testing.T) {
	client := newFakeFetchClient()
	client.diffStatsErr = errors.New("boom")
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMRAtSHA("gid://gitlab/Project/1", "1", "group/one", testSHAOld),
		gqlMRAtSHA("gid://gitlab/Project/2", "2", "group/two", testSHANew),
	}
	cachedStats := []domain.FileDiffStat{{Path: "a.go", Additions: 5}}
	previous := []domain.MergeRequest{
		{ProjectID: 1, IID: 1, HeadSHA: testSHAOld, DiffStats: cachedStats},
		{ProjectID: 2, IID: 2, HeadSHA: testSHAOld, DiffStats: []domain.FileDiffStat{{Path: "old.go", Additions: 1}}},
	}

	mrs, errs := userSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{Previous: previous})

	require.Len(t, mrs, 2, "a failed diff-stats batch never drops MRs")
	assert.Len(t, errs, 1, "a failed batch joins FetchAll's errors")
	byKey := map[domain.MRKey]domain.MergeRequest{mrs[0].Key(): mrs[0], mrs[1].Key(): mrs[1]}
	assert.Equal(t, cachedStats, byKey[domain.MRKey{ProjectID: 1, IID: 1}].DiffStats)
	moved := byKey[domain.MRKey{ProjectID: 2, IID: 2}]
	assert.Nil(t, moved.DiffStats, "stats of an older SHA must not describe the moved diff")
	assert.False(t, moved.MeasureReviewSize(domain.NewSizeExclusions(true, nil, nil)).Size.Known)
}

func TestFetchAll_DiffStats_RunsConcurrentlyWithPhase2(t *testing.T) {
	client := newFakeFetchClient()
	client.rendezvous = newRendezvous()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMRAtSHA("gid://gitlab/Project/1", "1", "group/one", testSHANew),
	}

	_, errs := userSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{})

	assert.Empty(t, errs)
	assert.True(t, client.rendezvous.met[0] && client.rendezvous.met[1],
		"the diff-stats and phase-2 discussions requests must be in flight together")
}

func TestPlanDiffStats_RESTStubsKeyedBySHA(t *testing.T) {
	stub := &gl.BasicMergeRequest{
		ProjectID:  9,
		IID:        3,
		SHA:        testSHANew,
		References: &gl.IssueReferences{Full: "group/rest!3"},
	}
	heads := liveDiffHeads([]*gl.BasicMergeRequest{stub}, nil)
	require.Len(t, heads, 1)
	assert.Equal(t, testSHANew, heads[0].headSHA)

	previous := []domain.MergeRequest{{ProjectID: 9, IID: 3, HeadSHA: testSHANew, DiffStats: []domain.FileDiffStat{}}}
	cached, targets, unaddressable := planDiffStats(heads, previous)
	assert.Len(t, cached, 1)
	assert.Empty(t, targets)
	assert.Zero(t, unaddressable)

	stub.SHA = testSHAOld
	cached, targets, _ = planDiffStats(liveDiffHeads([]*gl.BasicMergeRequest{stub}, nil), previous)
	assert.Empty(t, cached)
	assert.Equal(t, []diffStatsTarget{{
		key: mrKey{ProjectID: 9, IID: 3},
		req: pkggitlab.MRDiffStatsRequest{ProjectFullPath: "group/rest", IID: "3"},
	}}, targets)
}

func TestPlanDiffStats_RefetchesWhenCacheCannotVouch(t *testing.T) {
	key := mrKey{ProjectID: 1, IID: 1}
	tests := []struct {
		name string
		live string
		prev domain.MergeRequest
	}{
		{"snapshot predates diff stats", testSHANew, domain.MergeRequest{ProjectID: 1, IID: 1}},
		{"cached stats unknown", testSHANew, domain.MergeRequest{ProjectID: 1, IID: 1, HeadSHA: testSHANew}},
		{"no live head SHA", "", domain.MergeRequest{ProjectID: 1, IID: 1, DiffStats: []domain.FileDiffStat{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			heads := []liveDiffHead{{key: key, headSHA: tc.live, projectPath: "group/one", iid: "1"}}
			cached, targets, _ := planDiffStats(heads, []domain.MergeRequest{tc.prev})
			assert.Empty(t, cached)
			assert.Len(t, targets, 1)
		})
	}
}

func TestPlanDiffStats_UnaddressableLeftUnknown(t *testing.T) {
	heads := liveDiffHeads([]*gl.BasicMergeRequest{{ProjectID: 1, IID: 1, SHA: testSHANew}}, nil)
	cached, targets, unaddressable := planDiffStats(heads, nil)
	assert.Empty(t, cached)
	assert.Empty(t, targets)
	assert.Equal(t, 1, unaddressable)
}

func TestMappers_CarryHeadSHA(t *testing.T) {
	rest := MapMR(&gl.BasicMergeRequest{IID: 1, SHA: testSHANew}, nil, &gl.MergeRequestApprovals{}, nil)
	assert.Equal(t, testSHANew, rest.HeadSHA, "REST stubs map sha")

	thin := gqlMRAtSHA("gid://gitlab/Project/1", "1", "group/one", testSHAOld)
	assert.Equal(t, testSHAOld, MapMRFromGraphQL(thin).HeadSHA)
	assert.Equal(t, testSHAOld, MergeMRFromGraphQL(thin, domain.MergeRequest{}).HeadSHA)
}
