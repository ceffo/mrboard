package gitlabadpt

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/pkg/diskstore"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

const (
	testAttrsGenerated  = "gen/** linguist-generated\n"
	testBranchMain      = "main"
	testProjectOne      = "group/one"
	testProjectTwo      = "group/two"
	testAttrsKeyOneMain = testProjectOne + "@" + testBranchMain
)

// fakeClock is a settable diskstore.Clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestAttrsCache(t *testing.T) (*GitAttributesCache, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)}
	c, err := NewGitAttributesCache(diskstore.Config{FS: afero.NewMemMapFs(), Dir: "/cache/gitattributes", Clock: clock})
	require.NoError(t, err)
	return c, clock
}

func gqlMROnBranch(projectGID, iid, fullPath, branch string) pkggitlab.GQLMergeRequest {
	mr := gqlMR(projectGID, iid, fullPath)
	mr.TargetBranch = branch
	return mr
}

func attrsAdapter(client *fakeFetchClient, cache *GitAttributesCache) *GitLabAdapter {
	return New(client, Config{
		Sources:       []mrsvc.Source{{Type: mrsvc.SourceTypeUser, IDs: []string{testUserPriya}}},
		GitAttributes: cache,
	})
}

func TestFetchAll_GitAttributes_RulesReachEveryMROfTheProjectBranch(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMROnBranch("gid://gitlab/Project/1", "1", testProjectOne, testBranchMain),
		gqlMROnBranch("gid://gitlab/Project/1", "2", testProjectOne, testBranchMain),
		gqlMROnBranch("gid://gitlab/Project/1", "3", testProjectOne, "release"),
		gqlMROnBranch("gid://gitlab/Project/2", "4", testProjectTwo, testBranchMain),
	}
	client.gitAttrs = map[string]string{testAttrsKeyOneMain: testAttrsGenerated}
	cache, _ := newTestAttrsCache(t)

	mrs, errs := attrsAdapter(client, cache).FetchAll(context.Background(), mrsvc.FetchOptions{})

	assert.Empty(t, errs)
	require.Len(t, mrs, 4)
	assert.Equal(t, 1, client.gitAttrsCalls, "every expired pair is read in one aliased query")
	assert.ElementsMatch(t, []pkggitlab.GitAttributesRequest{
		{ProjectFullPath: testProjectOne, Ref: testBranchMain},
		{ProjectFullPath: testProjectOne, Ref: "release"},
		{ProjectFullPath: testProjectTwo, Ref: testBranchMain},
	}, client.gitAttrsReqs, "one request per distinct (project, target branch)")
	want := []domain.GeneratedRule{{Glob: "gen/**", Attr: domain.AttrLinguistGenerated, State: domain.AttrSet}}
	for _, mr := range mrs {
		if mr.ProjectID == 1 && mr.TargetBranch == testBranchMain {
			assert.Equal(t, want, mr.GeneratedRules, "MR !%d", mr.IID)
		} else {
			assert.Empty(t, mr.GeneratedRules, "MR !%d reads a .gitattributes with no rules", mr.IID)
		}
	}
}

func TestFetchAll_GitAttributes_RESTStubsUseTheirProjectAndTargetBranch(t *testing.T) {
	keys := gitAttrsKeys([]*gl.BasicMergeRequest{{
		ProjectID: 9, IID: 3, TargetBranch: "develop",
		References: &gl.IssueReferences{Full: "group/rest!3"},
	}}, nil)

	assert.Equal(t, map[mrKey]gitAttrsKey{{ProjectID: 9, IID: 3}: {project: "group/rest", ref: "develop"}}, keys)
}

func TestFetchAll_GitAttributes_CachedFor24h(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMROnBranch("gid://gitlab/Project/1", "1", testProjectOne, testBranchMain),
		gqlMROnBranch("gid://gitlab/Project/2", "2", testProjectTwo, testBranchMain),
	}
	client.gitAttrs = map[string]string{testAttrsKeyOneMain: testAttrsGenerated}
	cache, clock := newTestAttrsCache(t)
	adapter := attrsAdapter(client, cache)

	_, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{})
	require.Empty(t, errs)
	require.Len(t, client.gitAttrsReqs, 2)

	clock.advance(GitAttributesTTL - time.Minute)
	mrs, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{})
	assert.Empty(t, errs)
	assert.Equal(t, 1, client.gitAttrsCalls, "unexpired entries, a missing file included, issue no request")
	require.Len(t, mrs, 2)
	for _, mr := range mrs {
		if mr.ProjectID == 1 {
			assert.NotEmpty(t, mr.GeneratedRules, "cached rules still apply")
		}
	}

	clock.advance(2 * time.Minute)
	_, errs = adapter.FetchAll(context.Background(), mrsvc.FetchOptions{})
	assert.Empty(t, errs)
	assert.Equal(t, 2, client.gitAttrsCalls, "expired entries are read again")
	assert.Len(t, client.gitAttrsReqs, 4)
}

func TestFetchAll_GitAttributes_ColdBypassesAndRefreshesTheCache(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMROnBranch("gid://gitlab/Project/1", "1", testProjectOne, testBranchMain),
	}
	cache, _ := newTestAttrsCache(t)
	adapter := attrsAdapter(client, cache)
	_, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{})
	require.Empty(t, errs)

	client.gitAttrs = map[string]string{testAttrsKeyOneMain: testAttrsGenerated}
	mrs, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{Cold: true})
	assert.Empty(t, errs)
	assert.Equal(t, 2, client.gitAttrsCalls, "a cold fetch reads despite an unexpired entry")
	require.Len(t, mrs, 1)
	assert.NotEmpty(t, mrs[0].GeneratedRules)

	mrs, _ = adapter.FetchAll(context.Background(), mrsvc.FetchOptions{})
	assert.Equal(t, 2, client.gitAttrsCalls, "the cold read refreshed the cache")
	require.Len(t, mrs, 1)
	assert.NotEmpty(t, mrs[0].GeneratedRules)
}

func TestFetchAll_GitAttributes_FailureJoinsErrorsAndIsNotCached(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMROnBranch("gid://gitlab/Project/1", "1", testProjectOne, testBranchMain),
	}
	client.gitAttrsErr = errors.New("boom")
	cache, _ := newTestAttrsCache(t)
	adapter := attrsAdapter(client, cache)

	mrs, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{})
	assert.Len(t, errs, 1)
	require.Len(t, mrs, 1, "the MR is still returned")
	assert.Nil(t, mrs[0].GeneratedRules)

	client.gitAttrsErr = nil
	_, errs = adapter.FetchAll(context.Background(), mrsvc.FetchOptions{})
	assert.Empty(t, errs)
	assert.Equal(t, 2, client.gitAttrsCalls, "a failed read is retried on the next fetch")
}

func TestGitAttributesStage_Batches(t *testing.T) {
	client := newFakeFetchClient()
	keys := make(map[mrKey]gitAttrsKey)
	const pairs = gqlGitAttributesBatchSize + 1
	for i := range pairs {
		keys[mrKey{ProjectID: i, IID: 1}] = gitAttrsKey{project: fmt.Sprintf("group/p%d", i), ref: testBranchMain}
	}

	_, errs := attrsAdapter(client, nil).gitAttributesStage(context.Background(), keys, false)

	assert.Empty(t, errs)
	assert.Equal(t, 2, client.gitAttrsCalls)
	assert.Len(t, client.gitAttrsReqs, pairs)
}
