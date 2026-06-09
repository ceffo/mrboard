package domain

import "time"

// DiscussionEventKind classifies the action recorded in a DiscussionEvent.
type DiscussionEventKind int

const (
	// KindComment is a non-system note left by a reviewer.
	KindComment DiscussionEventKind = iota
	// KindReReviewRequest is a re-review system note; AuthorUsername is the targeted reviewer.
	KindReReviewRequest
	// KindApproval is the approval system note; AuthorUsername is the approving reviewer.
	KindApproval
)

// DiscussionEvent is a normalized, source-agnostic representation of a single
// action in a merge request discussion thread. REST and GQL responses each have
// an adapter that produces []DiscussionEvent; this package derives reviewer state
// from that stream without knowing the origin.
type DiscussionEvent struct {
	// AuthorUsername is who acted (KindComment/KindApproval) or who was targeted (KindReReviewRequest).
	AuthorUsername string
	Timestamp      time.Time
	Kind           DiscussionEventKind
}

// DeriveReviewerInfos derives the full ReviewerInfo slice from a normalized event
// stream, the approved-by set, and the MR creation time.
// Only Username and Name are read from the input reviewers; State, WaitingSince,
// and ApprovedAt are computed and returned on the output slice.
func DeriveReviewerInfos(
	reviewers []ReviewerInfo,
	events []DiscussionEvent,
	approvedBy map[string]bool,
	mrCreatedAt time.Time,
) []ReviewerInfo {
	type timestamps struct {
		lastComment  time.Time
		lastReReview time.Time
		lastApproval time.Time
	}
	stamps := make(map[string]*timestamps, len(reviewers))
	for _, r := range reviewers {
		stamps[r.Username] = &timestamps{}
	}
	for _, e := range events {
		s, ok := stamps[e.AuthorUsername]
		if !ok {
			continue
		}
		switch e.Kind {
		case KindComment:
			if e.Timestamp.After(s.lastComment) {
				s.lastComment = e.Timestamp
			}
		case KindReReviewRequest:
			if e.Timestamp.After(s.lastReReview) {
				s.lastReReview = e.Timestamp
			}
		case KindApproval:
			if e.Timestamp.After(s.lastApproval) {
				s.lastApproval = e.Timestamp
			}
		}
	}
	result := make([]ReviewerInfo, 0, len(reviewers))
	for _, r := range reviewers {
		s := stamps[r.Username]
		state := deriveReviewerState(approvedBy[r.Username], s.lastComment, s.lastReReview)
		var waitingSince, approvedAt time.Time
		switch state {
		case ReviewerReReviewRequested:
			waitingSince = s.lastReReview
			if waitingSince.IsZero() {
				waitingSince = mrCreatedAt
			}
		case ReviewerCommented:
			waitingSince = s.lastComment
		}
		if state == ReviewerApproved {
			approvedAt = s.lastApproval
		}
		result = append(result, ReviewerInfo{
			Username:     r.Username,
			Name:         r.Name,
			State:        state,
			WaitingSince: waitingSince,
			ApprovedAt:   approvedAt,
		})
	}
	return result
}

// CountRoundTrips counts KindReReviewRequest events, which corresponds to how
// many times the MR bounced between reviewer and author.
func CountRoundTrips(events []DiscussionEvent) int {
	count := 0
	for _, e := range events {
		if e.Kind == KindReReviewRequest {
			count++
		}
	}
	return count
}

// deriveReviewerState classifies a single reviewer's state from their timestamp
// extremes and the authoritative approved flag.
func deriveReviewerState(approved bool, lastComment, lastReReview time.Time) ReviewerState {
	if approved {
		return ReviewerApproved
	}
	if lastComment.IsZero() && lastReReview.IsZero() {
		return ReviewerNotStarted
	}
	if !lastReReview.IsZero() && (lastComment.IsZero() || lastReReview.After(lastComment)) {
		return ReviewerReReviewRequested
	}
	return ReviewerCommented
}
