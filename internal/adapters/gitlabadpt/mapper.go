package gitlabadpt

import (
	"strconv"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

const (
	// approversRuleName is the canonical name of the GitLab MR approval rule managed by mrboard.
	approversRuleName = "Approvers"

	// reReviewPrefix is the system note body prefix GitLab emits when an author
	// re-requests review from a specific reviewer.
	reReviewPrefix = "requested review from @"
)

// detailedMergeStatusMergeable is the GitLab detailed_merge_status value that means the MR can be merged.
const detailedMergeStatusMergeable = "mergeable"

// approvedBody is the GitLab system note body emitted when a reviewer approves.
const approvedBody = "approved this merge request"

// normalizeDiscussionEventsREST converts REST discussions to domain.DiscussionEvents.
// Resolved threads are skipped so a resolved reviewer comment does not keep the
// MR in NeedsAuthorAction.
func normalizeDiscussionEventsREST(discussions []*gl.Discussion) []domain.DiscussionEvent {
	var events []domain.DiscussionEvent
	for _, d := range discussions {
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
					events = append(events, domain.DiscussionEvent{
						AuthorUsername: note.Author.Username,
						Timestamp:      t,
						Kind:           domain.KindApproval,
					})
					continue
				}
				if username := extractReReviewUsername(note.Body); username != "" {
					events = append(events, domain.DiscussionEvent{
						AuthorUsername: username,
						Timestamp:      t,
						Kind:           domain.KindReReviewRequest,
					})
				}
				continue
			}
			events = append(events, domain.DiscussionEvent{
				AuthorUsername: note.Author.Username,
				Timestamp:      t,
				Kind:           domain.KindComment,
			})
		}
	}
	return events
}

// normalizeDiscussionEventsGQL converts GQL discussions to domain.DiscussionEvents.
// Resolved threads are skipped for the same reason as the REST version.
func normalizeDiscussionEventsGQL(discussions []pkggitlab.GQLDiscussion) []domain.DiscussionEvent {
	var events []domain.DiscussionEvent
	for _, d := range discussions {
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
					events = append(events, domain.DiscussionEvent{
						AuthorUsername: note.Author.Username,
						Timestamp:      t,
						Kind:           domain.KindApproval,
					})
					continue
				}
				if username := extractReReviewUsername(note.Body); username != "" {
					events = append(events, domain.DiscussionEvent{
						AuthorUsername: username,
						Timestamp:      t,
						Kind:           domain.KindReReviewRequest,
					})
				}
				continue
			}
			events = append(events, domain.DiscussionEvent{
				AuthorUsername: note.Author.Username,
				Timestamp:      t,
				Kind:           domain.KindComment,
			})
		}
	}
	return events
}

// threadState holds the resolution flags from the first note of a discussion thread.
type threadState struct {
	resolvable bool
	resolved   bool
}

// countOpenThreads counts threads that are resolvable but not yet resolved.
func countOpenThreads(threads []threadState) int {
	count := 0
	for _, t := range threads {
		if t.resolvable && !t.resolved {
			count++
		}
	}
	return count
}

func restThreadStates(discussions []*gl.Discussion) []threadState {
	states := make([]threadState, 0, len(discussions))
	for _, d := range discussions {
		if len(d.Notes) == 0 {
			continue
		}
		states = append(states, threadState{d.Notes[0].Resolvable, d.Notes[0].Resolved})
	}
	return states
}

func gqlThreadStates(discussions []pkggitlab.GQLDiscussion) []threadState {
	states := make([]threadState, 0, len(discussions))
	for _, d := range discussions {
		if len(d.Notes.Nodes) == 0 {
			continue
		}
		states = append(states, threadState{d.Notes.Nodes[0].Resolvable, d.Notes.Nodes[0].Resolved})
	}
	return states
}

// DeriveReviewerStates processes GitLab REST discussions chronologically and returns
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
	refs := make([]domain.ReviewerInfo, len(mr.Reviewers))
	for i, r := range mr.Reviewers {
		refs[i] = domain.ReviewerInfo{Username: r.Username, Name: r.Name}
	}
	var mrCreatedAt time.Time
	if mr.CreatedAt != nil {
		mrCreatedAt = *mr.CreatedAt
	}
	return domain.DeriveReviewerInfos(refs, normalizeDiscussionEventsREST(discussions), approvedBy, mrCreatedAt)
}

func extractReReviewUsername(body string) string {
	if !strings.HasPrefix(body, reReviewPrefix) {
		return ""
	}
	username := strings.TrimPrefix(body, reReviewPrefix)
	username = strings.TrimRight(username, " \t\n\r")
	return username
}

// approverSetFromRESTRules extracts eligible approver usernames from the "Approvers" rule.
func approverSetFromRESTRules(rules []*gl.MergeRequestApprovalRule) map[string]bool {
	for _, r := range rules {
		if r.Name == approversRuleName {
			set := make(map[string]bool, len(r.EligibleApprovers))
			for _, u := range r.EligibleApprovers {
				set[u.Username] = true
			}
			return set
		}
	}
	return nil
}

// approverSetFromGQLRules extracts eligible approver usernames from the GQL "Approvers" rule.
func approverSetFromGQLRules(rules []pkggitlab.GQLApprovalRule) map[string]bool {
	for _, r := range rules {
		if r.Name == approversRuleName {
			set := make(map[string]bool, len(r.EligibleApprovers))
			for _, u := range r.EligibleApprovers {
				set[u.Username] = true
			}
			return set
		}
	}
	return nil
}

// applyApproverFlag sets IsApprover on each ReviewerInfo whose username is in the approver set.
func applyApproverFlag(reviewers []domain.ReviewerInfo, approvers map[string]bool) []domain.ReviewerInfo {
	for i := range reviewers {
		reviewers[i].IsApprover = approvers[reviewers[i].Username]
	}
	return reviewers
}

// MapMR converts raw GitLab API responses into a domain.MergeRequest.
func MapMR(
	mr *gl.BasicMergeRequest,
	discussions []*gl.Discussion,
	approvals *gl.MergeRequestApprovals,
	approvalRules []*gl.MergeRequestApprovalRule,
) domain.MergeRequest {
	approvedBy := make(map[string]bool, len(approvals.ApprovedBy))
	for _, a := range approvals.ApprovedBy {
		if a.User != nil {
			approvedBy[a.User.Username] = true
		}
	}
	var mrCreatedAt time.Time
	if mr.CreatedAt != nil {
		mrCreatedAt = *mr.CreatedAt
	}
	refs := make([]domain.ReviewerInfo, len(mr.Reviewers))
	for i, r := range mr.Reviewers {
		refs[i] = domain.ReviewerInfo{Username: r.Username, Name: r.Name}
	}
	events := normalizeDiscussionEventsREST(discussions)
	reviewers := domain.DeriveReviewerInfos(refs, events, approvedBy, mrCreatedAt)
	reviewers = applyApproverFlag(reviewers, approverSetFromRESTRules(approvalRules))
	openThreads := countOpenThreads(restThreadStates(discussions))

	domainMR := domain.MergeRequest{
		ID:             int(mr.ID),
		IID:            int(mr.IID),
		ProjectID:      int(mr.ProjectID),
		Title:          mr.Title,
		WebURL:         mr.WebURL,
		ProjectPath:    projectPathFromRef(mr.References),
		Reviewers:      reviewers,
		CreatedAt:      mrCreatedAt,
		OpenThreads:    openThreads,
		RoundTripCount: domain.CountRoundTrips(events),
	}
	if mr.Author != nil {
		domainMR.Author = mr.Author.Username
		domainMR.AuthorName = mr.Author.Name
	}

	domainMR.DetailedMergeStatus = mr.DetailedMergeStatus
	domainMR.Phase = domain.ClassifyPhase(mr.Draft, mr.DetailedMergeStatus == detailedMergeStatusMergeable, reviewers)

	switch domainMR.Phase {
	case domain.PhaseNeedsAuthorAction:
		for _, r := range reviewers {
			if r.State == domain.ReviewerCommented && r.WaitingSince.After(domainMR.WaitingSince) {
				domainMR.WaitingSince = r.WaitingSince
			}
		}
	case domain.PhaseNeedsReview:
		domainMR.WaitingSince = mrCreatedAt
		for _, r := range reviewers {
			if r.State == domain.ReviewerReReviewRequested && r.WaitingSince.After(domainMR.WaitingSince) {
				domainMR.WaitingSince = r.WaitingSince
			}
		}
	case domain.PhaseReadyToMerge:
		domainMR.ReadyToMergeSince = deriveReadyToMergeSince(reviewers)
	}

	return domainMR
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
func MapMRFromGraphQL(mr pkggitlab.GQLMergeRequest) domain.MergeRequest {
	approvedBy := make(map[string]bool, len(mr.ApprovedBy.Nodes))
	for _, u := range mr.ApprovedBy.Nodes {
		approvedBy[u.Username] = true
	}
	refs := make([]domain.ReviewerInfo, len(mr.Reviewers.Nodes))
	for i, r := range mr.Reviewers.Nodes {
		refs[i] = domain.ReviewerInfo{Username: r.Username, Name: r.Name}
	}
	createdAt, _ := time.Parse(time.RFC3339, mr.CreatedAt) //nolint:errcheck

	events := normalizeDiscussionEventsGQL(mr.Discussions.Nodes)
	reviewers := domain.DeriveReviewerInfos(refs, events, approvedBy, createdAt)
	reviewers = applyApproverFlag(reviewers, approverSetFromGQLRules(mr.ApprovalState.Rules))
	openThreads := countOpenThreads(gqlThreadStates(mr.Discussions.Nodes))

	domainMR := domain.MergeRequest{
		ID:             parseGIDNumericSafe(mr.ID),
		IID:            parseIIDSafe(mr.IID),
		ProjectID:      parseGIDNumericSafe(mr.Project.ID),
		Title:          mr.Title,
		WebURL:         mr.WebURL,
		Author:         mr.Author.Username,
		AuthorName:     mr.Author.Name,
		ProjectPath:    mr.Project.FullPath,
		Reviewers:      reviewers,
		CreatedAt:      createdAt,
		OpenThreads:    openThreads,
		RoundTripCount: domain.CountRoundTrips(events),
	}

	domainMR.DetailedMergeStatus = strings.ToLower(mr.DetailedMergeStatus)
	isMergeable := domainMR.DetailedMergeStatus == detailedMergeStatusMergeable
	domainMR.Phase = domain.ClassifyPhase(mr.Draft, isMergeable, reviewers)

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
		domainMR.ReadyToMergeSince = deriveReadyToMergeSince(reviewers)
	}

	return domainMR
}

// deriveReadyToMergeSince returns the latest approval timestamp, used as a
// proxy for when the MR became ready to merge.
func deriveReadyToMergeSince(reviewers []domain.ReviewerInfo) time.Time {
	var latest time.Time
	for _, r := range reviewers {
		if r.State == domain.ReviewerApproved && r.ApprovedAt.After(latest) {
			latest = r.ApprovedAt
		}
	}
	return latest
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
