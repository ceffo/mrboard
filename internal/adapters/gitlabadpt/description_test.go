package gitlabadpt

import (
	"context"
	"errors"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go"

	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// fakeDescriptionClient implements gitLabClient with only the methods needed
// for description write tests. All unimplemented methods panic.
type fakeDescriptionClient struct {
	updateErr   error
	updateCalls []string // captured new descriptions
}

func (f *fakeDescriptionClient) GetMRDescription(_ context.Context, _, _ int64) (string, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) UpdateMRDescription(_ context.Context, _, _ int64, desc string) error {
	f.updateCalls = append(f.updateCalls, desc)
	return f.updateErr
}

// --- MRLister stubs ---
func (f *fakeDescriptionClient) ListGroupMRs(_ context.Context, _, _ string) ([]*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) ListUserMRs(_ context.Context, _ string) ([]*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) ListReviewerMRs(_ context.Context, _ string) ([]*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) ListNonArchivedProjectIDs(_ context.Context, _ string) (map[int64]bool, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) IsProjectArchived(_ context.Context, _ int64) (bool, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) FetchUserMRsGraphQL(_ context.Context, _ string) ([]pkggitlab.GQLMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) FetchReviewerMRsGraphQL(
	_ context.Context, _ string,
) ([]pkggitlab.GQLMergeRequest, error) {
	panic("not implemented")
}

// --- MREnricher stubs ---
func (f *fakeDescriptionClient) GetMR(_ context.Context, _, _ int64) (*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) GetMRDiscussions(_ context.Context, _, _ int64) ([]*gl.Discussion, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) GetMRApprovals(_ context.Context, _, _ int64) (*gl.MergeRequestApprovals, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) GetMRApprovalRules(
	_ context.Context, _, _ int64,
) ([]*gl.MergeRequestApprovalRule, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) GetMRDiffs(_ context.Context, _, _ int64) ([]*gl.MergeRequestDiff, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) GetMRDiffRefs(_ context.Context, _, _ int64) (string, string, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) GetRawFileContent(_ context.Context, _ int64, _, _ string) ([]byte, error) {
	panic("not implemented")
}

// --- MRWriter stubs ---
func (f *fakeDescriptionClient) GetProjectMembers(_ context.Context, _ int64, _ int) ([]*gl.ProjectMember, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) CreateMRApprovalRule(
	_ context.Context, _, _ int64, _ pkggitlab.MRApprovalRulePayload,
) (*gl.MergeRequestApprovalRule, error) {
	panic("not implemented")
}

func (f *fakeDescriptionClient) UpdateMRApprovalRule(
	_ context.Context, _, _, _ int64, _ pkggitlab.MRApprovalRulePayload,
) error {
	panic("not implemented")
}

func (f *fakeDescriptionClient) SetMRReviewers(_ context.Context, _, _ int64, _ []int64) error {
	panic("not implemented")
}

func (f *fakeDescriptionClient) ListUsersByUsername(_ context.Context, _ string) (*gl.User, error) {
	panic("not implemented")
}

func TestUpdateDescription_PassesThrough(t *testing.T) {
	c := &fakeDescriptionClient{}
	a := &GitLabAdapter{client: c}

	if err := a.UpdateDescription(context.Background(), 1, 10, "new body"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.updateCalls) != 1 || c.updateCalls[0] != "new body" {
		t.Errorf("expected UpdateMRDescription called once with %q, got %v", "new body", c.updateCalls)
	}
}

func TestUpdateDescription_PropagatesError(t *testing.T) {
	boom := errors.New("write error")
	c := &fakeDescriptionClient{updateErr: boom}
	a := &GitLabAdapter{client: c}

	err := a.UpdateDescription(context.Background(), 1, 10, "new body")
	if !errors.Is(err, boom) {
		t.Errorf("expected wrapped write error, got %v", err)
	}
}
