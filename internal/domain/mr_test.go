package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClassifyPhase_Draft(t *testing.T) {
	phase := ClassifyPhase(true, false, nil)
	assert.Equal(t, PhaseDraft, phase)
}

func TestClassifyPhase_ReadyToMerge_AllApproversApproved(t *testing.T) {
	phase := ClassifyPhase(false, false, []ReviewerInfo{
		{IsApprover: true, State: ReviewerApproved},
		{IsApprover: true, State: ReviewerApproved},
	})
	assert.Equal(t, PhaseReadyToMerge, phase, "expected PhaseReadyToMerge when all approvers approved")
}

func TestClassifyPhase_NotReadyToMerge_OnlyPartialApprovals(t *testing.T) {
	phase := ClassifyPhase(false, false, []ReviewerInfo{
		{IsApprover: true, State: ReviewerApproved},
		{IsApprover: true, State: ReviewerNotStarted},
	})
	assert.Equal(t, PhaseNeedsReview, phase, "expected PhaseNeedsReview when not all approvers approved")
}

func TestClassifyPhase_NotReadyToMerge_NoApprovers(t *testing.T) {
	phase := ClassifyPhase(false, false, []ReviewerInfo{
		{IsApprover: false, State: ReviewerApproved},
	})
	assert.Equal(t, PhaseNeedsReview, phase, "expected PhaseNeedsReview when no designated approvers")
}

func TestClassifyPhase_NotReadyToMerge_EmptyReviewers(t *testing.T) {
	phase := ClassifyPhase(false, true, nil)
	assert.Equal(t, PhaseNeedsReview, phase, "expected PhaseNeedsReview with no reviewers (no approvers)")
}

func TestClassifyPhase_MergeableIgnored_StillNeedsReview(t *testing.T) {
	// mergeable=true no longer drives phase; without approvers it stays NeedsReview
	phase := ClassifyPhase(false, true, nil)
	assert.Equal(t, PhaseNeedsReview, phase, "expected PhaseNeedsReview when mergeable=true but no approvers")
}

func TestClassifyPhase_NeedsAuthorAction(t *testing.T) {
	reviewers := []ReviewerInfo{
		{State: ReviewerCommented},
		{State: ReviewerReReviewRequested},
	}
	phase := ClassifyPhase(false, false, reviewers)
	assert.Equal(t, PhaseNeedsAuthorAction, phase)
}

func TestClassifyPhase_NeedsAuthorAction_TakesPrecedenceOverReReview(t *testing.T) {
	reviewers := []ReviewerInfo{
		{State: ReviewerReReviewRequested},
		{State: ReviewerCommented},
	}
	phase := ClassifyPhase(false, false, reviewers)
	assert.Equal(t, PhaseNeedsAuthorAction, phase, "expected PhaseNeedsAuthorAction when mixed states")
}

func TestClassifyPhase_NeedsReview_NoReviewers(t *testing.T) {
	phase := ClassifyPhase(false, false, nil)
	assert.Equal(t, PhaseNeedsReview, phase, "expected PhaseNeedsReview with no reviewers")
}

func TestClassifyPhase_NeedsReview_AllNotStarted(t *testing.T) {
	reviewers := []ReviewerInfo{
		{State: ReviewerNotStarted},
		{State: ReviewerNotStarted},
	}
	phase := ClassifyPhase(false, false, reviewers)
	assert.Equal(t, PhaseNeedsReview, phase, "expected PhaseNeedsReview when all NotStarted")
}

func TestClassifyPhase_DraftTakesPrecedence(t *testing.T) {
	phase := ClassifyPhase(true, true, nil)
	assert.Equal(t, PhaseDraft, phase, "expected PhaseDraft to take precedence over mergeable=true")
}

func TestDeriveWaitingSince_NeedsAuthorAction_LatestComment(t *testing.T) {
	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Hour)
	reviewers := []ReviewerInfo{
		{State: ReviewerCommented, WaitingSince: earlier},
		{State: ReviewerCommented, WaitingSince: later},
		{State: ReviewerReReviewRequested, WaitingSince: later.Add(time.Hour)},
	}
	since := DeriveWaitingSince(PhaseNeedsAuthorAction, reviewers, earlier.Add(-time.Hour))
	assert.True(t, since.Equal(later), "expected latest commented WaitingSince %v, got %v", later, since)
}

func TestDeriveWaitingSince_NeedsReview_FallsBackToCreatedAt(t *testing.T) {
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	since := DeriveWaitingSince(PhaseNeedsReview, nil, createdAt)
	assert.True(t, since.Equal(createdAt), "expected createdAt %v with no re-review requests, got %v", createdAt, since)
}

func TestDeriveWaitingSince_NeedsReview_LatestReReviewRequest(t *testing.T) {
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	latest := createdAt.Add(2 * time.Hour)
	reviewers := []ReviewerInfo{
		{State: ReviewerReReviewRequested, WaitingSince: createdAt.Add(time.Hour)},
		{State: ReviewerReReviewRequested, WaitingSince: latest},
	}
	since := DeriveWaitingSince(PhaseNeedsReview, reviewers, createdAt)
	assert.True(t, since.Equal(latest), "expected latest re-review WaitingSince %v, got %v", latest, since)
}

func TestDeriveWaitingSince_OtherPhases_ReturnsZero(t *testing.T) {
	since := DeriveWaitingSince(PhaseDraft, nil, time.Now())
	assert.True(t, since.IsZero(), "expected zero WaitingSince for PhaseDraft, got %v", since)

	since = DeriveWaitingSince(PhaseReadyToMerge, nil, time.Now())
	assert.True(t, since.IsZero(), "expected zero WaitingSince for PhaseReadyToMerge, got %v", since)
}

func TestFormatDuration_LessThanMinute(t *testing.T) {
	got := FormatDuration(30 * time.Second)
	assert.Equal(t, "< 1m", got)
}

func TestFormatDuration_Minutes(t *testing.T) {
	got := FormatDuration(45 * time.Minute)
	assert.Equal(t, "45m", got)
}

func TestFormatDuration_HoursAndMinutes(t *testing.T) {
	d := 3*time.Hour + 20*time.Minute
	got := FormatDuration(d)
	assert.Equal(t, "3h 20m", got)
}

func TestFormatDuration_HoursOnly(t *testing.T) {
	got := FormatDuration(3 * time.Hour)
	assert.Equal(t, "3h", got)
}

func TestFormatDuration_DaysAndHours(t *testing.T) {
	d := 2*24*time.Hour + 4*time.Hour
	got := FormatDuration(d)
	assert.Equal(t, "2d 4h", got)
}

func TestFormatDuration_DaysOnly(t *testing.T) {
	got := FormatDuration(3 * 24 * time.Hour)
	assert.Equal(t, "3d", got)
}

func TestFormatDuration_MonthsAndDays(t *testing.T) {
	d := 45 * 24 * time.Hour // 1mo 15d
	got := FormatDuration(d)
	assert.Equal(t, "1mo 15d", got)
}

func TestFormatDuration_MonthsOnly(t *testing.T) {
	d := 90 * 24 * time.Hour // 3mo exactly
	got := FormatDuration(d)
	assert.Equal(t, "3mo", got)
}

func TestFormatDuration_YearsAndMonths(t *testing.T) {
	d := 400 * 24 * time.Hour // 1y 1mo (360+30 = 390 days threshold; 400/360=1y, rem=40d, 40/30=1mo)
	got := FormatDuration(d)
	assert.Equal(t, "1y 1mo", got)
}

func TestFormatDuration_YearsOnly(t *testing.T) {
	d := 720 * 24 * time.Hour // 720/360=2y exactly
	got := FormatDuration(d)
	assert.Equal(t, "2y", got)
}
