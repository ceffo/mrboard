package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	userErin      = "erin"
	testRepoRule  = "orekit-stelar"
	testRuleLeads = "leads"
	testRuleTeam  = "team"
)

func ruledMR() MergeRequest {
	return MergeRequest{
		Reviewers: []ReviewerInfo{{Username: testUsername}, {Username: testOther}, {Username: userDave}},
		ApproverRules: []ApproverRule{
			{Name: testRepoRule, Approvers: []string{userDave, userErin}, ApprovalsRequired: 2},
			{Name: ManagedApproverRuleName, Approvers: []string{testOther, testUsername}, ApprovalsRequired: 2, Managed: true},
			{Name: "Approvers", Approvers: []string{userErin}, ApprovalsRequired: 1},
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

func TestLockedApprovers_OptionalRulesLockNoOne(t *testing.T) {
	mr := MergeRequest{ApproverRules: []ApproverRule{
		{Name: testRepoRule, Approvers: []string{userDave, userErin}, ApprovalsRequired: 1},
		{Name: testRuleLeads, Approvers: []string{userCarol}, ApprovalsRequired: 1},
		{Name: "unreachable", Approvers: []string{userDave}, ApprovalsRequired: 2},
		{Name: "advisory", Approvers: []string{userErin}, ApprovalsRequired: 0},
	}}

	assert.Equal(t, map[string][]string{userCarol: {testRuleLeads}}, mr.LockedApprovers(),
		"only a rule requiring every member locks its members")
}

func TestSetManagedApprovers_ReplacesOnlyTheManagedRule(t *testing.T) {
	mr := ruledMR()

	mr.SetManagedApprovers([]string{testOther})

	require.Len(t, mr.ApproverRules, 3)
	assert.Equal(t, []string{userDave, userErin}, mr.ApproverRules[0].Approvers, "unmanaged rules are untouched")
	assert.Equal(t, []string{testOther}, mr.ApproverRules[1].Approvers)
	assert.Equal(t, []string{userErin}, mr.ApproverRules[2].Approvers)
	assert.Equal(t, []string{testOther, userDave, userErin}, mr.Approvers,
		"approvers are the union of every all-required rule")
	flags := map[string]bool{}
	for _, r := range mr.Reviewers {
		flags[r.Username] = r.IsApprover
	}
	assert.Equal(t, map[string]bool{testUsername: false, testOther: true, userDave: true}, flags)
}

func TestSetManagedApprovers_AddsManagedRuleWhenMissing(t *testing.T) {
	mr := MergeRequest{
		Reviewers:     []ReviewerInfo{{Username: testUsername}},
		ApproverRules: []ApproverRule{{Name: testRepoRule, Approvers: []string{userDave}, ApprovalsRequired: 1}},
	}

	mr.SetManagedApprovers([]string{testUsername})

	require.Len(t, mr.ApproverRules, 2)
	assert.Equal(t, ApproverRule{
		Name: ManagedApproverRuleName, Approvers: []string{testUsername}, ApprovalsRequired: 1, Managed: true,
	}, mr.ApproverRules[1])
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

func TestRequiredApprovers(t *testing.T) {
	rule := func(name string, required int, members ...string) ApproverRule {
		return ApproverRule{Name: name, Approvers: members, ApprovalsRequired: required}
	}
	tests := []struct {
		name  string
		rules []ApproverRule
		want  []string
	}{
		{"no rules", nil, nil},
		{
			"every member required",
			[]ApproverRule{rule(testRuleLeads, 2, testUsername, testOther)},
			[]string{testUsername, testOther},
		},
		{"one of several required", []ApproverRule{rule(testRuleTeam, 1, testUsername, testOther, userCarol)}, nil},
		{"more required than members", []ApproverRule{rule(testRuleTeam, 3, testUsername, testOther)}, nil},
		{"no approval required", []ApproverRule{rule(testRuleTeam, 0, testUsername)}, nil},
		{"rule without members", []ApproverRule{rule("empty", 0)}, nil},
		{
			"member of a required and an optional rule",
			[]ApproverRule{rule(testRuleLeads, 1, testUsername), rule(testRuleTeam, 1, testUsername, testOther)},
			[]string{testUsername},
		},
		{
			"several required rules are unioned, optional ones ignored",
			[]ApproverRule{
				rule(testRuleLeads, 2, testOther, testUsername),
				rule("security", 1, userCarol),
				rule(testRuleTeam, 2, userDave, userErin, "frank"),
				rule("dup", 1, testUsername),
			},
			[]string{testUsername, testOther, userCarol},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, RequiredApprovers(tc.rules))
		})
	}
}

func TestApplyApproverRules_FlagsOnlyRequiredReviewers(t *testing.T) {
	mr := MergeRequest{Reviewers: []ReviewerInfo{
		{Username: testUsername}, {Username: testOther, IsApprover: true}, {Username: userCarol},
	}}
	rules := []ApproverRule{
		{Name: testRuleLeads, Approvers: []string{testUsername}, ApprovalsRequired: 1},
		{Name: testRuleTeam, Approvers: []string{testOther, userCarol}, ApprovalsRequired: 1},
	}

	mr.ApplyApproverRules(rules)

	assert.Equal(t, rules, mr.ApproverRules)
	assert.Equal(t, []string{testUsername}, mr.Approvers)
	flags := map[string]bool{}
	for _, r := range mr.Reviewers {
		flags[r.Username] = r.IsApprover
	}
	assert.Equal(t, map[string]bool{testUsername: true, testOther: false, userCarol: false}, flags,
		"members of a rule needing one of two approvals are not approvers")
}

func TestSetManagedApprovers_IgnoresOptionalRules(t *testing.T) {
	mr := MergeRequest{
		Reviewers:     []ReviewerInfo{{Username: testUsername}, {Username: userDave}},
		ApproverRules: []ApproverRule{{Name: testRuleTeam, Approvers: []string{userDave, userErin}, ApprovalsRequired: 1}},
	}

	mr.SetManagedApprovers([]string{testUsername})

	assert.Equal(t, []string{testUsername}, mr.Approvers)
	assert.True(t, mr.Reviewers[0].IsApprover)
	assert.False(t, mr.Reviewers[1].IsApprover, "a member of a 1-of-2 rule is not an approver")
}
