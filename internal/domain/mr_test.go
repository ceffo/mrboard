package domain

import (
	"testing"
	"time"
)

func TestClassifyPhase_Draft(t *testing.T) {
	phase := ClassifyPhase(true, 0, 2, 2, nil)
	if phase != PhaseDraft {
		t.Fatalf("expected PhaseDraft, got %d", phase)
	}
}

func TestClassifyPhase_ReadyToMerge(t *testing.T) {
	phase := ClassifyPhase(false, 0, 2, 2, []ReviewerInfo{
		{State: ReviewerApproved},
	})
	if phase != PhaseReadyToMerge {
		t.Fatalf("expected PhaseReadyToMerge, got %d", phase)
	}
}

func TestClassifyPhase_ReadyToMerge_NoReviewers(t *testing.T) {
	phase := ClassifyPhase(false, 0, 0, 0, nil)
	if phase != PhaseReadyToMerge {
		t.Fatalf("expected PhaseReadyToMerge when 0 required approvals and 0 threads, got %d", phase)
	}
}

func TestClassifyPhase_NeedsAuthorAction(t *testing.T) {
	reviewers := []ReviewerInfo{
		{State: ReviewerCommented},
		{State: ReviewerReReviewRequested},
	}
	phase := ClassifyPhase(false, 1, 0, 2, reviewers)
	if phase != PhaseNeedsAuthorAction {
		t.Fatalf("expected PhaseNeedsAuthorAction, got %d", phase)
	}
}

func TestClassifyPhase_NeedsAuthorAction_TakesPrecedenceOverReReview(t *testing.T) {
	reviewers := []ReviewerInfo{
		{State: ReviewerReReviewRequested},
		{State: ReviewerCommented},
	}
	phase := ClassifyPhase(false, 0, 1, 2, reviewers)
	if phase != PhaseNeedsAuthorAction {
		t.Fatalf("expected PhaseNeedsAuthorAction when mixed states, got %d", phase)
	}
}

func TestClassifyPhase_NeedsReview_NoReviewers(t *testing.T) {
	phase := ClassifyPhase(false, 1, 0, 2, nil)
	if phase != PhaseNeedsReview {
		t.Fatalf("expected PhaseNeedsReview with no reviewers, got %d", phase)
	}
}

func TestClassifyPhase_NeedsReview_AllNotStarted(t *testing.T) {
	reviewers := []ReviewerInfo{
		{State: ReviewerNotStarted},
		{State: ReviewerNotStarted},
	}
	phase := ClassifyPhase(false, 0, 0, 2, reviewers)
	if phase != PhaseNeedsReview {
		t.Fatalf("expected PhaseNeedsReview when all NotStarted, got %d", phase)
	}
}

func TestClassifyPhase_DraftTakesPrecedence(t *testing.T) {
	// Draft wins even with enough approvals and no threads
	phase := ClassifyPhase(true, 0, 2, 2, nil)
	if phase != PhaseDraft {
		t.Fatalf("expected PhaseDraft to take precedence, got %d", phase)
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
