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

func TestDeriveReviewerStates_NoReviewers(t *testing.T) {
	m := mr()
	result := DeriveReviewerStates(m, nil, approvals())
	if result != nil {
		t.Errorf("want nil for no reviewers, got %v", result)
	}
}

func TestCountRoundTrips(t *testing.T) {
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
			if got := countRoundTrips(tc.discussions); got != tc.want {
				t.Errorf("countRoundTrips: want %d, got %d", tc.want, got)
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
	result := MapMR(m, discussions, approvals(), 1)
	if result.RoundTripCount != 2 {
		t.Errorf("want RoundTripCount=2, got %d", result.RoundTripCount)
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
