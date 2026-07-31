package gitlabadpt

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// fakeFetchClient implements gitLabClient for FetchAll pipeline tests: thin
// GraphQL listing keyed by username, plus a call-recording batch discussions
// fetch used to assert dedup runs before enrichment and phase-2 diffing skips
// unchanged MRs. All methods outside that path panic.
type fakeFetchClient struct {
	userMRs     map[string][]pkggitlab.GQLMergeRequest
	reviewerMRs map[string][]pkggitlab.GQLMergeRequest

	mu             sync.Mutex
	batchCalls     int
	discussionReqs []pkggitlab.MRDiscussionsRequest // every req seen across all batch calls
	discussionsErr error
}

func newFakeFetchClient() *fakeFetchClient {
	return &fakeFetchClient{
		userMRs:     make(map[string][]pkggitlab.GQLMergeRequest),
		reviewerMRs: make(map[string][]pkggitlab.GQLMergeRequest),
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

func (f *fakeFetchClient) FetchMRsDiscussionsGraphQL(
	_ context.Context, reqs []pkggitlab.MRDiscussionsRequest,
) ([]pkggitlab.MRDiscussionsResult, error) {
	f.mu.Lock()
	f.batchCalls++
	f.discussionReqs = append(f.discussionReqs, reqs...)
	err := f.discussionsErr
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return make([]pkggitlab.MRDiscussionsResult, len(reqs)), nil
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

// gqlMR builds a minimal thin GQLMergeRequest for the given project/iid,
// well-formed enough for parseGIDNumericSafe/parseIIDSafe and MapMRFromGraphQL.
// Every test in this file lists it under testUserPriya's user source.
func gqlMR(projectGID, iid, fullPath string) pkggitlab.GQLMergeRequest {
	var mr pkggitlab.GQLMergeRequest
	mr.ID = "gid://gitlab/MergeRequest/1"
	mr.IID = iid
	mr.Title = "some change"
	mr.CreatedAt = "2026-07-30T00:00:00Z"
	mr.UpdatedAt = "2026-07-30T00:00:00Z"
	mr.DetailedMergeStatus = detailedMergeStatusMergeable
	mr.Author.Username = testUserPriya
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
	sharedMR := gqlMR("gid://gitlab/Project/123", "42", "group/project")
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{sharedMR}
	client.reviewerMRs[testUserBob] = []pkggitlab.GQLMergeRequest{sharedMR}

	adapter := New(client, Config{
		Sources:           []mrsvc.Source{{Type: mrsvc.SourceTypeUser, IDs: []string{testUserPriya}}},
		ReviewerUsernames: []string{testUserBob},
	})

	mrs, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{IncludeReviewerMRs: true})

	assert.Empty(t, errs)
	require.Len(t, mrs, 1, "duplicate MR from two sources must collapse to one result")
	assert.Equal(t, 42, mrs[0].IID)
	assert.Equal(t, 123, mrs[0].ProjectID)

	client.mu.Lock()
	defer client.mu.Unlock()
	assert.Equal(t, 1, client.batchCalls, "discussions must be fetched via exactly one batch request")
	require.Len(t, client.discussionReqs, 1, "discussions must be fetched exactly once for a duplicated MR")
	assert.Equal(t, pkggitlab.MRDiscussionsRequest{ProjectFullPath: "group/project", IID: "42"}, client.discussionReqs[0])
}

// TestFetchAll_Phase2SkipsUnchangedMRs verifies the epic's central latency win:
// when a phase-1 MR's updatedAt matches the previous snapshot, phase 2 issues
// zero requests for it, and the result reuses the cached discussion-derived
// fields (docs/adr/0005, "Two-phase conditional fetch").
func TestFetchAll_Phase2SkipsUnchangedMRs(t *testing.T) {
	client := newFakeFetchClient()
	sharedMR := gqlMR("gid://gitlab/Project/123", "42", "group/project")
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{sharedMR}

	updatedAt, err := time.Parse(time.RFC3339, sharedMR.UpdatedAt)
	require.NoError(t, err)
	cached := domain.MergeRequest{
		ProjectID:      123,
		IID:            42,
		UpdatedAt:      updatedAt,
		Reviewers:      []domain.ReviewerInfo{{Username: testUserBob, Name: testUserBobName, State: domain.ReviewerApproved}},
		OpenThreads:    3,
		RoundTripCount: 2,
	}

	adapter := New(client, Config{Sources: []mrsvc.Source{{Type: mrsvc.SourceTypeUser, IDs: []string{testUserPriya}}}})
	mrs, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{Previous: []domain.MergeRequest{cached}})

	assert.Empty(t, errs)
	require.Len(t, mrs, 1)

	client.mu.Lock()
	defer client.mu.Unlock()
	assert.Equal(t, 0, client.batchCalls, "phase 2 must issue zero requests when nothing changed")
	assert.Equal(t, cached.Reviewers, mrs[0].Reviewers, "unchanged MR must reuse cached reviewers")
	assert.Equal(t, cached.OpenThreads, mrs[0].OpenThreads)
	assert.Equal(t, cached.RoundTripCount, mrs[0].RoundTripCount)
}

// TestFetchAll_Phase2FetchesChangedMRs verifies an updatedAt mismatch against
// the previous snapshot triggers exactly one batched phase-2 request.
func TestFetchAll_Phase2FetchesChangedMRs(t *testing.T) {
	client := newFakeFetchClient()
	sharedMR := gqlMR("gid://gitlab/Project/123", "42", "group/project")
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{sharedMR}

	staleUpdatedAt, err := time.Parse(time.RFC3339, "2020-01-01T00:00:00Z")
	require.NoError(t, err)
	cached := domain.MergeRequest{ProjectID: 123, IID: 42, UpdatedAt: staleUpdatedAt}

	adapter := New(client, Config{Sources: []mrsvc.Source{{Type: mrsvc.SourceTypeUser, IDs: []string{testUserPriya}}}})
	mrs, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{Previous: []domain.MergeRequest{cached}})

	assert.Empty(t, errs)
	require.Len(t, mrs, 1)

	client.mu.Lock()
	defer client.mu.Unlock()
	assert.Equal(t, 1, client.batchCalls)
	require.Len(t, client.discussionReqs, 1)
	assert.Equal(t, pkggitlab.MRDiscussionsRequest{ProjectFullPath: "group/project", IID: "42"}, client.discussionReqs[0])
}

// TestFetchAll_NilPrevious_TreatsEveryMRAsChanged verifies a nil Previous — what
// mrboard fetch (the CLI JSON dump) always passes — is an unconditional full
// fetch: every survivor goes through phase 2, exactly as it did before this
// ticket introduced the diff.
func TestFetchAll_NilPrevious_TreatsEveryMRAsChanged(t *testing.T) {
	client := newFakeFetchClient()
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMR("gid://gitlab/Project/1", "1", "group/one"),
		gqlMR("gid://gitlab/Project/2", "2", "group/two"),
	}

	adapter := New(client, Config{Sources: []mrsvc.Source{{Type: mrsvc.SourceTypeUser, IDs: []string{testUserPriya}}}})
	mrs, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{})

	assert.Empty(t, errs)
	require.Len(t, mrs, 2)

	client.mu.Lock()
	defer client.mu.Unlock()
	assert.Equal(t, 1, client.batchCalls, "all changed MRs must still be a single aliased batch request")
	assert.Len(t, client.discussionReqs, 2)
}

// TestFetchAll_ForceStale_OverridesCacheHit verifies ForceStale forces a fresh
// phase-2 fetch even when updatedAt matches — the hook the write-race dirty-set
// guard (docs/adr/0005) reuses to refetch a locally-mutated MR.
func TestFetchAll_ForceStale_OverridesCacheHit(t *testing.T) {
	client := newFakeFetchClient()
	sharedMR := gqlMR("gid://gitlab/Project/123", "42", "group/project")
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{sharedMR}

	updatedAt, err := time.Parse(time.RFC3339, sharedMR.UpdatedAt)
	require.NoError(t, err)
	cached := domain.MergeRequest{ProjectID: 123, IID: 42, UpdatedAt: updatedAt}

	adapter := New(client, Config{Sources: []mrsvc.Source{{Type: mrsvc.SourceTypeUser, IDs: []string{testUserPriya}}}})
	mrs, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{
		Previous:   []domain.MergeRequest{cached},
		ForceStale: []domain.MRKey{{ProjectID: 123, IID: 42}},
	})

	assert.Empty(t, errs)
	require.Len(t, mrs, 1)

	client.mu.Lock()
	defer client.mu.Unlock()
	assert.Equal(t, 1, client.batchCalls, "a forced-stale key must be re-fetched even on an updatedAt match")
}

// TestFetchAll_Phase2BatchError_ReportedPerMR verifies a whole-batch failure
// (e.g. the aliased request's transport call fails) surfaces one error per
// affected MR rather than silently dropping them or panicking on a short
// results slice.
func TestFetchAll_Phase2BatchError_ReportedPerMR(t *testing.T) {
	client := newFakeFetchClient()
	client.discussionsErr = errors.New("boom")
	client.userMRs[testUserPriya] = []pkggitlab.GQLMergeRequest{
		gqlMR("gid://gitlab/Project/1", "1", "group/one"),
		gqlMR("gid://gitlab/Project/2", "2", "group/two"),
	}

	adapter := New(client, Config{Sources: []mrsvc.Source{{Type: mrsvc.SourceTypeUser, IDs: []string{testUserPriya}}}})
	mrs, errs := adapter.FetchAll(context.Background(), mrsvc.FetchOptions{})

	assert.Empty(t, mrs)
	assert.Len(t, errs, 2, "a whole-batch failure must surface one error per affected MR")
}
