package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ruledMR() MergeRequest {
	return MergeRequest{
		Reviewers: []ReviewerInfo{{Username: "alice"}, {Username: "bob"}, {Username: "dave"}},
		ApproverRules: []ApproverRule{
			{Name: "orekit-stelar", Approvers: []string{"dave", "erin"}},
			{Name: ManagedApproverRuleName, Approvers: []string{"bob", "alice"}, Managed: true},
			{Name: "Approvers", Approvers: []string{"erin"}},
		},
	}
}

func TestManagedApprovers_ReturnsSortedManagedMembers(t *testing.T) {
	assert.Equal(t, []string{"alice", "bob"}, ruledMR().ManagedApprovers())
	assert.Nil(t, MergeRequest{}.ManagedApprovers(), "an MR without a managed rule has no managed approvers")
}

func TestLockedApprovers_MapsUnmanagedMembersToTheirRules(t *testing.T) {
	locked := ruledMR().LockedApprovers()

	assert.Equal(t, map[string][]string{
		"dave": {"orekit-stelar"},
		"erin": {"Approvers", "orekit-stelar"},
	}, locked, "managed members are never locked; rule names are sorted")
}

func TestSetManagedApprovers_ReplacesOnlyTheManagedRule(t *testing.T) {
	mr := ruledMR()

	mr.SetManagedApprovers([]string{"bob"})

	require.Len(t, mr.ApproverRules, 3)
	assert.Equal(t, []string{"dave", "erin"}, mr.ApproverRules[0].Approvers, "unmanaged rules are untouched")
	assert.Equal(t, []string{"bob"}, mr.ApproverRules[1].Approvers)
	assert.Equal(t, []string{"erin"}, mr.ApproverRules[2].Approvers)
	assert.Equal(t, []string{"bob", "dave", "erin"}, mr.Approvers, "approvers are the union of every rule")
	flags := map[string]bool{}
	for _, r := range mr.Reviewers {
		flags[r.Username] = r.IsApprover
	}
	assert.Equal(t, map[string]bool{"alice": false, "bob": true, "dave": true}, flags)
}

func TestSetManagedApprovers_AddsManagedRuleWhenMissing(t *testing.T) {
	mr := MergeRequest{
		Reviewers:     []ReviewerInfo{{Username: "alice"}},
		ApproverRules: []ApproverRule{{Name: "orekit-stelar", Approvers: []string{"dave"}}},
	}

	mr.SetManagedApprovers([]string{"alice"})

	require.Len(t, mr.ApproverRules, 2)
	assert.Equal(t, ApproverRule{Name: ManagedApproverRuleName, Approvers: []string{"alice"}, Managed: true},
		mr.ApproverRules[1])
	assert.Equal(t, []string{"alice", "dave"}, mr.Approvers)
	assert.True(t, mr.Reviewers[0].IsApprover)
}

func TestSetManagedApprovers_EmptyLeavesNilApprovers(t *testing.T) {
	mr := MergeRequest{Reviewers: []ReviewerInfo{{Username: "alice", IsApprover: true}}}

	mr.SetManagedApprovers(nil)

	assert.Nil(t, mr.Approvers)
	assert.False(t, mr.Reviewers[0].IsApprover)
}
