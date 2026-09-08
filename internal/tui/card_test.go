package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ceffo/mrboard/internal/domain"
)

func TestReviewerIcon(t *testing.T) {
	cases := []struct {
		name       string
		state      domain.ReviewerState
		isApprover bool
		want       string
	}{
		{"not started, approver", domain.ReviewerNotStarted, true, "⏳"},
		{"not started, basic reviewer", domain.ReviewerNotStarted, false, ""},
		{"commented, approver", domain.ReviewerCommented, true, "💬"},
		{"commented, basic reviewer", domain.ReviewerCommented, false, "💬"},
		{"re-review requested, approver", domain.ReviewerReReviewRequested, true, "🔄"},
		{"re-review requested, basic reviewer", domain.ReviewerReReviewRequested, false, "🔄"},
		{"approved, approver", domain.ReviewerApproved, true, "✓"},
		{"approved, basic reviewer", domain.ReviewerApproved, false, "✓"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, reviewerIcon(tc.state, tc.isApprover))
		})
	}
}

func TestShowWaitingDuration(t *testing.T) {
	waitingSince := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		r    domain.ReviewerInfo
		want bool
	}{
		{
			name: "zero WaitingSince never shows",
			r:    domain.ReviewerInfo{State: domain.ReviewerNotStarted, IsApprover: true},
			want: false,
		},
		{
			name: "not started, approver shows",
			r:    domain.ReviewerInfo{State: domain.ReviewerNotStarted, IsApprover: true, WaitingSince: waitingSince},
			want: true,
		},
		{
			name: "not started, basic reviewer hides",
			r:    domain.ReviewerInfo{State: domain.ReviewerNotStarted, IsApprover: false, WaitingSince: waitingSince},
			want: false,
		},
		{
			name: "re-review requested, approver shows",
			r: domain.ReviewerInfo{
				State: domain.ReviewerReReviewRequested, IsApprover: true, WaitingSince: waitingSince,
			},
			want: true,
		},
		{
			name: "re-review requested, basic reviewer hides",
			r: domain.ReviewerInfo{
				State: domain.ReviewerReReviewRequested, IsApprover: false, WaitingSince: waitingSince,
			},
			want: false,
		},
		{
			name: "commented, basic reviewer still shows",
			r:    domain.ReviewerInfo{State: domain.ReviewerCommented, IsApprover: false, WaitingSince: waitingSince},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, showWaitingDuration(tc.r))
		})
	}
}
