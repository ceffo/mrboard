package gitlab

import (
	"testing"
	"time"

	"github.com/mrboard/mrboard/internal/domain"
	gl "github.com/xanzy/go-gitlab"
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
		Author: struct {
			ID        int    `json:"id"`
			Username  string `json:"username"`
			Email     string `json:"email"`
			Name      string `json:"name"`
			State     string `json:"state"`
			AvatarURL string `json:"avatar_url"`
			WebURL    string `json:"web_url"`
		}{},
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
		ApprovalsRequired: len(usernames),
		ApprovalsLeft:     0,
	}
}

func mr(reviewers ...*gl.BasicUser) *gl.MergeRequest {
	return &gl.MergeRequest{
		ID:        1,
		IID:       1,
		ProjectID: 10,
		Title:     "Test MR",
		Reviewers: reviewers,
		CreatedAt: ptr(t0),
		Author:    basicUser("author", "Author"),
	}
}

// TestDeriveReviewerStates_NotStarted verifies a reviewer who has never commented
// and was never re-requested gets ReviewerNotStarted.
func TestDeriveReviewerStates_NotStarted(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	result := DeriveReviewerStates(m, nil, approvals())

	if len(result) != 1 {
		t.Fatalf("want 1 reviewer, got %d", len(result))
	}
	if result[0].State != domain.ReviewerNotStarted {
		t.Errorf("want NotStarted, got %v", result[0].State)
	}
}

// TestDeriveReviewerStates_Commented verifies that a reviewer whose last comment
// is more recent than the last re-review request gets ReviewerCommented.
func TestDeriveReviewerStates_Commented(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(systemNote("requested review from @alice", t1)),
		discussion(userNote("alice", t2)), // alice comments after re-request
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	if result[0].State != domain.ReviewerCommented {
		t.Errorf("want Commented, got %v", result[0].State)
	}
}

// TestDeriveReviewerStates_ReReviewRequested verifies that a re-review request
// more recent than the reviewer's last comment produces ReviewerReReviewRequested.
func TestDeriveReviewerStates_ReReviewRequested(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(userNote("alice", t1)),                          // alice comments first
		discussion(systemNote("requested review from @alice", t2)), // then re-requested
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	if result[0].State != domain.ReviewerReReviewRequested {
		t.Errorf("want ReReviewRequested, got %v", result[0].State)
	}
}

// TestDeriveReviewerStates_Approved verifies that a reviewer in ApprovedBy gets
// ReviewerApproved regardless of discussion state.
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

// TestDeriveReviewerStates_MultipleReviewers verifies independent state derivation
// for multiple reviewers in the same MR.
func TestDeriveReviewerStates_MultipleReviewers(t *testing.T) {
	m := mr(basicUser("alice", "Alice"), basicUser("bob", "Bob"))
	discussions := []*gl.Discussion{
		// alice: commented after re-request → Commented
		discussion(systemNote("requested review from @alice", t1)),
		discussion(userNote("alice", t2)),
		// bob: re-requested after comment → ReReviewRequested
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

// TestDeriveReviewerStates_NonReviewerNotesIgnored verifies that comments from
// non-reviewers do not affect reviewer state.
func TestDeriveReviewerStates_NonReviewerNotesIgnored(t *testing.T) {
	m := mr(basicUser("alice", "Alice"))
	discussions := []*gl.Discussion{
		discussion(userNote("not-a-reviewer", t1)),
	}

	result := DeriveReviewerStates(m, discussions, approvals())

	if result[0].State != domain.ReviewerNotStarted {
		t.Errorf("want NotStarted, got %v", result[0].State)
	}
}

// TestDeriveReviewerStates_NoReviewers verifies nil return when there are no reviewers.
func TestDeriveReviewerStates_NoReviewers(t *testing.T) {
	m := mr()
	result := DeriveReviewerStates(m, nil, approvals())
	if result != nil {
		t.Errorf("want nil for no reviewers, got %v", result)
	}
}

// TestExtractReReviewUsername covers the system note parsing helper.
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
