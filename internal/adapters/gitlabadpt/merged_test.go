package gitlabadpt

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

const (
	testMergedGroup  = "team-group"
	testMergedAuthor = "user:" + testUserPriya
)

var testSprintStart = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

func mergedGQLMR(projectGID, iid, fullPath, mergedAt string) pkggitlab.GQLMergeRequest {
	mr := gqlMRAtSHA(projectGID, iid, fullPath, testSHANew)
	mr.MergedAt = mergedAt
	return mr
}

func mergedSourceAdapter(client *fakeFetchClient) *GitLabAdapter {
	return New(client, Config{
		Sources: []mrsvc.Source{
			{Type: mrsvc.SourceTypeUser, IDs: []string{testUserPriya}},
			{Type: mrsvc.SourceTypeGroup, IDs: []string{testMergedGroup}},
		},
		ExcludedAuthors:   []string{"bot"},
		ReviewerUsernames: []string{testUserAlice},
	})
}

func byMRKey(mrs []domain.MergeRequest) map[domain.MRKey]domain.MergeRequest {
	out := make(map[domain.MRKey]domain.MergeRequest, len(mrs))
	for _, mr := range mrs {
		out[mr.Key()] = mr
	}
	return out
}

func TestFetchAll_MergedSinceZero_ListsNoMergedMRs(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{gqlMR("gid://gitlab/Project/1", "1", testProjectOne)}

	mrs, errs := mergedSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{IncludeReviewerMRs: true})

	assert.Empty(t, errs)
	assert.Len(t, mrs, 1)
	assert.Empty(t, client.mergedCalls, "a zero MergedSince must not list merged MRs")
}

func TestFetchAll_MergedSince_ListsEverySourceThinAndSized(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{gqlMR("gid://gitlab/Project/1", "1", testProjectOne)}
	client.merged = map[string][]pkggitlab.GQLMergeRequest{
		testMergedAuthor: {mergedGQLMR("gid://gitlab/Project/1", "2", testProjectOne, "2026-10-13T09:00:00Z")},
		"group:" + testMergedGroup: {
			mergedGQLMR("gid://gitlab/Project/2", "3", "group/two", "2026-10-12T00:00:00Z"),
		},
	}
	client.diffStats = map[string][]pkggitlab.GQLDiffStat{
		testProjectOne + "!2": {{Path: "merged.go", Additions: 4, Deletions: 1}},
	}

	mrs, errs := mergedSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{
		MergedSince: testSprintStart,
	})

	assert.Empty(t, errs)
	assert.ElementsMatch(t, []string{testMergedAuthor, "group:" + testMergedGroup}, client.mergedCalls,
		"every source is listed; reviewer listings only with reviewer MRs included")
	for _, after := range client.mergedAfterArg {
		assert.True(t, after.Equal(testSprintStart))
	}
	got := byMRKey(mrs)
	require.Len(t, got, 3)

	open := got[domain.MRKey{ProjectID: 1, IID: 1}]
	assert.False(t, open.Merged())
	merged := got[domain.MRKey{ProjectID: 1, IID: 2}]
	assert.Equal(t, time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC), merged.MergedAt.UTC())
	assert.Equal(t, []domain.FileDiffStat{{Path: "merged.go", Additions: 4, Deletions: 1}}, merged.DiffStats,
		"merged MRs get the size stage")
	assert.False(t, merged.GeneratedRulesUnknown, "merged MRs get the generated-file rules")
	assert.True(t, got[domain.MRKey{ProjectID: 2, IID: 3}].Merged(), "a merge exactly at the bound counts")

	assert.Equal(t, []pkggitlab.MRDiscussionsRequest{{ProjectFullPath: testProjectOne, IID: "1"}},
		client.discussionReqs, "merged MRs skip phase-2 discussions")
	assert.ElementsMatch(t, []pkggitlab.MRDiffStatsRequest{
		{ProjectFullPath: testProjectOne, IID: "1"},
		{ProjectFullPath: testProjectOne, IID: "2"},
		{ProjectFullPath: "group/two", IID: "3"},
	}, client.diffStatsReqs)
}

func TestFetchAll_MergedSince_SameDedupAndExclusions(t *testing.T) {
	client := newFakeFetchClient()
	both := gqlMR("gid://gitlab/Project/1", "1", testProjectOne)
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{both}

	bot := mergedGQLMR("gid://gitlab/Project/1", "5", testProjectOne, "2026-10-14T00:00:00Z")
	bot.Author.Username = "bot"
	archived := mergedGQLMR("gid://gitlab/Project/9", "6", "old/archive", "2026-10-14T00:00:00Z")
	archived.Project.Archived = true
	client.merged = map[string][]pkggitlab.GQLMergeRequest{
		testMergedAuthor: {
			mergedGQLMR("gid://gitlab/Project/1", "1", testProjectOne, "2026-10-14T00:00:00Z"),
			mergedGQLMR("gid://gitlab/Project/1", "7", testProjectOne, "2026-10-11T23:59:59Z"),
		},
		"group:" + testMergedGroup: {
			mergedGQLMR("gid://gitlab/Project/1", "1", testProjectOne, "2026-10-14T00:00:00Z"),
			bot, archived,
		},
	}

	mrs, errs := mergedSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{
		MergedSince: testSprintStart,
	})

	assert.Empty(t, errs)
	require.Len(t, mrs, 1, "duplicates collapse; excluded authors, archived projects and earlier merges drop")
	assert.True(t, mrs[0].Merged(), "an MR listed both open and merged counts once, as merged")
	assert.Empty(t, client.discussionReqs)
}

func TestFetchAll_MergedSince_ReviewerListingMarksReviewerSource(t *testing.T) {
	client := newFakeFetchClient()
	client.merged = map[string][]pkggitlab.GQLMergeRequest{
		testMergedAuthor: {mergedGQLMR("gid://gitlab/Project/1", "1", testProjectOne, "2026-10-14T00:00:00Z")},
		"reviewer:" + testUserAlice: {
			mergedGQLMR("gid://gitlab/Project/1", "1", testProjectOne, "2026-10-14T00:00:00Z"),
			mergedGQLMR("gid://gitlab/Project/3", "8", "other/three", "2026-10-14T00:00:00Z"),
		},
	}

	mrs, errs := mergedSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{
		MergedSince: testSprintStart, IncludeReviewerMRs: true,
	})

	assert.Empty(t, errs)
	assert.Contains(t, client.mergedCalls, "reviewer:"+testUserAlice)
	got := byMRKey(mrs)
	require.Len(t, got, 2)
	assert.False(t, got[domain.MRKey{ProjectID: 1, IID: 1}].ReviewerSource)
	assert.True(t, got[domain.MRKey{ProjectID: 3, IID: 8}].ReviewerSource)
}

func TestFetchAll_MergedSince_ReusesCachedStatsAtSameSHA(t *testing.T) {
	client := newFakeFetchClient()
	client.merged = map[string][]pkggitlab.GQLMergeRequest{
		testMergedAuthor: {mergedGQLMR("gid://gitlab/Project/1", "2", testProjectOne, "2026-10-14T00:00:00Z")},
	}
	cached := []domain.FileDiffStat{{Path: "cached.go", Additions: 9}}
	previous := []domain.MergeRequest{{
		ProjectID: 1, IID: 2, HeadSHA: testSHANew, DiffStats: cached,
		MergedAt: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
	}}

	mrs, errs := mergedSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{
		MergedSince: testSprintStart, Previous: previous,
	})

	assert.Empty(t, errs)
	require.Len(t, mrs, 1)
	assert.Equal(t, cached, mrs[0].DiffStats)
	assert.Empty(t, client.diffStatsReqs, "a merged MR's stats are fetched once while its head SHA holds")
}

func TestFetchAll_MergedSince_ListingErrorKeepsTheRest(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{gqlMR("gid://gitlab/Project/1", "1", testProjectOne)}
	client.merged = map[string][]pkggitlab.GQLMergeRequest{
		testMergedAuthor: {mergedGQLMR("gid://gitlab/Project/1", "2", testProjectOne, "2026-10-14T00:00:00Z")},
	}
	client.mergedErr = map[string]error{"group:" + testMergedGroup: errors.New("boom")}

	mrs, errs := mergedSourceAdapter(client).FetchAll(context.Background(), mrsvc.FetchOptions{
		MergedSince: testSprintStart,
	})

	require.Len(t, errs, 1)
	assert.ErrorContains(t, errs[0], testMergedGroup)
	assert.Len(t, mrs, 2, "a failed merged listing leaves the open MRs and the other listings intact")
}
