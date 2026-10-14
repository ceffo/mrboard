package gitlabadpt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gl "gitlab.com/gitlab-org/api/client-go"
)

// fakeEnrichClient serves the REST enrichment reads. The embedded interface is
// nil, so reading the MR's own approval rules instead of the enforced ones panics.
type fakeEnrichClient struct {
	gitLabClient
	enforced []*gl.MergeRequestApprovalRule
}

func (f *fakeEnrichClient) GetMRDiscussions(_ context.Context, _, _ int64) ([]*gl.Discussion, error) {
	return nil, nil
}

func (f *fakeEnrichClient) GetMRApprovals(_ context.Context, _, _ int64) (*gl.MergeRequestApprovals, error) {
	return &gl.MergeRequestApprovals{}, nil
}

func (f *fakeEnrichClient) GetMRApprovalStateRules(
	_ context.Context, _, _ int64,
) ([]*gl.MergeRequestApprovalRule, error) {
	return f.enforced, nil
}

// An MR that never overrode its project's rules has no rules of its own, yet
// GitLab enforces the project's; those are its approvers.
func TestEnrichMR_ApproversComeFromEnforcedRules(t *testing.T) {
	c := &fakeEnrichClient{enforced: []*gl.MergeRequestApprovalRule{approvalRule("project rule", testUserAlice)}}
	a := &GitLabAdapter{client: c}

	mr, err := a.enrichMR(context.Background(), &gl.BasicMergeRequest{ProjectID: 1, IID: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{testUserAlice}, mr.Approvers)
}

func TestEnrichMR_DraftKeepsItsApprovers(t *testing.T) {
	c := &fakeEnrichClient{enforced: []*gl.MergeRequestApprovalRule{approvalRule("approvers", testUserAlice)}}
	a := &GitLabAdapter{client: c}

	mr, err := a.enrichMR(context.Background(), &gl.BasicMergeRequest{
		ProjectID: 1, IID: 2, Draft: true,
		Reviewers: []*gl.BasicUser{basicUser(testUserAlice, testUserAlice)},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{testUserAlice}, mr.Approvers)
	require.Len(t, mr.Reviewers, 1)
	assert.True(t, mr.Reviewers[0].IsApprover)
}
