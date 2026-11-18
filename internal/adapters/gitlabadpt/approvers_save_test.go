package gitlabadpt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gl "gitlab.com/gitlab-org/api/client-go"

	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// fakeApprovalRulesClient serves a fixed rule list and records approval-rule
// writes. The embedded interface is nil: any other client call panics.
type fakeApprovalRulesClient struct {
	gitLabClient
	rules []*gl.MergeRequestApprovalRule

	created []pkggitlab.MRApprovalRulePayload
	updated map[int64]pkggitlab.MRApprovalRulePayload
	deleted []int64
}

func (f *fakeApprovalRulesClient) GetMRApprovalRules(
	_ context.Context, _, _ int64,
) ([]*gl.MergeRequestApprovalRule, error) {
	return f.rules, nil
}

func (f *fakeApprovalRulesClient) CreateMRApprovalRule(
	_ context.Context, _, _ int64, p pkggitlab.MRApprovalRulePayload,
) (*gl.MergeRequestApprovalRule, error) {
	f.created = append(f.created, p)
	return &gl.MergeRequestApprovalRule{ID: 99, Name: p.Name}, nil
}

func (f *fakeApprovalRulesClient) UpdateMRApprovalRule(
	_ context.Context, _, _, ruleID int64, p pkggitlab.MRApprovalRulePayload,
) error {
	if f.updated == nil {
		f.updated = make(map[int64]pkggitlab.MRApprovalRulePayload)
	}
	f.updated[ruleID] = p
	return nil
}

func (f *fakeApprovalRulesClient) DeleteMRApprovalRule(_ context.Context, _, _, ruleID int64) error {
	f.deleted = append(f.deleted, ruleID)
	return nil
}

func rule(id int64, name, ruleType string) *gl.MergeRequestApprovalRule {
	return &gl.MergeRequestApprovalRule{ID: id, Name: name, RuleType: ruleType}
}

// foreignRules are rules mrboard did not create: one copied from the project's
// settings, one a person added by hand, and the system-managed kinds.
func foreignRules() []*gl.MergeRequestApprovalRule {
	repo := rule(1, testRepoRule, "regular")
	repo.SourceRule = &gl.ProjectApprovalRule{ID: 500, Name: testRepoRule}
	return []*gl.MergeRequestApprovalRule{
		repo,
		rule(2, "Approvers", "regular"),
		rule(3, "Any", "any_approver"),
		rule(4, "Code Owners", "code_owner"),
	}
}

func TestSaveApprovers_NoManagedRule_CreatesItAndLeavesOtherRules(t *testing.T) {
	c := &fakeApprovalRulesClient{rules: foreignRules()}
	a := &GitLabAdapter{client: c}

	require.NoError(t, a.SaveApprovers(context.Background(), 1, 10, []int64{7, 8}))

	require.Len(t, c.created, 1)
	assert.Equal(t, approversRuleName, c.created[0].Name)
	assert.Equal(t, []int64{7, 8}, c.created[0].UserIDs)
	assert.Equal(t, 2, c.created[0].ApprovalsRequired)
	assert.Empty(t, c.updated, "no other rule is rewritten")
	assert.Empty(t, c.deleted, "no rule is deleted")
}

func TestSaveApprovers_ManagedRuleExists_UpdatesOnlyIt(t *testing.T) {
	c := &fakeApprovalRulesClient{rules: append(foreignRules(), rule(9, approversRuleName, "regular"))}
	a := &GitLabAdapter{client: c}

	require.NoError(t, a.SaveApprovers(context.Background(), 1, 10, []int64{7}))

	assert.Empty(t, c.created)
	require.Len(t, c.updated, 1)
	require.Contains(t, c.updated, int64(9))
	assert.Equal(t, []int64{7}, c.updated[9].UserIDs)
	assert.Equal(t, 1, c.updated[9].ApprovalsRequired)
	assert.Empty(t, c.deleted, "no rule is deleted")
}

func TestSaveApprovers_NonRegularRuleWithManagedName_IsNotWritten(t *testing.T) {
	// Only a regular rule named approvers_mrboard is mrboard's: the read marks
	// no other kind managed, so the write must not update it either.
	c := &fakeApprovalRulesClient{rules: append(foreignRules(), rule(9, approversRuleName, "code_owner"))}
	a := &GitLabAdapter{client: c}

	require.NoError(t, a.SaveApprovers(context.Background(), 1, 10, []int64{7}))

	require.Len(t, c.created, 1, "a regular managed rule is created instead")
	assert.Empty(t, c.updated, "the code-owner rule is left alone")
	assert.Empty(t, c.deleted)
}
