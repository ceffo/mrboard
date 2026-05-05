package gitlab

import (
	"strings"
	"time"

	"github.com/mrboard/mrboard/internal/domain"
	gl "github.com/xanzy/go-gitlab"
)

// reReviewPrefix is the system note body prefix GitLab emits when an author
// re-requests review from a specific reviewer.
const reReviewPrefix = "requested review from @"

// DeriveReviewerStates processes GitLab discussions chronologically and returns
// a ReviewerInfo slice for the active reviewers listed on the MR.
//
// Active reviewers are taken from mr.Reviewers (the GitLab Reviewers field).
// Approval state comes from approvals.ApprovedBy.
// Discussion notes are scanned for:
//   - Non-system notes authored by a reviewer → updates lastComment timestamp.
//   - System notes matching "requested review from @<username>" → updates lastReReview timestamp.
func DeriveReviewerStates(
	mr *gl.MergeRequest,
	discussions []*gl.Discussion,
	approvals *gl.MergeRequestApprovals,
) []domain.ReviewerInfo {
	if len(mr.Reviewers) == 0 {
		return nil
	}

	// Build set of approved usernames.
	approvedBy := make(map[string]bool, len(approvals.ApprovedBy))
	for _, a := range approvals.ApprovedBy {
		if a.User != nil {
			approvedBy[a.User.Username] = true
		}
	}

	type reviewerTimestamps struct {
		lastComment  time.Time
		lastReReview time.Time
	}

	ts := make(map[string]*reviewerTimestamps, len(mr.Reviewers))
	for _, r := range mr.Reviewers {
		ts[r.Username] = &reviewerTimestamps{}
	}

	// Walk all notes in chronological order.
	// GitLab returns discussions in chronological order; notes within a
	// discussion are also in order. We iterate discussion by discussion, note
	// by note — effectively chronological.
	for _, d := range discussions {
		for _, note := range d.Notes {
			if note.CreatedAt == nil {
				continue
			}
			t := *note.CreatedAt

			if note.System {
				// Check for re-review request system notes.
				username := extractReReviewUsername(note.Body)
				if username == "" {
					continue
				}
				if rts, ok := ts[username]; ok {
					if t.After(rts.lastReReview) {
						rts.lastReReview = t
					}
				}
				continue
			}

			// Non-system note: check if the author is an active reviewer.
			if rts, ok := ts[note.Author.Username]; ok {
				if t.After(rts.lastComment) {
					rts.lastComment = t
				}
			}
		}
	}

	result := make([]domain.ReviewerInfo, 0, len(mr.Reviewers))
	for _, r := range mr.Reviewers {
		rts := ts[r.Username]
		state := deriveState(approvedBy[r.Username], rts.lastComment, rts.lastReReview)

		var waitingSince time.Time
		switch state {
		case domain.ReviewerNotStarted, domain.ReviewerReReviewRequested:
			waitingSince = rts.lastReReview
			if waitingSince.IsZero() && mr.CreatedAt != nil {
				waitingSince = *mr.CreatedAt
			}
		}

		result = append(result, domain.ReviewerInfo{
			Username:     r.Username,
			Name:         r.Name,
			State:        state,
			WaitingSince: waitingSince,
		})
	}
	return result
}

// extractReReviewUsername parses a system note body and returns the reviewer
// username if the note is a re-review request, or "" otherwise.
func extractReReviewUsername(body string) string {
	if !strings.HasPrefix(body, reReviewPrefix) {
		return ""
	}
	username := strings.TrimPrefix(body, reReviewPrefix)
	// Strip any trailing punctuation or whitespace GitLab might append.
	username = strings.TrimRight(username, " \t\n\r")
	return username
}

// deriveState applies the four state rules to produce the reviewer's state.
func deriveState(approved bool, lastComment, lastReReview time.Time) domain.ReviewerState {
	if approved {
		return domain.ReviewerApproved
	}
	if lastComment.IsZero() && lastReReview.IsZero() {
		return domain.ReviewerNotStarted
	}
	if !lastReReview.IsZero() && (lastComment.IsZero() || lastReReview.After(lastComment)) {
		return domain.ReviewerReReviewRequested
	}
	return domain.ReviewerCommented
}

// MapMR converts raw GitLab API responses into a domain.MergeRequest.
// requiredApprovals comes from project config, not the API, so it is passed in.
func MapMR(
	mr *gl.MergeRequest,
	discussions []*gl.Discussion,
	approvals *gl.MergeRequestApprovals,
	requiredApprovals int,
) domain.MergeRequest {
	reviewers := DeriveReviewerStates(mr, discussions, approvals)

	openThreads := countOpenThreads(discussions)

	var createdAt time.Time
	if mr.CreatedAt != nil {
		createdAt = *mr.CreatedAt
	}

	domainMR := domain.MergeRequest{
		ID:                mr.ID,
		IID:               mr.IID,
		ProjectID:         mr.ProjectID,
		Title:             mr.Title,
		WebURL:            mr.WebURL,
		Reviewers:         reviewers,
		CreatedAt:         createdAt,
		ApprovalCount:     approvals.ApprovalsRequired - approvals.ApprovalsLeft,
		RequiredApprovals: requiredApprovals,
		OpenThreads:       openThreads,
		RoundTripCount:    countRoundTrips(discussions),
	}
	if mr.Author != nil {
		domainMR.Author = mr.Author.Username
	}

	domainMR.Phase = domain.ClassifyPhase(
		mr.Draft || mr.WorkInProgress,
		openThreads,
		domainMR.ApprovalCount,
		requiredApprovals,
		reviewers,
	)

	return domainMR
}

// countRoundTrips returns the total number of "requested review from @X" system
// notes across all discussions. Each re-request counts independently; no dedup.
func countRoundTrips(discussions []*gl.Discussion) int {
	count := 0
	for _, d := range discussions {
		for _, note := range d.Notes {
			if note.System && extractReReviewUsername(note.Body) != "" {
				count++
			}
		}
	}
	return count
}

// countOpenThreads returns the number of unresolved, resolvable discussion threads.
func countOpenThreads(discussions []*gl.Discussion) int {
	count := 0
	for _, d := range discussions {
		if len(d.Notes) == 0 {
			continue
		}
		first := d.Notes[0]
		if first.Resolvable && !first.Resolved {
			count++
		}
	}
	return count
}

// MapMRApprovalCount returns the number of approvals granted, derived from
// ApprovalsRequired - ApprovalsLeft (avoids negative values).
func MapMRApprovalCount(approvals *gl.MergeRequestApprovals) int {
	count := approvals.ApprovalsRequired - approvals.ApprovalsLeft
	if count < 0 {
		return 0
	}
	return count
}
