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

// detailedMergeStatusMergeable is the GitLab detailed_merge_status value that means the MR can be merged.
const detailedMergeStatusMergeable = "mergeable"

// approvedBody is the GitLab system note body emitted when a reviewer approves.
const approvedBody = "approved this merge request"

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
// action in a merge request discussion thread.
type DiscussionEvent struct {
	// AuthorUsername is who acted (KindComment/KindApproval) or who was targeted (KindReReviewRequest).
	AuthorUsername string
	Timestamp      time.Time
	Kind           DiscussionEventKind
}

// normalizeDiscussionEventsREST converts REST discussions to DiscussionEvents.
// Resolved threads are skipped so a resolved reviewer comment does not keep the
// MR in NeedsAuthorAction.
func normalizeDiscussionEventsREST(discussions []*gl.Discussion) []DiscussionEvent {
	var events []DiscussionEvent
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
					events = append(events, DiscussionEvent{
						AuthorUsername: note.Author.Username,
						Timestamp:      t,
						Kind:           KindApproval,
					})
					continue
				}
				if username := extractReReviewUsername(note.Body); username != "" {
					events = append(events, DiscussionEvent{
						AuthorUsername: username,
						Timestamp:      t,
						Kind:           KindReReviewRequest,
					})
				}
				continue
			}
			events = append(events, DiscussionEvent{
				AuthorUsername: note.Author.Username,
				Timestamp:      t,
				Kind:           KindComment,
			})
		}
	}
	return events
}

// normalizeDiscussionEventsGQL converts GQL discussions to DiscussionEvents.
// Resolved threads are skipped for the same reason as the REST version.
func normalizeDiscussionEventsGQL(discussions []pkggitlab.GQLDiscussion) []DiscussionEvent {
	var events []DiscussionEvent
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
					events = append(events, DiscussionEvent{
						AuthorUsername: note.Author.Username,
						Timestamp:      t,
						Kind:           KindApproval,
					})
					continue
				}
				if username := extractReReviewUsername(note.Body); username != "" {
					events = append(events, DiscussionEvent{
						AuthorUsername: username,
						Timestamp:      t,
						Kind:           KindReReviewRequest,
					})
				}
				continue
			}
			events = append(events, DiscussionEvent{
				AuthorUsername: note.Author.Username,
				Timestamp:      t,
				Kind:           KindComment,
			})
		}
	}
	return events
}

// deriveReviewerState is a pure classifier for a single reviewer's state.
// events must be pre-filtered to this reviewer's events only.
// approved is sourced from the authoritative approvals API/field, not from events.
func deriveReviewerState(approved bool, events []DiscussionEvent) domain.ReviewerState {
	var lastComment, lastReReview time.Time
	for _, e := range events {
		switch e.Kind {
		case KindComment:
			if e.Timestamp.After(lastComment) {
				lastComment = e.Timestamp
			}
		case KindReReviewRequest:
			if e.Timestamp.After(lastReReview) {
				lastReReview = e.Timestamp
			}
		}
	}
	return deriveState(approved, lastComment, lastReReview)
}

type reviewerRef struct {
	username string
	name     string
}

// buildReviewerInfos derives the full ReviewerInfo slice from a normalized event
// stream, the set of approved reviewer usernames, and the MR creation time.
func buildReviewerInfos(
	reviewers []reviewerRef,
	events []DiscussionEvent,
	approvedBy map[string]bool,
	mrCreatedAt time.Time,
) []domain.ReviewerInfo {
	type ts struct {
		lastComment  time.Time
		lastReReview time.Time
		lastApproval time.Time
	}
	stamps := make(map[string]*ts, len(reviewers))
	for _, r := range reviewers {
		stamps[r.username] = &ts{}
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
	result := make([]domain.ReviewerInfo, 0, len(reviewers))
	for _, r := range reviewers {
		s := stamps[r.username]
		state := deriveState(approvedBy[r.username], s.lastComment, s.lastReReview)
		var waitingSince, approvedAt time.Time
		switch state {
		case domain.ReviewerReReviewRequested:
			waitingSince = s.lastReReview
			if waitingSince.IsZero() {
				waitingSince = mrCreatedAt
			}
		case domain.ReviewerCommented:
			waitingSince = s.lastComment
		}
		if state == domain.ReviewerApproved {
			approvedAt = s.lastApproval
		}
		result = append(result, domain.ReviewerInfo{
			Username:     r.username,
			Name:         r.name,
			State:        state,
			WaitingSince: waitingSince,
			ApprovedAt:   approvedAt,
		})
	}
	return result
}

// countRoundTripsFromEvents counts the number of KindReReviewRequest events,
// which corresponds to how many times the MR bounced between reviewer and author.
func countRoundTripsFromEvents(events []DiscussionEvent) int {
	count := 0
	for _, e := range events {
		if e.Kind == KindReReviewRequest {
			count++
		}
	}
	return count
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
	refs := make([]reviewerRef, len(mr.Reviewers))
	for i, r := range mr.Reviewers {
		refs[i] = reviewerRef{username: r.Username, name: r.Name}
	}
	var mrCreatedAt time.Time
	if mr.CreatedAt != nil {
		mrCreatedAt = *mr.CreatedAt
	}
	return buildReviewerInfos(refs, normalizeDiscussionEventsREST(discussions), approvedBy, mrCreatedAt)
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

// approverSetFromRESTRules extracts eligible approver usernames from the "Approvers" rule.
func approverSetFromRESTRules(rules []*gl.MergeRequestApprovalRule) map[string]bool {
	for _, r := range rules {
		if r.Name == "Approvers" {
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
		if r.Name == "Approvers" {
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
	refs := make([]reviewerRef, len(mr.Reviewers))
	for i, r := range mr.Reviewers {
		refs[i] = reviewerRef{username: r.Username, name: r.Name}
	}
	events := normalizeDiscussionEventsREST(discussions)
	reviewers := buildReviewerInfos(refs, events, approvedBy, mrCreatedAt)
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
		RoundTripCount: countRoundTripsFromEvents(events),
	}
	if mr.Author != nil {
		domainMR.Author = mr.Author.Username
		domainMR.AuthorName = mr.Author.Name
	}

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
	refs := make([]reviewerRef, len(mr.Reviewers.Nodes))
	for i, r := range mr.Reviewers.Nodes {
		refs[i] = reviewerRef{username: r.Username, name: r.Name}
	}
	createdAt, _ := time.Parse(time.RFC3339, mr.CreatedAt) //nolint:errcheck

	events := normalizeDiscussionEventsGQL(mr.Discussions.Nodes)
	reviewers := buildReviewerInfos(refs, events, approvedBy, createdAt)
	reviewers = applyApproverFlag(reviewers, approverSetFromGQLRules(mr.ApprovalRules))
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
		RoundTripCount: countRoundTripsFromEvents(events),
	}

	domainMR.Phase = domain.ClassifyPhase(mr.Draft, mr.DetailedMergeStatus == detailedMergeStatusMergeable, reviewers)

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
