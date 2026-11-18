package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	userErin     = "erin"
	testRepoRule = "orekit-stelar"
)

func ruledMR() MergeRequest {
	return MergeRequest{
		Reviewers: []ReviewerInfo{{Username: testUsername}, {Username: testOther}, {Username: userDave}},
		ApproverRules: []ApproverRule{
			{Name: testRepoRule, Approvers: []string{userDave, userErin}},
			{Name: ManagedApproverRuleName, Approvers: []string{testOther, testUsername}, Managed: true},
			{Name: "Approvers", Approvers: []string{userErin}},
		},
	}
}

func TestManagedApprovers_ReturnsSortedManagedMembers(t *testing.T) {
	assert.Equal(t, []string{testUsername, testOther}, ruledMR().ManagedApprovers())
	assert.Nil(t, MergeRequest{}.ManagedApprovers(), "an MR without a managed rule has no managed approvers")
}

func TestLockedApprovers_MapsUnmanagedMembersToTheirRules(t *testing.T) {
	locked := ruledMR().LockedApprovers()

	assert.Equal(t, map[string][]string{
		userDave: {testRepoRule},
		userErin: {"Approvers", testRepoRule},
	}, locked, "managed members are never locked; rule names are sorted")
}

func TestSetManagedApprovers_ReplacesOnlyTheManagedRule(t *testing.T) {
	mr := ruledMR()

	mr.SetManagedApprovers([]string{testOther})

	require.Len(t, mr.ApproverRules, 3)
	assert.Equal(t, []string{userDave, userErin}, mr.ApproverRules[0].Approvers, "unmanaged rules are untouched")
	assert.Equal(t, []string{testOther}, mr.ApproverRules[1].Approvers)
	assert.Equal(t, []string{userErin}, mr.ApproverRules[2].Approvers)
	assert.Equal(t, []string{testOther, userDave, userErin}, mr.Approvers, "approvers are the union of every rule")
	flags := map[string]bool{}
	for _, r := range mr.Reviewers {
		flags[r.Username] = r.IsApprover
	}
	assert.Equal(t, map[string]bool{testUsername: false, testOther: true, userDave: true}, flags)
}

func TestSetManagedApprovers_AddsManagedRuleWhenMissing(t *testing.T) {
	mr := MergeRequest{
		Reviewers:     []ReviewerInfo{{Username: testUsername}},
		ApproverRules: []ApproverRule{{Name: testRepoRule, Approvers: []string{userDave}}},
	}

	mr.SetManagedApprovers([]string{testUsername})

	require.Len(t, mr.ApproverRules, 2)
	assert.Equal(t, ApproverRule{Name: ManagedApproverRuleName, Approvers: []string{testUsername}, Managed: true},
		mr.ApproverRules[1])
	assert.Equal(t, []string{testUsername, userDave}, mr.Approvers)
	assert.True(t, mr.Reviewers[0].IsApprover)
}

func TestSetManagedApprovers_EmptyLeavesNilApprovers(t *testing.T) {
	mr := MergeRequest{Reviewers: []ReviewerInfo{{Username: testUsername, IsApprover: true}}}

	mr.SetManagedApprovers(nil)

	assert.Nil(t, mr.Approvers)
	assert.False(t, mr.Reviewers[0].IsApprover)
}

func TestSetManagedApprovers_KeepsIDsOfRemainingMembersOnly(t *testing.T) {
	mr := ruledMR()
	mr.ApproverRules[1].UserIDs = map[string]int64{testUsername: 1, testOther: 2}

	mr.SetManagedApprovers([]string{testOther, "frank"})

	assert.Equal(t, map[string]int64{testOther: 2}, mr.ManagedApproverIDs())
}
