package gitlabadpt

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// fakeJiraClient implements gitLabClient with only the methods needed for
// JIRA link injection tests. All unimplemented methods panic.
type fakeJiraClient struct {
	desc        string
	descErr     error
	updateErr   error
	updateCalls []string // captured new descriptions
}

func (f *fakeJiraClient) GetMRDescription(_ context.Context, _, _ int64) (string, error) {
	return f.desc, f.descErr
}

func (f *fakeJiraClient) UpdateMRDescription(_ context.Context, _, _ int64, desc string) error {
	f.updateCalls = append(f.updateCalls, desc)
	return f.updateErr
}

// --- MRLister stubs ---
func (f *fakeJiraClient) ListGroupMRs(_ context.Context, _, _ string) ([]*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) ListUserMRs(_ context.Context, _ string) ([]*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) ListReviewerMRs(_ context.Context, _ string) ([]*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) ListNonArchivedProjectIDs(_ context.Context, _ string) (map[int64]bool, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) IsProjectArchived(_ context.Context, _ int64) (bool, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) FetchUserMRsGraphQL(_ context.Context, _ string) ([]pkggitlab.GQLMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) FetchReviewerMRsGraphQL(_ context.Context, _ string) ([]pkggitlab.GQLMergeRequest, error) {
	panic("not implemented")
}

// --- MREnricher stubs ---
func (f *fakeJiraClient) GetMR(_ context.Context, _, _ int64) (*gl.BasicMergeRequest, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) GetMRDiscussions(_ context.Context, _, _ int64) ([]*gl.Discussion, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) GetMRApprovals(_ context.Context, _, _ int64) (*gl.MergeRequestApprovals, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) GetMRApprovalRules(_ context.Context, _, _ int64) ([]*gl.MergeRequestApprovalRule, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) GetMRDiffs(_ context.Context, _, _ int64) ([]*gl.MergeRequestDiff, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) GetMRDiffRefs(_ context.Context, _, _ int64) (string, string, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) GetRawFileContent(_ context.Context, _ int64, _, _ string) ([]byte, error) {
	panic("not implemented")
}

// --- MRWriter stubs ---
func (f *fakeJiraClient) GetProjectMembers(_ context.Context, _ int64, _ int) ([]*gl.ProjectMember, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) CreateMRApprovalRule(
	_ context.Context, _, _ int64, _ pkggitlab.MRApprovalRulePayload,
) (*gl.MergeRequestApprovalRule, error) {
	panic("not implemented")
}

func (f *fakeJiraClient) UpdateMRApprovalRule(
	_ context.Context, _, _, _ int64, _ pkggitlab.MRApprovalRulePayload,
) error {
	panic("not implemented")
}

func (f *fakeJiraClient) SetMRReviewers(_ context.Context, _, _ int64, _ []int64) error {
	panic("not implemented")
}

func (f *fakeJiraClient) ListUsersByUsername(_ context.Context, _ string) (*gl.User, error) {
	panic("not implemented")
}

// --- helpers ---

const testJiraURL = "https://jira.example.com"

func newTestAdapter(c *fakeJiraClient) *GitLabAdapter {
	return &GitLabAdapter{client: c, cfg: Config{JiraInstanceURL: testJiraURL}}
}

var discardLogger = slog.New(slog.DiscardHandler)

// --- tests ---

func TestAppendJiraLink(t *testing.T) {
	const issueKey = "OD-123"
	tests := []struct {
		name string
		desc string
		want string
	}{
		{
			name: "empty description",
			desc: "",
			want: "---\n🎫 [OD-123](https://jira.example.com/browse/OD-123) <!-- mrboard -->",
		},
		{
			name: "non-empty description",
			desc: "some body",
			want: "some body\n---\n🎫 [OD-123](https://jira.example.com/browse/OD-123) <!-- mrboard -->",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := appendJiraLink(tc.desc, testJiraURL, issueKey)
			if got != tc.want {
				t.Errorf("appendJiraLink() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMaybeInjectJiraLink_Noop_WhenMarkerPresent(t *testing.T) {
	c := &fakeJiraClient{desc: "existing body\n---\n🎫 [OD-1](url) <!-- mrboard -->"}
	a := newTestAdapter(c)

	err := a.maybeInjectJiraLink(context.Background(), 1, 10, "OD-1", discardLogger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.updateCalls) != 0 {
		t.Errorf("expected no UpdateMRDescription call, got %d", len(c.updateCalls))
	}
}

func TestMaybeInjectJiraLink_Writes_WhenMarkerAbsent(t *testing.T) {
	c := &fakeJiraClient{desc: "some description"}
	a := newTestAdapter(c)

	err := a.maybeInjectJiraLink(context.Background(), 1, 10, "OD-42", discardLogger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.updateCalls) != 1 {
		t.Fatalf("expected 1 UpdateMRDescription call, got %d", len(c.updateCalls))
	}
	want := "some description\n---\n🎫 [OD-42](https://jira.example.com/browse/OD-42) <!-- mrboard -->"
	if c.updateCalls[0] != want {
		t.Errorf("updated description = %q, want %q", c.updateCalls[0], want)
	}
}

func TestMaybeInjectJiraLink_Writes_WhenDescriptionEmpty(t *testing.T) {
	c := &fakeJiraClient{desc: ""}
	a := newTestAdapter(c)

	if err := a.maybeInjectJiraLink(context.Background(), 1, 10, "OD-1", discardLogger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.updateCalls) != 1 {
		t.Fatalf("expected 1 UpdateMRDescription call, got %d", len(c.updateCalls))
	}
}

func TestMaybeInjectJiraLink_PropagatesGetDescriptionError(t *testing.T) {
	boom := errors.New("network error")
	c := &fakeJiraClient{descErr: boom}
	a := newTestAdapter(c)

	err := a.maybeInjectJiraLink(context.Background(), 1, 10, "OD-1", discardLogger)
	if !errors.Is(err, boom) {
		t.Errorf("expected wrapped network error, got %v", err)
	}
	if len(c.updateCalls) != 0 {
		t.Errorf("expected no UpdateMRDescription call on get-desc error")
	}
}

func TestMaybeInjectJiraLink_PropagatesUpdateError(t *testing.T) {
	boom := errors.New("write error")
	c := &fakeJiraClient{desc: "body", updateErr: boom}
	a := newTestAdapter(c)

	err := a.maybeInjectJiraLink(context.Background(), 1, 10, "OD-1", discardLogger)
	if !errors.Is(err, boom) {
		t.Errorf("expected wrapped write error, got %v", err)
	}
}

func TestInjectJiraLinksBackground_NoopWhenNoURL(_ *testing.T) {
	c := &fakeJiraClient{}
	a := &GitLabAdapter{client: c, cfg: Config{JiraInstanceURL: ""}}

	mrs := []domain.MergeRequest{
		{ProjectID: 1, IID: 10, Title: "feat(OD-1): something"},
	}
	// Should not panic or call any client methods.
	a.injectJiraLinksBackground(context.Background(), mrs)
}

func TestInjectJiraLinksBackground_NoopWhenNoJiraID(_ *testing.T) {
	c := &fakeJiraClient{}
	a := newTestAdapter(c)

	mrs := []domain.MergeRequest{
		{ProjectID: 1, IID: 10, Title: "chore: update dependencies"},
	}
	// No JIRA ID in title — no goroutine fired, no client call.
	a.injectJiraLinksBackground(context.Background(), mrs)
}
