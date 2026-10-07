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

func TestSaveApprovers_NoManagedRule_CreatesItAndReplacesManualRules(t *testing.T) {
	c := &fakeApprovalRulesClient{rules: []*gl.MergeRequestApprovalRule{
		rule(1, "Backend", "regular"),
		rule(2, "Any", "any_approver"),
		rule(3, "Code Owners", "code_owner"),
	}}
	a := &GitLabAdapter{client: c}

	require.NoError(t, a.SaveApprovers(context.Background(), 1, 10, []int64{7, 8}))

	require.Len(t, c.created, 1)
	assert.Equal(t, approversRuleName, c.created[0].Name)
	assert.Equal(t, []int64{7, 8}, c.created[0].UserIDs)
	assert.Equal(t, []int64{1}, c.deleted, "only manual regular rules are replaced")
}

func TestSaveApprovers_ManagedRuleExists_UpdatesItAndReplacesLegacyRule(t *testing.T) {
	c := &fakeApprovalRulesClient{rules: []*gl.MergeRequestApprovalRule{
		rule(1, approversRuleName, "regular"),
		rule(2, "Approvers", "regular"),
	}}
	a := &GitLabAdapter{client: c}

	require.NoError(t, a.SaveApprovers(context.Background(), 1, 10, []int64{7}))

	assert.Empty(t, c.created)
	require.Contains(t, c.updated, int64(1))
	assert.Equal(t, []int64{2}, c.deleted)
}
