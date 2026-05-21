package gitlabadpt

import (
	"strconv"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// reReviewPrefix is the system note body prefix GitLab emits when an author
// re-requests review from a specific reviewer.
const reReviewPrefix = "requested review from @"

// approvedBody is the GitLab system note body emitted when a reviewer approves.
const approvedBody = "approved this merge request"

// DeriveReviewerStates processes GitLab discussions chronologically and returns
// a ReviewerInfo slice for the active reviewers listed on the MR.
func DeriveReviewerStates(
	mr *gl.BasicMergeRequest,
	discussions []*gl.Discussion,
	approvals *gl.MergeRequestApprovals,
) []domain.ReviewerInfo {
	if len(mr.Reviewers) == 0 {
		return nil
	}

	approvedBy := make(map[string]bool, len(approvals.ApprovedBy))
	for _, a := range approvals.ApprovedBy {
		if a.User != nil {
			approvedBy[a.User.Username] = true
		}
	}

	type reviewerTimestamps struct {
		lastComment  time.Time
		lastReReview time.Time
		lastApproval time.Time
	}

	ts := make(map[string]*reviewerTimestamps, len(mr.Reviewers))
	for _, r := range mr.Reviewers {
		ts[r.Username] = &reviewerTimestamps{}
	}

	for _, d := range discussions {
		// Resolved threads are no longer the author's ball — skip them when
		// computing lastComment so a resolved reviewer comment doesn't keep
		// the MR in NeedsAuthorAction.
		if len(d.Notes) > 0 && d.Notes[0].Resolvable && d.Notes[0].Resolved {
			continue
		}
		for _, note := range d.Notes {
			if note.CreatedAt == nil {
				continue
			}
			t := *note.CreatedAt

			if note.System {
				if note.Body == approvedBody {
					if rts, ok := ts[note.Author.Username]; ok && t.After(rts.lastApproval) {
						rts.lastApproval = t
					}
					continue
				}
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
		case domain.ReviewerReReviewRequested:
			waitingSince = rts.lastReReview
			if waitingSince.IsZero() && mr.CreatedAt != nil {
				waitingSince = *mr.CreatedAt
			}
		case domain.ReviewerCommented:
			waitingSince = rts.lastComment
		}

		var approvedAt time.Time
		if state == domain.ReviewerApproved {
			approvedAt = rts.lastApproval
		}

		result = append(result, domain.ReviewerInfo{
			Username:     r.Username,
			Name:         r.Name,
			State:        state,
			WaitingSince: waitingSince,
			ApprovedAt:   approvedAt,
		})
	}
	return result
}

func extractReReviewUsername(body string) string {
	if !strings.HasPrefix(body, reReviewPrefix) {
		return ""
	}
	username := strings.TrimPrefix(body, reReviewPrefix)
	username = strings.TrimRight(username, " \t\n\r")
	return username
}

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
func MapMR(
	mr *gl.BasicMergeRequest,
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
		ID:                int(mr.ID),
		IID:               int(mr.IID),
		ProjectID:         int(mr.ProjectID),
		Title:             mr.Title,
		WebURL:            mr.WebURL,
		ProjectPath:       projectPathFromRef(mr.References),
		Reviewers:         reviewers,
		CreatedAt:         createdAt,
		ApprovalCount:     countApprovals(reviewers),
		RequiredApprovals: requiredApprovals,
		OpenThreads:       openThreads,
		RoundTripCount:    countRoundTrips(discussions),
	}
	if mr.Author != nil {
		domainMR.Author = mr.Author.Username
		domainMR.AuthorName = mr.Author.Name
	}

	domainMR.Phase = domain.ClassifyPhase(
		mr.Draft,
		openThreads,
		domainMR.ApprovalCount,
		requiredApprovals,
		reviewers,
	)

	switch domainMR.Phase {
	case domain.PhaseNeedsAuthorAction:
		for _, r := range reviewers {
			if r.State == domain.ReviewerCommented && r.WaitingSince.After(domainMR.WaitingSince) {
				domainMR.WaitingSince = r.WaitingSince
			}
		}
	case domain.PhaseNeedsReview:
		domainMR.WaitingSince = createdAt
		for _, r := range reviewers {
			if r.State == domain.ReviewerReReviewRequested && r.WaitingSince.After(domainMR.WaitingSince) {
				domainMR.WaitingSince = r.WaitingSince
			}
		}
	case domain.PhaseReadyToMerge:
		domainMR.ReadyToMergeSince = deriveReadyToMergeSince(reviewers, requiredApprovals)
	}

	return domainMR
}

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

func countApprovals(reviewers []domain.ReviewerInfo) int {
	n := 0
	for _, r := range reviewers {
		if r.State == domain.ReviewerApproved {
			n++
		}
	}
	return n
}

// MapDiscussionsToThreads converts raw GitLab discussions into domain threads,
// filtering out system-only threads.
func MapDiscussionsToThreads(discussions []*gl.Discussion) []domain.Thread {
	threads := make([]domain.Thread, 0, len(discussions))
	for _, d := range discussions {
		var notes []domain.DiscussionNote
		allSystem := true
		for _, n := range d.Notes {
			if !n.System {
				allSystem = false
			}
			var t time.Time
			if n.CreatedAt != nil {
				t = *n.CreatedAt
			}
			notes = append(notes, domain.DiscussionNote{
				Author:    n.Author.Name,
				Body:      n.Body,
				CreatedAt: t,
				System:    n.System,
			})
		}
		if allSystem || len(notes) == 0 {
			continue
		}
		resolved := false
		if len(d.Notes) > 0 {
			resolved = d.Notes[0].Resolved
		}
		threads = append(threads, domain.Thread{Notes: notes, Resolved: resolved})
	}
	return threads
}

func projectPathFromRef(refs *gl.IssueReferences) string {
	if refs == nil {
		return ""
	}
	full := refs.Full
	if i := strings.LastIndex(full, "!"); i > 0 {
		return full[:i]
	}
	return ""
}

// MapMRFromGraphQL converts a GitLab GraphQL MR response into a domain.MergeRequest.
// If the MR's discussions overflowed (hasNextPage=true) the caller should have already
// logged a warning; this function uses whatever data was returned.
func MapMRFromGraphQL(mr pkggitlab.GQLMergeRequest, requiredApprovals int) domain.MergeRequest {
	reviewers := DeriveReviewerStatesFromGQL(mr)
	openThreads := countOpenThreadsGQL(mr)

	createdAt, _ := time.Parse(time.RFC3339, mr.CreatedAt) //nolint:errcheck

	domainMR := domain.MergeRequest{
		ID:                parseGIDNumericSafe(mr.ID),
		IID:               parseIIDSafe(mr.IID),
		ProjectID:         parseGIDNumericSafe(mr.Project.ID),
		Title:             mr.Title,
		WebURL:            mr.WebURL,
		Author:            mr.Author.Username,
		AuthorName:        mr.Author.Name,
		ProjectPath:       mr.Project.FullPath,
		Reviewers:         reviewers,
		CreatedAt:         createdAt,
		ApprovalCount:     countApprovals(reviewers),
		RequiredApprovals: requiredApprovals,
		OpenThreads:       openThreads,
		RoundTripCount:    countRoundTripsGQL(mr),
	}

	domainMR.Phase = domain.ClassifyPhase(
		mr.Draft,
		openThreads,
		domainMR.ApprovalCount,
		requiredApprovals,
		reviewers,
	)

	switch domainMR.Phase {
	case domain.PhaseNeedsAuthorAction:
		for _, r := range reviewers {
			if r.State == domain.ReviewerCommented && r.WaitingSince.After(domainMR.WaitingSince) {
				domainMR.WaitingSince = r.WaitingSince
			}
		}
	case domain.PhaseNeedsReview:
		domainMR.WaitingSince = createdAt
		for _, r := range reviewers {
			if r.State == domain.ReviewerReReviewRequested && r.WaitingSince.After(domainMR.WaitingSince) {
				domainMR.WaitingSince = r.WaitingSince
			}
		}
	case domain.PhaseReadyToMerge:
		domainMR.ReadyToMergeSince = deriveReadyToMergeSince(reviewers, requiredApprovals)
	}

	return domainMR
}

// DeriveReviewerStatesFromGQL is the GraphQL equivalent of DeriveReviewerStates.
func DeriveReviewerStatesFromGQL(mr pkggitlab.GQLMergeRequest) []domain.ReviewerInfo {
	if len(mr.Reviewers.Nodes) == 0 {
		return nil
	}

	approvedBy := make(map[string]bool, len(mr.ApprovedBy.Nodes))
	for _, u := range mr.ApprovedBy.Nodes {
		approvedBy[u.Username] = true
	}

	type reviewerTimestamps struct {
		lastComment  time.Time
		lastReReview time.Time
		lastApproval time.Time
	}
	ts := make(map[string]*reviewerTimestamps, len(mr.Reviewers.Nodes))
	for _, r := range mr.Reviewers.Nodes {
		ts[r.Username] = &reviewerTimestamps{}
	}

	for _, d := range mr.Discussions.Nodes {
		// Resolved threads are no longer the author's ball — skip them when
		// computing lastComment so a resolved reviewer comment doesn't keep
		// the MR in NeedsAuthorAction.
		if len(d.Notes.Nodes) > 0 && d.Notes.Nodes[0].Resolvable && d.Notes.Nodes[0].Resolved {
			continue
		}
		for _, note := range d.Notes.Nodes {
			if note.CreatedAt == "" {
				continue
			}
			t, err := time.Parse(time.RFC3339, note.CreatedAt)
			if err != nil {
				continue
			}
			if note.System {
				if note.Body == approvedBody {
					if rts, ok := ts[note.Author.Username]; ok && t.After(rts.lastApproval) {
						rts.lastApproval = t
					}
					continue
				}
				username := extractReReviewUsername(note.Body)
				if username == "" {
					continue
				}
				if rts, ok := ts[username]; ok && t.After(rts.lastReReview) {
					rts.lastReReview = t
				}
				continue
			}
			if rts, ok := ts[note.Author.Username]; ok && t.After(rts.lastComment) {
				rts.lastComment = t
			}
		}
	}

	mrCreatedAt, _ := time.Parse(time.RFC3339, mr.CreatedAt) //nolint:errcheck

	result := make([]domain.ReviewerInfo, 0, len(mr.Reviewers.Nodes))
	for _, r := range mr.Reviewers.Nodes {
		rts := ts[r.Username]
		state := deriveState(approvedBy[r.Username], rts.lastComment, rts.lastReReview)
		var waitingSince time.Time
		switch state {
		case domain.ReviewerReReviewRequested:
			waitingSince = rts.lastReReview
			if waitingSince.IsZero() {
				waitingSince = mrCreatedAt
			}
		case domain.ReviewerCommented:
			waitingSince = rts.lastComment
		}
		var approvedAt time.Time
		if state == domain.ReviewerApproved {
			approvedAt = rts.lastApproval
		}
		result = append(result, domain.ReviewerInfo{
			Username:     r.Username,
			Name:         r.Name,
			State:        state,
			WaitingSince: waitingSince,
			ApprovedAt:   approvedAt,
		})
	}
	return result
}

func countOpenThreadsGQL(mr pkggitlab.GQLMergeRequest) int {
	count := 0
	for _, d := range mr.Discussions.Nodes {
		if len(d.Notes.Nodes) == 0 {
			continue
		}
		first := d.Notes.Nodes[0]
		if first.Resolvable && !first.Resolved {
			count++
		}
	}
	return count
}

func countRoundTripsGQL(mr pkggitlab.GQLMergeRequest) int {
	count := 0
	for _, d := range mr.Discussions.Nodes {
		for _, note := range d.Notes.Nodes {
			if note.System && extractReReviewUsername(note.Body) != "" {
				count++
			}
		}
	}
	return count
}

// deriveReadyToMergeSince returns when the MR last reached the required approval
// count by finding the requiredApprovals-th oldest ApprovedAt timestamp.
// Returns zero time if timestamps are unavailable (e.g. zero required approvals).
func deriveReadyToMergeSince(reviewers []domain.ReviewerInfo, requiredApprovals int) time.Time {
	if requiredApprovals <= 0 {
		return time.Time{}
	}
	var times []time.Time
	for _, r := range reviewers {
		if r.State == domain.ReviewerApproved && !r.ApprovedAt.IsZero() {
			times = append(times, r.ApprovedAt)
		}
	}
	if len(times) < requiredApprovals {
		return time.Time{}
	}
	// Sort ascending; the requiredApprovals-th entry is when the count hit the threshold.
	for i := 1; i < len(times); i++ {
		for j := i; j > 0 && times[j].Before(times[j-1]); j-- {
			times[j], times[j-1] = times[j-1], times[j]
		}
	}
	return times[requiredApprovals-1]
}

// parseGIDNumericSafe extracts the trailing numeric ID from a GitLab global ID
// like "gid://gitlab/MergeRequest/456". Returns 0 on failure.
func parseGIDNumericSafe(gid string) int {
	i := strings.LastIndex(gid, "/")
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(gid[i+1:]) //nolint:errcheck
	return n
}

// parseIIDSafe converts a GitLab GraphQL IID string (e.g. "42") to int.
func parseIIDSafe(iid string) int {
	n, _ := strconv.Atoi(iid) //nolint:errcheck
	return n
}
