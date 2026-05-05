// Package domain defines the core types and business logic for mrboard.
package domain

import (
	"fmt"
	"time"
)

const (
	minutesPerHour = 60
	hoursPerDay    = 24
)

// ReviewerState tracks where a reviewer is in the review lifecycle.
type ReviewerState int

// Reviewer lifecycle states, in rough progression.
const (
	ReviewerNotStarted        ReviewerState = iota // assigned, no activity
	ReviewerCommented                              // left comments; ball is in author's court
	ReviewerReReviewRequested                      // author re-requested; ball in reviewer's court
	ReviewerApproved                               // approved (terminal unless revoked)
)

// MRPhase classifies the overall state of a merge request.
type MRPhase int

// MR phase values, in priority order per domain-model.md.
const (
	PhaseDraft             MRPhase = iota // MR is still a draft
	PhaseNeedsReview                      // ball is in reviewer(s)' court
	PhaseNeedsAuthorAction                // ball is in author's court
	PhaseReadyToMerge                     // all threads resolved + enough approvals
)

// ReviewerInfo holds the current state for a single reviewer on an MR.
type ReviewerInfo struct {
	Username     string
	Name         string
	State        ReviewerState
	WaitingSince time.Time
}

// MergeRequest is the core domain type representing a GitLab merge request.
type MergeRequest struct {
	ID        int
	IID       int
	ProjectID int
	Title     string
	Author    string
	WebURL    string

	Phase     MRPhase
	Reviewers []ReviewerInfo

	CreatedAt     time.Time
	NonDraftSince time.Time
	WaitingSince  time.Time

	ApprovalCount     int
	RequiredApprovals int
	OpenThreads       int
	RoundTripCount    int
}

// ClassifyPhase determines the MRPhase from the MR's fields.
// Evaluated in priority order per docs/domain-model.md.
func ClassifyPhase(draft bool, openThreads, approvalCount, requiredApprovals int, reviewers []ReviewerInfo) MRPhase {
	if draft {
		return PhaseDraft
	}
	if openThreads == 0 && approvalCount >= requiredApprovals {
		return PhaseReadyToMerge
	}
	for _, r := range reviewers {
		if r.State == ReviewerCommented {
			return PhaseNeedsAuthorAction
		}
	}
	return PhaseNeedsReview
}

// FormatDuration formats a duration as a human-readable string.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return "< 1m"
	}
	totalMinutes := int(d.Minutes())
	days := totalMinutes / (minutesPerHour * hoursPerDay)
	hours := (totalMinutes % (minutesPerHour * hoursPerDay)) / minutesPerHour
	minutes := totalMinutes % minutesPerHour

	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}
