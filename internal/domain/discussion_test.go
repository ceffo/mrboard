package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testUsername = "alice"
	testOther    = "bob"
)

var (
	evT0 = time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	evT1 = evT0.Add(1 * time.Hour)
	evT2 = evT0.Add(2 * time.Hour)
	evT3 = evT0.Add(3 * time.Hour)
)

func singleReviewer() []ReviewerInfo {
	return []ReviewerInfo{{Username: testUsername, Name: testUsername}}
}

// TestDeriveReviewerInfos_SingleReviewer exercises the state machine through the
// public DeriveReviewerInfos API using a single reviewer and hand-crafted event slices.
func TestDeriveReviewerInfos_SingleReviewer(t *testing.T) {
	cases := []struct {
		name       string
		approvedBy map[string]bool
		events     []DiscussionEvent
		wantState  ReviewerState
	}{
		{
			name:      "not started — no events",
			events:    nil,
			wantState: ReviewerNotStarted,
		},
		{
			name:       "approved flag with no events",
			approvedBy: map[string]bool{testUsername: true},
			events:     nil,
			wantState:  ReviewerApproved,
		},
		{
			name:      "commented",
			events:    []DiscussionEvent{{AuthorUsername: testUsername, Kind: KindComment, Timestamp: evT1}},
			wantState: ReviewerCommented,
		},
		{
			name: "re-review after comment → re-review requested",
			events: []DiscussionEvent{
				{AuthorUsername: testUsername, Kind: KindComment, Timestamp: evT1},
				{AuthorUsername: testUsername, Kind: KindReReviewRequest, Timestamp: evT2},
			},
			wantState: ReviewerReReviewRequested,
		},
		{
			name: "comment after re-review → still commented",
			events: []DiscussionEvent{
				{AuthorUsername: testUsername, Kind: KindReReviewRequest, Timestamp: evT1},
				{AuthorUsername: testUsername, Kind: KindComment, Timestamp: evT2},
			},
			wantState: ReviewerCommented,
		},
		{
			name:       "approved overrides comment",
			approvedBy: map[string]bool{testUsername: true},
			events:     []DiscussionEvent{{AuthorUsername: testUsername, Kind: KindComment, Timestamp: evT1}},
			wantState:  ReviewerApproved,
		},
		{
			name:       "approved overrides re-review request",
			approvedBy: map[string]bool{testUsername: true},
			events:     []DiscussionEvent{{AuthorUsername: testUsername, Kind: KindReReviewRequest, Timestamp: evT1}},
			wantState:  ReviewerApproved,
		},
		{
			// KindApproval events carry the timestamp but state comes from approvedBy, not events.
			name:      "approval event alone does not change state (not in approvedBy)",
			events:    []DiscussionEvent{{AuthorUsername: testUsername, Kind: KindApproval, Timestamp: evT1}},
			wantState: ReviewerNotStarted,
		},
		{
			name: "re-review only → re-review requested",
			events: []DiscussionEvent{
				{AuthorUsername: testUsername, Kind: KindReReviewRequest, Timestamp: evT1},
			},
			wantState: ReviewerReReviewRequested,
		},
		{
			name:      "events from non-reviewer are ignored",
			events:    []DiscussionEvent{{AuthorUsername: testOther, Kind: KindComment, Timestamp: evT1}},
			wantState: ReviewerNotStarted,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			approvedBy := tc.approvedBy
			if approvedBy == nil {
				approvedBy = map[string]bool{}
			}
			result := DeriveReviewerInfos(singleReviewer(), tc.events, approvedBy, evT0)
			require.Len(t, result, 1, "want 1 reviewer")
			assert.Equal(t, tc.wantState, result[0].State, "want state")
		})
	}
}

func TestDeriveReviewerInfos_MultipleReviewers(t *testing.T) {
	reviewers := []ReviewerInfo{
		{Username: testUsername, Name: "Alice"},
		{Username: testOther, Name: "Bob"},
	}
	events := []DiscussionEvent{
		{AuthorUsername: testUsername, Kind: KindReReviewRequest, Timestamp: evT1},
		{AuthorUsername: testUsername, Kind: KindComment, Timestamp: evT2},
		{AuthorUsername: testOther, Kind: KindComment, Timestamp: evT1},
		{AuthorUsername: testOther, Kind: KindReReviewRequest, Timestamp: evT3},
	}
	result := DeriveReviewerInfos(reviewers, events, nil, evT0)

	stateFor := func(username string) ReviewerState {
		for _, r := range result {
			if r.Username == username {
				return r.State
			}
		}
		require.Failf(t, "reviewer not found", "reviewer %q not found", username)
		return ReviewerNotStarted
	}

	assert.Equal(t, ReviewerCommented, stateFor(testUsername), "alice")
	assert.Equal(t, ReviewerReReviewRequested, stateFor(testOther), "bob")
}

func TestDeriveReviewerInfos_WaitingSince_ReReviewRequested(t *testing.T) {
	events := []DiscussionEvent{
		{AuthorUsername: testUsername, Kind: KindReReviewRequest, Timestamp: evT2},
	}
	result := DeriveReviewerInfos(singleReviewer(), events, nil, evT0)
	assert.Equal(t, evT2, result[0].WaitingSince, "WaitingSince should be re-review timestamp")
}

func TestDeriveReviewerInfos_WaitingSince_Commented(t *testing.T) {
	events := []DiscussionEvent{
		{AuthorUsername: testUsername, Kind: KindComment, Timestamp: evT1},
	}
	result := DeriveReviewerInfos(singleReviewer(), events, nil, evT0)
	assert.Equal(t, evT1, result[0].WaitingSince, "WaitingSince should be comment timestamp")
}

func TestDeriveReviewerInfos_ApprovedAt_Set(t *testing.T) {
	events := []DiscussionEvent{
		{AuthorUsername: testUsername, Kind: KindApproval, Timestamp: evT2},
	}
	result := DeriveReviewerInfos(singleReviewer(), events, map[string]bool{testUsername: true}, evT0)
	assert.Equal(t, evT2, result[0].ApprovedAt, "ApprovedAt should be approval timestamp")
}

func TestDeriveReviewerInfos_Empty(t *testing.T) {
	result := DeriveReviewerInfos(nil, nil, nil, evT0)
	assert.Empty(t, result, "want empty result for no reviewers")
}

func TestCountRoundTrips(t *testing.T) {
	cases := []struct {
		name   string
		events []DiscussionEvent
		want   int
	}{
		{"nil events", nil, 0},
		{
			"no re-review events",
			[]DiscussionEvent{
				{Kind: KindComment},
				{Kind: KindApproval},
			},
			0,
		},
		{
			"single re-review",
			[]DiscussionEvent{{Kind: KindReReviewRequest}},
			1,
		},
		{
			"multiple re-reviews",
			[]DiscussionEvent{
				{Kind: KindReReviewRequest},
				{Kind: KindComment},
				{Kind: KindReReviewRequest},
				{Kind: KindReReviewRequest},
			},
			3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, CountRoundTrips(tc.events))
		})
	}
}
