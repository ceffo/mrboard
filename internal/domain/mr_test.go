package domain

import (
	"testing"
	"time"
)

func TestClassifyPhase_Draft(t *testing.T) {
	phase := ClassifyPhase(true, false, nil)
	if phase != PhaseDraft {
		t.Fatalf("expected PhaseDraft, got %d", phase)
	}
}

func TestClassifyPhase_ReadyToMerge_AllApproversApproved(t *testing.T) {
	phase := ClassifyPhase(false, false, []ReviewerInfo{
		{IsApprover: true, State: ReviewerApproved},
		{IsApprover: true, State: ReviewerApproved},
	})
	if phase != PhaseReadyToMerge {
		t.Fatalf("expected PhaseReadyToMerge when all approvers approved, got %d", phase)
	}
}

func TestClassifyPhase_NotReadyToMerge_OnlyPartialApprovals(t *testing.T) {
	phase := ClassifyPhase(false, false, []ReviewerInfo{
		{IsApprover: true, State: ReviewerApproved},
		{IsApprover: true, State: ReviewerNotStarted},
	})
	if phase != PhaseNeedsReview {
		t.Fatalf("expected PhaseNeedsReview when not all approvers approved, got %d", phase)
	}
}

func TestClassifyPhase_NotReadyToMerge_NoApprovers(t *testing.T) {
	phase := ClassifyPhase(false, false, []ReviewerInfo{
		{IsApprover: false, State: ReviewerApproved},
	})
	if phase != PhaseNeedsReview {
		t.Fatalf("expected PhaseNeedsReview when no designated approvers, got %d", phase)
	}
}

func TestClassifyPhase_NotReadyToMerge_EmptyReviewers(t *testing.T) {
	phase := ClassifyPhase(false, true, nil)
	if phase != PhaseNeedsReview {
		t.Fatalf("expected PhaseNeedsReview with no reviewers (no approvers), got %d", phase)
	}
}

func TestClassifyPhase_MergeableIgnored_StillNeedsReview(t *testing.T) {
	// mergeable=true no longer drives phase; without approvers it stays NeedsReview
	phase := ClassifyPhase(false, true, nil)
	if phase != PhaseNeedsReview {
		t.Fatalf("expected PhaseNeedsReview when mergeable=true but no approvers, got %d", phase)
	}
}

func TestClassifyPhase_NeedsAuthorAction(t *testing.T) {
	reviewers := []ReviewerInfo{
		{State: ReviewerCommented},
		{State: ReviewerReReviewRequested},
	}
	phase := ClassifyPhase(false, false, reviewers)
	if phase != PhaseNeedsAuthorAction {
		t.Fatalf("expected PhaseNeedsAuthorAction, got %d", phase)
	}
}

func TestClassifyPhase_NeedsAuthorAction_TakesPrecedenceOverReReview(t *testing.T) {
	reviewers := []ReviewerInfo{
		{State: ReviewerReReviewRequested},
		{State: ReviewerCommented},
	}
	phase := ClassifyPhase(false, false, reviewers)
	if phase != PhaseNeedsAuthorAction {
		t.Fatalf("expected PhaseNeedsAuthorAction when mixed states, got %d", phase)
	}
}

func TestClassifyPhase_NeedsReview_NoReviewers(t *testing.T) {
	phase := ClassifyPhase(false, false, nil)
	if phase != PhaseNeedsReview {
		t.Fatalf("expected PhaseNeedsReview with no reviewers, got %d", phase)
	}
}

func TestClassifyPhase_NeedsReview_AllNotStarted(t *testing.T) {
	reviewers := []ReviewerInfo{
		{State: ReviewerNotStarted},
		{State: ReviewerNotStarted},
	}
	phase := ClassifyPhase(false, false, reviewers)
	if phase != PhaseNeedsReview {
		t.Fatalf("expected PhaseNeedsReview when all NotStarted, got %d", phase)
	}
}

func TestClassifyPhase_DraftTakesPrecedence(t *testing.T) {
	phase := ClassifyPhase(true, true, nil)
	if phase != PhaseDraft {
		t.Fatalf("expected PhaseDraft to take precedence over mergeable=true, got %d", phase)
	}
}

func TestFormatDuration_LessThanMinute(t *testing.T) {
	if got := FormatDuration(30 * time.Second); got != "< 1m" {
		t.Fatalf("expected '< 1m', got %q", got)
	}
}

func TestFormatDuration_Minutes(t *testing.T) {
	if got := FormatDuration(45 * time.Minute); got != "45m" {
		t.Fatalf("expected '45m', got %q", got)
	}
}

func TestFormatDuration_HoursAndMinutes(t *testing.T) {
	d := 3*time.Hour + 20*time.Minute
	if got := FormatDuration(d); got != "3h 20m" {
		t.Fatalf("expected '3h 20m', got %q", got)
	}
}

func TestFormatDuration_HoursOnly(t *testing.T) {
	if got := FormatDuration(3 * time.Hour); got != "3h" {
		t.Fatalf("expected '3h', got %q", got)
	}
}

func TestFormatDuration_DaysAndHours(t *testing.T) {
	d := 2*24*time.Hour + 4*time.Hour
	if got := FormatDuration(d); got != "2d 4h" {
		t.Fatalf("expected '2d 4h', got %q", got)
	}
}

func TestFormatDuration_DaysOnly(t *testing.T) {
	if got := FormatDuration(3 * 24 * time.Hour); got != "3d" {
		t.Fatalf("expected '3d', got %q", got)
	}
}

func TestFormatDuration_MonthsAndDays(t *testing.T) {
	d := 45 * 24 * time.Hour // 1mo 15d
	if got := FormatDuration(d); got != "1mo 15d" {
		t.Fatalf("expected '1mo 15d', got %q", got)
	}
}

func TestFormatDuration_MonthsOnly(t *testing.T) {
	d := 90 * 24 * time.Hour // 3mo exactly
	if got := FormatDuration(d); got != "3mo" {
		t.Fatalf("expected '3mo', got %q", got)
	}
}

func TestFormatDuration_YearsAndMonths(t *testing.T) {
	d := 400 * 24 * time.Hour // 1y 1mo (360+30 = 390 days threshold; 400/360=1y, rem=40d, 40/30=1mo)
	if got := FormatDuration(d); got != "1y 1mo" {
		t.Fatalf("expected '1y 1mo', got %q", got)
	}
}

func TestFormatDuration_YearsOnly(t *testing.T) {
	d := 720 * 24 * time.Hour // 720/360=2y exactly
	if got := FormatDuration(d); got != "2y" {
		t.Fatalf("expected '2y', got %q", got)
	}
}
