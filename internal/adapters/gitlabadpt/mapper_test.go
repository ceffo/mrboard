package gitlabadpt

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
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

// TestDeriveReviewerState exercises the pure classifier with pre-filtered event slices.
func TestDeriveReviewerState(t *testing.T) {
	cases := []struct {
		name     string
		approved bool
		events   []DiscussionEvent
		want     domain.ReviewerState
	}{
		{
			name:   "not started - no events",
			events: nil,
			want:   domain.ReviewerNotStarted,
		},
		{
			name:     "approved flag with no events",
			approved: true,
			events:   nil,
			want:     domain.ReviewerApproved,
		},
		{
			name:   "commented",
			events: []DiscussionEvent{{Kind: KindComment, Timestamp: t1}},
			want:   domain.ReviewerCommented,
		},
		{
			name: "re-review after comment → re-review requested",
			events: []DiscussionEvent{
				{Kind: KindComment, Timestamp: t1},
				{Kind: KindReReviewRequest, Timestamp: t2},
			},
			want: domain.ReviewerReReviewRequested,
		},
		{
			name: "comment after re-review → still commented",
			events: []DiscussionEvent{
				{Kind: KindReReviewRequest, Timestamp: t1},
				{Kind: KindComment, Timestamp: t2},
			},
			want: domain.ReviewerCommented,
		},
		{
			name:     "approved overrides comment",
			approved: true,
			events:   []DiscussionEvent{{Kind: KindComment, Timestamp: t1}},
			want:     domain.ReviewerApproved,
		},
		{
			name:     "approved overrides re-review request",
			approved: true,
			events:   []DiscussionEvent{{Kind: KindReReviewRequest, Timestamp: t1}},
			want:     domain.ReviewerApproved,
		},
		{
			// KindApproval events carry the timestamp but state comes from the approved flag.
			name:   "approval event alone does not change state (approved=false)",
			events: []DiscussionEvent{{Kind: KindApproval, Timestamp: t1}},
			want:   domain.ReviewerNotStarted,
		},
		{
			name: "re-review only → re-review requested",
			events: []DiscussionEvent{
				{Kind: KindReReviewRequest, Timestamp: t1},
			},
			want: domain.ReviewerReReviewRequested,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveReviewerState(tc.approved, tc.events)
			if got != tc.want {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestDeriveReviewerStates_NotStarted(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	result := DeriveReviewerStates(m, nil, approvals())

	if len(result) != 1 {
		t.Fatalf("want 1 reviewer (not-started included), got %d", len(result))
	}
	if result[0].State != domain.ReviewerNotStarted {
		t.Errorf("want ReviewerNotStarted, got %v", result[0].State)
	}
}

func TestDeriveReviewerStates_Commented(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(systemNote("requested review from @alice", t1)),
		discussion(userNote("alice", t2)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	if result[0].State != domain.ReviewerCommented {
		t.Errorf("want Commented, got %v", result[0].State)
	}
}

func TestDeriveReviewerStates_ReReviewRequested(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(userNote("alice", t1)),
		discussion(systemNote("requested review from @alice", t2)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	if result[0].State != domain.ReviewerReReviewRequested {
		t.Errorf("want ReReviewRequested, got %v", result[0].State)
	}
}

func TestDeriveReviewerStates_Approved(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(userNote("alice", t1)),
	}

	result := DeriveReviewerStates(m, discussions, approvals("alice"))

	if result[0].State != domain.ReviewerApproved {
		t.Errorf("want Approved, got %v", result[0].State)
	}
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
		t.Fatalf("reviewer %q not found", username)
		return domain.ReviewerNotStarted
	}

	if got := stateFor("alice"); got != domain.ReviewerCommented {
		t.Errorf("alice: want Commented, got %v", got)
	}
	if got := stateFor("bob"); got != domain.ReviewerReReviewRequested {
		t.Errorf("bob: want ReReviewRequested, got %v", got)
	}
}

func TestDeriveReviewerStates_NonReviewerNotesIgnored(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(userNote("not-a-reviewer", t1)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	if len(result) != 1 {
		t.Fatalf("want 1 reviewer (alice not-started, included), got %d", len(result))
	}
	if result[0].State != domain.ReviewerNotStarted {
		t.Errorf("want ReviewerNotStarted, got %v", result[0].State)
	}
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

	if result[0].State != domain.ReviewerNotStarted {
		t.Errorf("want NotStarted (thread resolved), got %v", result[0].State)
	}
}

func TestDeriveReviewerStates_UnresolvedThreadStillCommented(t *testing.T) {
	// Reviewer has one resolved thread and one open thread — still Commented.
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		resolvedDiscussion(userNote("alice", t1)),
		discussion(userNote("alice", t2)), // unresolved
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	if result[0].State != domain.ReviewerCommented {
		t.Errorf("want Commented (unresolved thread remains), got %v", result[0].State)
	}
}

func TestDeriveReviewerStates_NoReviewers(t *testing.T) {
	m := mr()
	result := DeriveReviewerStates(m, nil, approvals())
	if result != nil {
		t.Errorf("want nil for no reviewers, got %v", result)
	}
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
			if got := countRoundTripsFromEvents(events); got != tc.want {
				t.Errorf("countRoundTripsFromEvents: want %d, got %d", tc.want, got)
			}
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
	if result.RoundTripCount != 2 {
		t.Errorf("want RoundTripCount=2, got %d", result.RoundTripCount)
	}
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
		if r.Username == "alice" && !r.IsApprover {
			t.Errorf("alice should be IsApprover=true")
		}
		if r.Username == "bob" && r.IsApprover {
			t.Errorf("bob should be IsApprover=false")
		}
	}
}

func TestMapMR_IsApprover_NoApproversRule(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	result := MapMR(m, nil, approvals(), nil)
	for _, r := range result.Reviewers {
		if r.IsApprover {
			t.Errorf("want IsApprover=false when no Approvers rule, got true for %s", r.Username)
		}
	}
}

func TestMapMR_DetailedMergeStatus_Stored(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	m.DetailedMergeStatus = detailedMergeStatusMergeable
	result := MapMR(m, nil, approvals(), nil)
	if result.DetailedMergeStatus != detailedMergeStatusMergeable {
		t.Errorf("want DetailedMergeStatus=%s stored on domain MR, got %q",
			detailedMergeStatusMergeable, result.DetailedMergeStatus)
	}
}

func TestMapMR_DetailedMergeStatus_Stored_NonMergeable(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	m.DetailedMergeStatus = "ci_must_pass"
	result := MapMR(m, nil, approvals(), nil)
	if result.DetailedMergeStatus != "ci_must_pass" {
		t.Errorf("want DetailedMergeStatus=ci_must_pass stored, got %q", result.DetailedMergeStatus)
	}
}

func TestMapMR_PhaseReadyToMerge_WhenAllApproversApproved(t *testing.T) {
	// alice is in the Approvers rule and has approved
	m := mr(basicUser("alice", "Alice"))
	rules := []*gl.MergeRequestApprovalRule{approvalRule("Approvers", "alice")}
	result := MapMR(m, nil, approvals("alice"), rules)
	if result.Phase != domain.PhaseReadyToMerge {
		t.Errorf("want PhaseReadyToMerge when all approvers approved, got %v", result.Phase)
	}
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
		if tc.want == "" && got != "" {
			t.Errorf("body=%q: want empty, got %q", tc.body, got)
		} else if tc.want != "" && got != tc.want {
			t.Errorf("body=%q: want %q, got %q", tc.body, tc.want, got)
		}
	}
}
