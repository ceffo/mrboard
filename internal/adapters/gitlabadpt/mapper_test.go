package gitlabadpt

import (
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
	m := mr(basicUser("alice", "Alice"))
	result := DeriveReviewerStates(m, nil, approvals())

	require.Len(t, result, 1, "want 1 reviewer (not-started included)")
	assert.Equal(t, domain.ReviewerNotStarted, result[0].State)
}

func TestDeriveReviewerStates_Commented(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(systemNote("requested review from @alice", t1)),
		discussion(userNote("alice", t2)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	assert.Equal(t, domain.ReviewerCommented, result[0].State)
}

func TestDeriveReviewerStates_ReReviewRequested(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(userNote("alice", t1)),
		discussion(systemNote("requested review from @alice", t2)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	assert.Equal(t, domain.ReviewerReReviewRequested, result[0].State)
}

func TestDeriveReviewerStates_Approved(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(userNote("alice", t1)),
	}

	result := DeriveReviewerStates(m, discussions, approvals("alice"))

	assert.Equal(t, domain.ReviewerApproved, result[0].State)
}

func TestDeriveReviewerStates_MultipleReviewers(t *testing.T) {
	m := mr(basicUser("alice", "Alice"), basicUser("bob", "Bob"))
	discussions := []*gl.Discussion{
		discussion(systemNote("requested review from @alice", t1)),
		discussion(userNote("alice", t2)),
		discussion(userNote("bob", t1)),
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

	assert.Equal(t, domain.ReviewerCommented, stateFor("alice"), "alice")
	assert.Equal(t, domain.ReviewerReReviewRequested, stateFor("bob"), "bob")
}

func TestDeriveReviewerStates_NonReviewerNotesIgnored(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
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
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		resolvedDiscussion(userNote("alice", t1)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	assert.Equal(t, domain.ReviewerNotStarted, result[0].State, "thread resolved")
}

func TestDeriveReviewerStates_UnresolvedThreadStillCommented(t *testing.T) {
	// Reviewer has one resolved thread and one open thread — still Commented.
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		resolvedDiscussion(userNote("alice", t1)),
		discussion(userNote("alice", t2)), // unresolved
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
				discussion(userNote("alice", t1)),
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
				discussion(userNote("alice", t3)),
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
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(systemNote("requested review from @alice", t1)),
		discussion(userNote("alice", t2)),
		discussion(systemNote("requested review from @alice", t3)),
	}
	result := MapMR(m, discussions, approvals(), nil)
	assert.Equal(t, 2, result.RoundTripCount)
}

func approvalRule(name string, usernames ...string) *gl.MergeRequestApprovalRule {
	eligible := make([]*gl.BasicUser, len(usernames))
	for i, u := range usernames {
		eligible[i] = basicUser(u, u)
	}
	return &gl.MergeRequestApprovalRule{Name: name, EligibleApprovers: eligible}
}

func TestMapMR_IsApprover_InApproversRule(t *testing.T) {
	m := mr(basicUser("alice", "Alice"), basicUser("bob", "Bob"))
	rules := []*gl.MergeRequestApprovalRule{approvalRule("Approvers", "alice")}
	result := MapMR(m, nil, approvals(), rules)
	for _, r := range result.Reviewers {
		if r.Username == "alice" {
			assert.True(t, r.IsApprover, "alice should be IsApprover=true")
		}
		if r.Username == "bob" {
			assert.False(t, r.IsApprover, "bob should be IsApprover=false")
		}
	}
}

func TestMapMR_IsApprover_NoApproversRule(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	result := MapMR(m, nil, approvals(), nil)
	for _, r := range result.Reviewers {
		assert.False(t, r.IsApprover, "want IsApprover=false when no Approvers rule, got true for %s", r.Username)
	}
}

func TestMapMR_DetailedMergeStatus_Stored(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	m.DetailedMergeStatus = detailedMergeStatusMergeable
	result := MapMR(m, nil, approvals(), nil)
	assert.Equal(t, detailedMergeStatusMergeable, result.DetailedMergeStatus,
		"want DetailedMergeStatus stored on domain MR")
}

func TestMapMR_DetailedMergeStatus_Stored_NonMergeable(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	m.DetailedMergeStatus = "ci_must_pass"
	result := MapMR(m, nil, approvals(), nil)
	assert.Equal(t, "ci_must_pass", result.DetailedMergeStatus, "want DetailedMergeStatus=ci_must_pass stored")
}

func TestMapMRFromGraphQL_DetailedMergeStatus_NormalizedToLowercase(t *testing.T) {
	mr := pkggitlab.GQLMergeRequest{}
	mr.DetailedMergeStatus = "MERGEABLE"
	result := MapMRFromGraphQL(mr)
	assert.Equal(t, "mergeable", result.DetailedMergeStatus, "want DetailedMergeStatus normalized")
}

func TestMapMRFromGraphQL_DetailedMergeStatus_NonMergeable_Normalized(t *testing.T) {
	mr := pkggitlab.GQLMergeRequest{}
	mr.DetailedMergeStatus = "CI_MUST_PASS"
	result := MapMRFromGraphQL(mr)
	const wantStatus = "ci_must_pass"
	assert.Equal(t, wantStatus, result.DetailedMergeStatus, "want DetailedMergeStatus normalized")
}

func TestMapMR_SourceTargetBranch_Stored(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
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
	m := mr(basicUser("alice", "Alice"))
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
	m := mr(basicUser("alice", "Alice"))
	rules := []*gl.MergeRequestApprovalRule{approvalRule("Approvers", "alice")}
	result := MapMR(m, nil, approvals("alice"), rules)
	assert.Equal(t, domain.PhaseReadyToMerge, result.Phase, "want PhaseReadyToMerge when all approvers approved")
}

func TestExtractReReviewUsername(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{"requested review from @alice", "alice"},
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
