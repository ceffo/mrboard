package gitlabadpt

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// fakeFetchClient implements gitLabClient for FetchAll pipeline tests: thin
// GraphQL listing keyed by username, plus a call-counting discussions fetch used
// to assert dedup runs before enrichment. All methods outside that path panic.
type fakeFetchClient struct {
	userMRs     map[string][]pkggitlab.GQLMergeRequest
	reviewerMRs map[string][]pkggitlab.GQLMergeRequest

	mu              sync.Mutex
	discussionCalls map[string]int // key: fullPath+"!"+iid
}

func newFakeFetchClient() *fakeFetchClient {
	return &fakeFetchClient{
		userMRs:         make(map[string][]pkggitlab.GQLMergeRequest),
		reviewerMRs:     make(map[string][]pkggitlab.GQLMergeRequest),
		discussionCalls: make(map[string]int),
	}
}

func (f *fakeFetchClient) FetchUserMRsThinGraphQL(
	_ context.Context, username string,
) ([]pkggitlab.GQLMergeRequest, error) {
	return f.userMRs[username], nil
}

func (f *fakeFetchClient) FetchReviewerMRsThinGraphQL(
	_ context.Context, username string,
) ([]pkggitlab.GQLMergeRequest, error) {
	return f.reviewerMRs[username], nil
}

func (f *fakeFetchClient) FetchMRDiscussionsGraphQL(
	_ context.Context, fullPath, iid string,
) ([]pkggitlab.GQLDiscussion, bool, error) {
	f.mu.Lock()
	f.discussionCalls[fullPath+"!"+iid]++
	f.mu.Unlock()
	return nil, false, nil
}

// --- unused MRLister/MREnricher/MRWriter methods: panic if ever called ---

func (f *fakeFetchClient) FetchUserMRsGraphQL(
	_ context.Context, _ string,
) ([]pkggitlab.GQLMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) FetchReviewerMRsGraphQL(
	_ context.Context, _ string,
) ([]pkggitlab.GQLMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) ListGroupMRs(_ context.Context, _, _ string) ([]*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) ListUserMRs(_ context.Context, _ string) ([]*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) ListReviewerMRs(_ context.Context, _ string) ([]*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) ListNonArchivedProjectIDs(_ context.Context, _ string) (map[int64]bool, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) IsProjectArchived(_ context.Context, _ int64) (bool, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) GetMR(_ context.Context, _, _ int64) (*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) GetMRDiscussions(_ context.Context, _, _ int64) ([]*gl.Discussion, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) GetMRApprovals(_ context.Context, _, _ int64) (*gl.MergeRequestApprovals, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) GetMRApprovalRules(
	_ context.Context, _, _ int64,
) ([]*gl.MergeRequestApprovalRule, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) GetMRDescription(_ context.Context, _, _ int64) (string, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) GetMRDiffs(_ context.Context, _, _ int64) ([]*gl.MergeRequestDiff, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) GetMRDiffRefs(_ context.Context, _, _ int64) (string, string, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) GetRawFileContent(_ context.Context, _ int64, _, _ string) ([]byte, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) GetProjectMembers(_ context.Context, _ int64, _ int) ([]*gl.ProjectMember, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) CreateMRApprovalRule(
	_ context.Context, _, _ int64, _ pkggitlab.MRApprovalRulePayload,
) (*gl.MergeRequestApprovalRule, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) UpdateMRApprovalRule(
	_ context.Context, _, _, _ int64, _ pkggitlab.MRApprovalRulePayload,
) error {
	panic("not implemented")
}

func (f *fakeFetchClient) SetMRReviewers(_ context.Context, _, _ int64, _ []int64) error {
	panic("not implemented")
}

func (f *fakeFetchClient) ListUsersByUsername(_ context.Context, _ string) (*gl.User, error) {
	panic("not implemented")
}

func (f *fakeFetchClient) UpdateMRDescription(_ context.Context, _, _ int64, _ string) error {
	panic("not implemented")
}

// gqlMR builds a minimal thin GQLMergeRequest for the given project/iid/author,
// well-formed enough for parseGIDNumericSafe/parseIIDSafe and MapMRFromGraphQL.
func gqlMR(projectGID, iid, fullPath, author string) pkggitlab.GQLMergeRequest {
	var mr pkggitlab.GQLMergeRequest
	mr.ID = "gid://gitlab/MergeRequest/1"
	mr.IID = iid
	mr.Title = "some change"
	mr.CreatedAt = "2026-07-30T00:00:00Z"
	mr.UpdatedAt = "2026-07-30T00:00:00Z"
	mr.DetailedMergeStatus = "mergeable"
	mr.Author.Username = author
	mr.Project.ID = projectGID
	mr.Project.FullPath = fullPath
	return mr
}

// TestFetchAll_DedupBeforeEnrichment verifies the phase-1 fix: an MR reachable
// from two sources (here, its author's user source and a teammate's reviewer
// source) is deduped on the cheap thin listing before discussions are ever
// fetched, so enrichment happens exactly once — not once per source, as it did
// before this ticket (docs/adr/0005).
func TestFetchAll_DedupBeforeEnrichment(t *testing.T) {
	client := newFakeFetchClient()
	sharedMR := gqlMR("gid://gitlab/Project/123", "42", "group/project", "priya")
	client.userMRs["priya"] = []pkggitlab.GQLMergeRequest{sharedMR}
	client.reviewerMRs["bob"] = []pkggitlab.GQLMergeRequest{sharedMR}

	adapter := New(client, Config{
		Sources:           []mrsvc.Source{{Type: mrsvc.SourceTypeUser, IDs: []string{"priya"}}},
		ReviewerUsernames: []string{"bob"},
	})

	mrs, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{IncludeReviewerMRs: true})

	assert.Empty(t, errs)
	require.Len(t, mrs, 1, "duplicate MR from two sources must collapse to one result")
	assert.Equal(t, 42, mrs[0].IID)
	assert.Equal(t, 123, mrs[0].ProjectID)

	client.mu.Lock()
	defer client.mu.Unlock()
	assert.Equal(t, 1, client.discussionCalls["group/project!42"],
		"discussions must be fetched exactly once for a duplicated MR")
}
