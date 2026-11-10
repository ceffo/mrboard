package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func reviewedBy(phase MRPhase, reviewers ...ReviewerInfo) MergeRequest {
	return MergeRequest{Author: attnOther, Phase: phase, Reviewers: reviewers}
}

func TestNeedsReviewBy(t *testing.T) {
	merged := reviewedBy(PhaseNeedsReview,
		ReviewerInfo{Username: attnMe, State: ReviewerReReviewRequested, IsApprover: true})
	merged.MergedAt = time.Date(2026, 5, 2, 8, 0, 0, 0, time.UTC)
	approverOther := ReviewerInfo{Username: attnApprover, State: ReviewerNotStarted, IsApprover: true}
	tests := []struct {
		name string
		mr   MergeRequest
		user string
		want bool
	}{
		{"plain reviewer not started", reviewedBy(PhaseNeedsReview,
			ReviewerInfo{Username: attnMe, State: ReviewerNotStarted}), attnMe, false},
		{"plain reviewer commented", reviewedBy(PhaseNeedsAuthorAction,
			ReviewerInfo{Username: attnMe, State: ReviewerCommented}), attnMe, true},
		{"plain reviewer re-review requested", reviewedBy(PhaseNeedsReview,
			ReviewerInfo{Username: attnMe, State: ReviewerReReviewRequested}), attnMe, true},
		{"plain reviewer approved", reviewedBy(PhaseNeedsReview,
			ReviewerInfo{Username: attnMe, State: ReviewerApproved}), attnMe, false},
		{"approver not started", reviewedBy(PhaseNeedsReview,
			ReviewerInfo{Username: attnMe, State: ReviewerNotStarted, IsApprover: true}), attnMe, true},
		{"approver commented", reviewedBy(PhaseNeedsAuthorAction,
			ReviewerInfo{Username: attnMe, State: ReviewerCommented, IsApprover: true}), attnMe, true},
		{"approver re-review requested", reviewedBy(PhaseNeedsReview,
			ReviewerInfo{Username: attnMe, State: ReviewerReReviewRequested, IsApprover: true}), attnMe, true},
		{"approver already approved", reviewedBy(PhaseReadyToMerge,
			ReviewerInfo{Username: attnMe, State: ReviewerApproved, IsApprover: true}), attnMe, true},
		{"draft with approver", reviewedBy(PhaseDraft,
			ReviewerInfo{Username: attnMe, State: ReviewerNotStarted, IsApprover: true}), attnMe, false},
		{"draft with re-review requested", reviewedBy(PhaseDraft,
			ReviewerInfo{Username: attnMe, State: ReviewerReReviewRequested}), attnMe, false},
		{"non-approver not started on approver MR", reviewedBy(PhaseNeedsReview, approverOther,
			ReviewerInfo{Username: attnMe, State: ReviewerNotStarted}), attnMe, false},
		{"non-approver commented on approver MR", reviewedBy(PhaseNeedsAuthorAction, approverOther,
			ReviewerInfo{Username: attnMe, State: ReviewerCommented}), attnMe, true},
		{"approval-rule approver who is not a reviewer", MergeRequest{
			Author: attnOther, Phase: PhaseNeedsReview, Approvers: []string{attnMe},
			Reviewers: []ReviewerInfo{approverOther},
		}, attnMe, false},
		{"author only", MergeRequest{
			Author: attnMe, Phase: PhaseNeedsAuthorAction,
			Reviewers: []ReviewerInfo{{Username: attnOther, State: ReviewerCommented}},
		}, attnMe, false},
		{"merged with approver re-review requested", merged, attnMe, false},
		{"no current user, approver reviewer", reviewedBy(PhaseNeedsReview,
			ReviewerInfo{Username: attnMe, State: ReviewerCommented, IsApprover: true}), "", false},
		{"no current user, nameless entry", reviewedBy(PhaseNeedsReview,
			ReviewerInfo{Username: "", State: ReviewerCommented, IsApprover: true}), "", false},
		{"unrelated user", reviewedBy(PhaseNeedsReview, approverOther), attnMe, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.mr.NeedsReviewBy(tt.user))
		})
	}
}
