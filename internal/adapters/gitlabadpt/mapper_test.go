package gitlabadpt

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

func ptr[T any](v T) *T { return &v }

var (
	t0 = time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 = t0.Add(1 * time.Hour)
	t2 = t0.Add(2 * time.Hour)
	t3 = t0.Add(3 * time.Hour)
)

// Shared test reviewer/author identities, reused across mapper_test.go,
// merge_test.go, and gitlabadpt_test.go.
const (
	testUserAlice     = "alice"
	testUserAliceName = "Alice"
	testUserBob       = "bob"
	testUserCarol     = "carol"
	testUserBobName   = "Bob"
	testUserPriya     = "priya"
	testRepoRule      = "orekit-stelar"
	testRuleLeads     = "leads"
	testRuleTeam      = "team"
)

func basicUser(username, name string) *gl.BasicUser {
	return &gl.BasicUser{Username: username, Name: name}
}

func systemNote(body string, at time.Time) *gl.Note {
	return &gl.Note{
		System:    true,
		Body:      body,
		CreatedAt: ptr(at),
		Author:    gl.NoteAuthor{},
	}
}

func userNote(username string, at time.Time) *gl.Note {
	n := &gl.Note{
		System:    false,
		Body:      "some comment",
		CreatedAt: ptr(at),
	}
	n.Author.Username = username
	return n
}

func discussion(notes ...*gl.Note) *gl.Discussion {
	return &gl.Discussion{Notes: notes}
}

func resolvedDiscussion(notes ...*gl.Note) *gl.Discussion {
	if len(notes) > 0 {
		notes[0].Resolvable = true
		notes[0].Resolved = true
	}
	return &gl.Discussion{Notes: notes}
}

func approvals(usernames ...string) *gl.MergeRequestApprovals {
	var approved []*gl.MergeRequestApproverUser
	for _, u := range usernames {
		approved = append(approved, &gl.MergeRequestApproverUser{
			User: basicUser(u, u),
		})
	}
	return &gl.MergeRequestApprovals{
		ApprovedBy:        approved,
		ApprovalsRequired: int64(len(usernames)),
		ApprovalsLeft:     0,
	}
}

func mr(reviewers ...*gl.BasicUser) *gl.BasicMergeRequest {
	return &gl.BasicMergeRequest{
		ID:        1,
		IID:       1,
		ProjectID: 10,
		Title:     "Test MR",
		Reviewers: reviewers,
		CreatedAt: ptr(t0),
		Author:    basicUser("author", "Author"),
	}
}

func TestDeriveReviewerStates_NotStarted(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	result := DeriveReviewerStates(m, nil, approvals())

	require.Len(t, result, 1, "want 1 reviewer (not-started included)")
	assert.Equal(t, domain.ReviewerNotStarted, result[0].State)
}

func TestDeriveReviewerStates_NewlyAssignedNoComment(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	discussions := []*gl.Discussion{
		discussion(systemNote("requested review from @alice", t1)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	assert.Equal(t, domain.ReviewerNotStarted, result[0].State)
}

func TestDeriveReviewerStates_Commented(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	discussions := []*gl.Discussion{
		discussion(systemNote("requested review from @alice", t1)),
		discussion(userNote(testUserAlice, t2)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	assert.Equal(t, domain.ReviewerCommented, result[0].State)
}

func TestDeriveReviewerStates_ReReviewRequested(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	discussions := []*gl.Discussion{
		discussion(userNote(testUserAlice, t1)),
		discussion(systemNote("requested review from @alice", t2)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	assert.Equal(t, domain.ReviewerReReviewRequested, result[0].State)
}

func TestDeriveReviewerStates_Approved(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	discussions := []*gl.Discussion{
		discussion(userNote(testUserAlice, t1)),
	}

	result := DeriveReviewerStates(m, discussions, approvals(testUserAlice))

	assert.Equal(t, domain.ReviewerApproved, result[0].State)
}

func TestDeriveReviewerStates_MultipleReviewers(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName), basicUser(testUserBob, testUserBobName))
	discussions := []*gl.Discussion{
		discussion(systemNote("requested review from @alice", t1)),
		discussion(userNote(testUserAlice, t2)),
		discussion(userNote(testUserBob, t1)),
		discussion(systemNote("requested review from @bob", t3)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	stateFor := func(username string) domain.ReviewerState {
		for _, r := range result {
			if r.Username == username {
				return r.State
			}
		}
		require.Failf(t, "reviewer %q not found", username)
		return domain.ReviewerNotStarted
	}

	assert.Equal(t, domain.ReviewerCommented, stateFor(testUserAlice), testUserAlice)
	assert.Equal(t, domain.ReviewerReReviewRequested, stateFor(testUserBob), testUserBob)
}

func TestDeriveReviewerStates_NonReviewerNotesIgnored(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	discussions := []*gl.Discussion{
		discussion(userNote("not-a-reviewer", t1)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	require.Len(t, result, 1, "want 1 reviewer (alice not-started, included)")
	assert.Equal(t, domain.ReviewerNotStarted, result[0].State)
}

func TestDeriveReviewerStates_ResolvedThreadNotCommented(t *testing.T) {
	// Reviewer left a comment, but the thread was resolved by the author.
	// Should NOT stay in ReviewerCommented — that would wrongly put the MR
	// in NeedsAuthorAction even though there's nothing left to address.
	m := mr(basicUser(testUserAlice, testUserAliceName))
	discussions := []*gl.Discussion{
		resolvedDiscussion(userNote(testUserAlice, t1)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	assert.Equal(t, domain.ReviewerNotStarted, result[0].State, "thread resolved")
}

func TestDeriveReviewerStates_UnresolvedThreadStillCommented(t *testing.T) {
	// Reviewer has one resolved thread and one open thread — still Commented.
	m := mr(basicUser(testUserAlice, testUserAliceName))
	discussions := []*gl.Discussion{
		resolvedDiscussion(userNote(testUserAlice, t1)),
		discussion(userNote(testUserAlice, t2)), // unresolved
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	assert.Equal(t, domain.ReviewerCommented, result[0].State, "unresolved thread remains")
}

func TestDeriveReviewerStates_NoReviewers(t *testing.T) {
	m := mr()
	result := DeriveReviewerStates(m, nil, approvals())
	assert.Nil(t, result, "want nil for no reviewers")
}

func TestCountRoundTripsFromEvents(t *testing.T) {
	cases := []struct {
		name        string
		discussions []*gl.Discussion
		want        int
	}{
		{
			name:        "no discussions",
			discussions: nil,
			want:        0,
		},
		{
			name: "no re-review notes",
			discussions: []*gl.Discussion{
				discussion(userNote(testUserAlice, t1)),
				discussion(systemNote("assigned to @alice", t1)),
			},
			want: 0,
		},
		{
			name: "single re-review",
			discussions: []*gl.Discussion{
				discussion(systemNote("requested review from @alice", t1)),
			},
			want: 1,
		},
		{
			name: "multiple re-reviews same reviewer",
			discussions: []*gl.Discussion{
				discussion(systemNote("requested review from @alice", t1)),
				discussion(systemNote("requested review from @alice", t2)),
				discussion(systemNote("requested review from @alice", t3)),
			},
			want: 3,
		},
		{
			name: "multiple reviewers, mixed notes",
			discussions: []*gl.Discussion{
				discussion(
					systemNote("requested review from @alice", t1),
					systemNote("requested review from @bob", t2),
				),
				discussion(userNote(testUserAlice, t3)),
				discussion(systemNote("requested review from @alice", t3)),
			},
			want: 3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := normalizeDiscussionEventsREST(tc.discussions)
			assert.Equal(t, tc.want, domain.CountRoundTrips(events))
		})
	}
}

func TestMapMR_RoundTripCount(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	discussions := []*gl.Discussion{
		discussion(systemNote("requested review from @alice", t1)),
		discussion(userNote(testUserAlice, t2)),
		discussion(systemNote("requested review from @alice", t3)),
	}
	result := MapMR(m, discussions, approvals(), nil)
	assert.Equal(t, 2, result.RoundTripCount)
}

// approvalRule is a regular rule that needs every one of usernames to approve.
func approvalRule(name string, usernames ...string) *gl.MergeRequestApprovalRule {
	return approvalRuleNeeding(name, len(usernames), usernames...)
}

// approvalRuleNeeding is a regular rule that needs required approvals from usernames.
func approvalRuleNeeding(name string, required int, usernames ...string) *gl.MergeRequestApprovalRule {
	eligible := make([]*gl.BasicUser, len(usernames))
	for i, u := range usernames {
		eligible[i] = basicUser(u, u)
	}
	return &gl.MergeRequestApprovalRule{
		Name: name, RuleType: ruleTypeRegular, ApprovalsRequired: int64(required), EligibleApprovers: eligible,
	}
}

func TestMapMR_IsApprover_InApproversRule(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName), basicUser(testUserBob, testUserBobName))
	rules := []*gl.MergeRequestApprovalRule{approvalRule("Approvers", testUserAlice)}
	result := MapMR(m, nil, approvals(), rules)
	for _, r := range result.Reviewers {
		if r.Username == testUserAlice {
			assert.True(t, r.IsApprover, "alice should be IsApprover=true")
		}
		if r.Username == testUserBob {
			assert.False(t, r.IsApprover, "bob should be IsApprover=false")
		}
	}
}

func TestMapMR_IsApprover_NoApproversRule(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	result := MapMR(m, nil, approvals(), nil)
	for _, r := range result.Reviewers {
		assert.False(t, r.IsApprover, "want IsApprover=false when no Approvers rule, got true for %s", r.Username)
	}
}

func TestMapMR_Approvers_IncludesNonReviewerEligibleApprovers(t *testing.T) {
	// carol is eligible to approve per the rule but hasn't been added as a
	// reviewer yet — Approvers must still include her, unlike IsApprover
	// which only flags entries already present in Reviewers.
	m := mr(basicUser(testUserAlice, testUserAliceName))
	rules := []*gl.MergeRequestApprovalRule{approvalRule("Approvers", testUserAlice, testUserCarol)}
	result := MapMR(m, nil, approvals(), rules)
	assert.ElementsMatch(t, []string{testUserAlice, testUserCarol}, result.Approvers)
}

func TestMapMR_DetailedMergeStatus_Stored(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	m.DetailedMergeStatus = detailedMergeStatusMergeable
	result := MapMR(m, nil, approvals(), nil)
	assert.Equal(t, detailedMergeStatusMergeable, result.DetailedMergeStatus,
		"want DetailedMergeStatus stored on domain MR")
}

func TestMapMR_DetailedMergeStatus_Stored_NonMergeable(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	m.DetailedMergeStatus = "ci_must_pass"
	result := MapMR(m, nil, approvals(), nil)
	assert.Equal(t, "ci_must_pass", result.DetailedMergeStatus, "want DetailedMergeStatus=ci_must_pass stored")
}

func TestMapMRFromGraphQL_DetailedMergeStatus_NormalizedToLowercase(t *testing.T) {
	mr := pkggitlab.GQLMergeRequest{}
	mr.DetailedMergeStatus = "MERGEABLE"
	result := MapMRFromGraphQL(mr)
	assert.Equal(t, detailedMergeStatusMergeable, result.DetailedMergeStatus, "want DetailedMergeStatus normalized")
}

func TestMapMRFromGraphQL_DetailedMergeStatus_NonMergeable_Normalized(t *testing.T) {
	mr := pkggitlab.GQLMergeRequest{}
	mr.DetailedMergeStatus = "CI_MUST_PASS"
	result := MapMRFromGraphQL(mr)
	const wantStatus = "ci_must_pass"
	assert.Equal(t, wantStatus, result.DetailedMergeStatus, "want DetailedMergeStatus normalized")
}

func TestMapMR_SourceTargetBranch_Stored(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	m.SourceBranch = "feature/foo"
	m.TargetBranch = "main"
	result := MapMR(m, nil, approvals(), nil)
	assert.Equal(t, "feature/foo", result.SourceBranch, "want SourceBranch stored on domain MR")
	assert.Equal(t, "main", result.TargetBranch, "want TargetBranch stored on domain MR")
}

func TestMapMRFromGraphQL_SourceTargetBranch_Stored(t *testing.T) {
	mr := pkggitlab.GQLMergeRequest{}
	mr.SourceBranch = "feature/foo"
	mr.TargetBranch = "main"
	result := MapMRFromGraphQL(mr)
	assert.Equal(t, "feature/foo", result.SourceBranch, "want SourceBranch stored on domain MR")
	assert.Equal(t, "main", result.TargetBranch, "want TargetBranch stored on domain MR")
}

func TestMapMR_UpdatedAt_Stored(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	m.UpdatedAt = ptr(t1)
	result := MapMR(m, nil, approvals(), nil)
	assert.True(t, t1.Equal(result.UpdatedAt), "want UpdatedAt stored on domain MR")
}

func TestMapMRFromGraphQL_UpdatedAt_Stored(t *testing.T) {
	mr := pkggitlab.GQLMergeRequest{}
	mr.UpdatedAt = t1.Format(time.RFC3339)
	result := MapMRFromGraphQL(mr)
	assert.True(t, t1.Equal(result.UpdatedAt), "want UpdatedAt parsed and stored on domain MR")
}

func TestMapMR_PhaseReadyToMerge_WhenAllApproversApproved(t *testing.T) {
	// alice is in the Approvers rule and has approved
	m := mr(basicUser(testUserAlice, testUserAliceName))
	rules := []*gl.MergeRequestApprovalRule{approvalRule("Approvers", testUserAlice)}
	result := MapMR(m, nil, approvals(testUserAlice), rules)
	assert.Equal(t, domain.PhaseReadyToMerge, result.Phase, "want PhaseReadyToMerge when all approvers approved")
}

func TestExtractReReviewUsername(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{"requested review from @alice", testUserAlice},
		{"requested review from @bob.smith", "bob.smith"},
		{"assigned to @alice", ""},
		{"", ""},
		{"requested review from @", ""},
	}
	for _, tc := range cases {
		got := extractReReviewUsername(tc.body)
		assert.Equal(t, tc.want, got, "body=%q", tc.body)
	}
}

func TestMapMR_Approvers_UnionsEveryRuleRegardlessOfName(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName), basicUser(testUserBob, testUserBobName))
	rules := []*gl.MergeRequestApprovalRule{
		approvalRule("Backend approvers", testUserAlice),
		approvalRule("Security", testUserAlice, testUserCarol),
		approvalRule("Any approver"),
	}
	result := MapMR(m, nil, approvals(), rules)
	assert.ElementsMatch(t, []string{testUserAlice, testUserCarol}, result.Approvers)
	for _, r := range result.Reviewers {
		assert.Equal(t, r.Username == testUserAlice, r.IsApprover, "IsApprover for %s", r.Username)
	}
}

func TestMapMR_Approvers_IgnoresSystemManagedRules(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName), basicUser(testUserBob, testUserBobName))
	codeOwners := approvalRule("*", testUserBob, testUserCarol)
	codeOwners.RuleType = "code_owner"
	rules := []*gl.MergeRequestApprovalRule{approvalRule("Approvers", testUserAlice), codeOwners}
	result := MapMR(m, nil, approvals(), rules)
	assert.Equal(t, []string{testUserAlice}, result.Approvers)
	for _, r := range result.Reviewers {
		assert.Equal(t, r.Username == testUserAlice, r.IsApprover, "IsApprover for %s", r.Username)
	}
}

func TestMapMRFromGraphQL_Approvers_IgnoresSystemManagedRules(t *testing.T) {
	codeOwners := pkggitlab.GQLApprovalRule{
		Name:              "*",
		Type:              "CODE_OWNER",
		EligibleApprovers: []pkggitlab.GQLUser{{Username: testUserCarol}},
	}
	anyApprover := pkggitlab.GQLApprovalRule{Name: "All Members", Type: "ANY_APPROVER"}
	regular := pkggitlab.GQLApprovalRule{
		Name:              "approvers",
		Type:              gqlRuleTypeRegular,
		EligibleApprovers: []pkggitlab.GQLUser{{Username: testUserAlice}},
	}
	regular.ApprovalsRequired = 1
	codeOwners.ApprovalsRequired = 1
	mr := pkggitlab.GQLMergeRequest{}
	mr.ApprovalState.Rules = []pkggitlab.GQLApprovalRule{codeOwners, anyApprover, regular}
	assert.Equal(t, []string{testUserAlice}, MapMRFromGraphQL(mr).Approvers)
}

func TestMapMR_ApproverRules_MarksOnlyTheManagedRule(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	codeOwners := approvalRule("*", testUserBob)
	codeOwners.RuleType = "code_owner"
	rules := []*gl.MergeRequestApprovalRule{
		approvalRule(testRepoRule, testUserCarol, testUserAlice),
		approvalRule(approversRuleName, testUserBob),
		codeOwners,
	}
	result := MapMR(m, nil, approvals(), rules)
	assert.Equal(t, []domain.ApproverRule{
		{Name: testRepoRule, Approvers: []string{testUserAlice, testUserCarol}, ApprovalsRequired: 2},
		{Name: approversRuleName, Approvers: []string{testUserBob}, ApprovalsRequired: 1, Managed: true},
	}, result.ApproverRules)
	assert.Equal(t, []string{testUserBob}, result.ManagedApprovers())
	assert.Equal(t, map[string][]string{testUserAlice: {testRepoRule}, testUserCarol: {testRepoRule}},
		result.LockedApprovers())
}

func TestMergeMRFromGraphQL_ApproverRules_FromFreshRules(t *testing.T) {
	mr := pkggitlab.GQLMergeRequest{}
	mr.ApprovalState.Rules = []pkggitlab.GQLApprovalRule{
		gqlApprovalRule(testRepoRule, testUserCarol),
		gqlApprovalRule(approversRuleName, testUserAlice),
	}
	result := MergeMRFromGraphQL(mr, domain.MergeRequest{})
	assert.Equal(t, []domain.ApproverRule{
		{Name: testRepoRule, Approvers: []string{testUserCarol}, ApprovalsRequired: 1},
		{Name: approversRuleName, Approvers: []string{testUserAlice}, ApprovalsRequired: 1, Managed: true},
	}, result.ApproverRules)
}

func TestMapMR_ApproverRules_CarriesManagedMemberIDs(t *testing.T) {
	m := mr(basicUser(testUserAlice, testUserAliceName))
	managed := approvalRule(approversRuleName, testUserBob)
	managed.EligibleApprovers[0].ID = 42
	result := MapMR(m, nil, approvals(), []*gl.MergeRequestApprovalRule{managed})
	assert.Equal(t, map[string]int64{testUserBob: 42}, result.ManagedApproverIDs())
}

// requiredApproverCases are the rule sets the required-approver definition is
// tested against, keyed by the reviewers alice, bob and carol.
var requiredApproverCases = []struct {
	name  string
	rules []testRule
	want  []string
}{
	{
		"every member required",
		[]testRule{{testRuleLeads, 2, []string{testUserAlice, testUserBob}}},
		[]string{testUserAlice, testUserBob},
	},
	{"one of several required", []testRule{{testRuleTeam, 1, []string{testUserAlice, testUserBob, testUserCarol}}}, nil},
	{"more required than members", []testRule{{testRuleTeam, 3, []string{testUserAlice, testUserBob}}}, nil},
	{"no approval required", []testRule{{testRuleTeam, 0, []string{testUserAlice}}}, nil},
	{
		"member of a required and an optional rule",
		[]testRule{{testRuleLeads, 1, []string{testUserAlice}}, {testRuleTeam, 1, []string{testUserAlice, testUserBob}}},
		[]string{testUserAlice},
	},
	{
		"several rules",
		[]testRule{
			{testRuleLeads, 1, []string{testUserAlice}},
			{"security", 1, []string{testUserCarol}},
			{testRuleTeam, 1, []string{testUserBob, testUserCarol}},
		},
		[]string{testUserAlice, testUserCarol},
	},
}

type testRule struct {
	name     string
	required int
	members  []string
}

func (r testRule) rest() *gl.MergeRequestApprovalRule {
	return approvalRuleNeeding(r.name, r.required, r.members...)
}

func (r testRule) gql() pkggitlab.GQLApprovalRule {
	rule := gqlApprovalRule(r.name, r.members...)
	rule.ApprovalsRequired = r.required
	return rule
}

// assertRequiredApprovers checks mr's Approvers and the IsApprover flags of
// its reviewers alice, bob and carol against want.
func assertRequiredApprovers(t *testing.T, want []string, mr domain.MergeRequest) {
	t.Helper()
	assert.Equal(t, want, mr.Approvers)
	require.Len(t, mr.Reviewers, 3)
	for _, r := range mr.Reviewers {
		assert.Equal(t, slices.Contains(want, r.Username), r.IsApprover, "IsApprover for %s", r.Username)
	}
}

func TestMapMR_RequiredApprovers(t *testing.T) {
	for _, tc := range requiredApproverCases {
		t.Run(tc.name, func(t *testing.T) {
			m := mr(basicUser(testUserAlice, testUserAliceName), basicUser(testUserBob, testUserBobName),
				basicUser(testUserCarol, testUserCarol))
			var rules []*gl.MergeRequestApprovalRule
			for _, r := range tc.rules {
				rules = append(rules, r.rest())
			}
			assertRequiredApprovers(t, tc.want, MapMR(m, nil, approvals(), rules))
		})
	}
}

func gqlMRWithRules(rules []testRule) pkggitlab.GQLMergeRequest {
	mr := pkggitlab.GQLMergeRequest{}
	for _, u := range []string{testUserAlice, testUserBob, testUserCarol} {
		mr.Reviewers.Nodes = append(mr.Reviewers.Nodes, pkggitlab.GQLUser{Username: u, Name: u})
	}
	for _, r := range rules {
		mr.ApprovalState.Rules = append(mr.ApprovalState.Rules, r.gql())
	}
	return mr
}

// TestMapMRFromGraphQL_RequiredApprovers covers the fat and thin open-MR reads
// and the merged-MR read, which all map through MapMRFromGraphQL.
func TestMapMRFromGraphQL_RequiredApprovers(t *testing.T) {
	for _, tc := range requiredApproverCases {
		t.Run(tc.name, func(t *testing.T) {
			assertRequiredApprovers(t, tc.want, MapMRFromGraphQL(gqlMRWithRules(tc.rules)))
		})
	}
}

func TestMergeMRFromGraphQL_RequiredApprovers(t *testing.T) {
	for _, tc := range requiredApproverCases {
		t.Run(tc.name, func(t *testing.T) {
			cached := domain.MergeRequest{Reviewers: []domain.ReviewerInfo{
				{Username: testUserAlice, IsApprover: true},
				{Username: testUserBob, IsApprover: true},
				{Username: testUserCarol, IsApprover: true},
			}}
			assertRequiredApprovers(t, tc.want, MergeMRFromGraphQL(gqlMRWithRules(tc.rules), cached))
		})
	}
}

func TestMapMRFromGraphQL_OptionalApproverDoesNotHoldBackReadyToMerge(t *testing.T) {
	mr := gqlMRWithRules([]testRule{
		{testRuleLeads, 1, []string{testUserAlice}},
		{testRuleTeam, 1, []string{testUserBob, testUserCarol}},
	})
	mr.ApprovedBy.Nodes = []pkggitlab.GQLUser{{Username: testUserAlice}}

	assert.Equal(t, domain.PhaseReadyToMerge, MapMRFromGraphQL(mr).Phase,
		"bob and carol are optional: alice's approval is the only one required")
}
