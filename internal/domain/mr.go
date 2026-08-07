// Package domain defines the core types and business logic for mrboard.
package domain

import (
	"fmt"
	"time"
)

const (
	minutesPerHour = 60
	hoursPerDay    = 24
	daysPerMonth   = 30
	monthsPerYear  = 12
)

// DiscussionNote is a single note within a discussion thread.
type DiscussionNote struct {
	Author    string
	Body      string
	CreatedAt time.Time
	System    bool
}

// Thread is a collapsible discussion thread on an MR.
type Thread struct {
	Notes    []DiscussionNote
	Resolved bool
}

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

// String returns the snake_case name used in JSON output and logs.
func (s ReviewerState) String() string {
	switch s {
	case ReviewerNotStarted:
		return "not_started"
	case ReviewerCommented:
		return "commented"
	case ReviewerReReviewRequested:
		return "re_review_requested"
	case ReviewerApproved:
		return "approved"
	default:
		return "unknown"
	}
}

// String returns the snake_case name used in JSON output and logs.
func (p MRPhase) String() string {
	switch p {
	case PhaseDraft:
		return "draft"
	case PhaseNeedsReview:
		return "needs_review"
	case PhaseNeedsAuthorAction:
		return "needs_author_action"
	case PhaseReadyToMerge:
		return "ready_to_merge"
	default:
		return "unknown"
	}
}

// User is a GitLab instance user, used for username-to-ID resolution.
// Distinct from ProjectMember: a User is resolved instance-wide from a username,
// while ProjectMember is project-scoped membership.
type User struct {
	ID       int64
	Username string
	Name     string
}

// ProjectMember is a GitLab project member (Developer or higher access level).
type ProjectMember struct {
	UserID   int64
	Username string
	Name     string
}

// ReviewerInfo holds the current state for a single reviewer on an MR.
type ReviewerInfo struct {
	Username     string
	Name         string
	State        ReviewerState
	WaitingSince time.Time
	ApprovedAt   time.Time // zero unless State == ReviewerApproved
	IsApprover   bool      // member of the "Approvers" approval rule
}

// FileDiff holds the diff for a single file in an MR.
type FileDiff struct {
	OldPath      string
	NewPath      string
	NewFile      bool
	DeletedFile  bool
	RenamedFile  bool
	TooLarge     bool
	Diff         string // raw unified diff text
	LinesAdded   int
	LinesRemoved int
}

// MRDiff holds diff refs and per-file diffs for a single MR.
type MRDiff struct {
	BaseSHA string
	HeadSHA string
	Files   []FileDiff
}

// MRKey uniquely identifies a merge request across all sources, independent of
// position in any slice or column. It is the single identity type for selection
// tracking, dedup, and cache lookups — see docs/adr/0005.
type MRKey struct {
	ProjectID int
	IID       int
}

// Key returns mr's identity for selection tracking, dedup, and cache lookups.
func (mr MergeRequest) Key() MRKey {
	return MRKey{ProjectID: mr.ProjectID, IID: mr.IID}
}

// MergeRequest is the core domain type representing a GitLab merge request.
type MergeRequest struct {
	ID           int
	IID          int
	ProjectID    int
	Title        string
	Author       string // GitLab username — canonical ID
	AuthorName   string // display name; falls back to Author if empty
	Assignee     string // GitLab username of the current assignee
	AssigneeName string // display name; falls back to Assignee if empty
	WebURL       string
	ProjectPath  string // namespace/project without domain, e.g. "group/repo"
	Description  string

	Phase               MRPhase
	DetailedMergeStatus string // raw value from GitLab's detailed_merge_status field
	SourceBranch        string // raw value from GitLab's source_branch field
	TargetBranch        string // raw value from GitLab's target_branch field
	Reviewers           []ReviewerInfo

	// Approvers is the full membership of the "Approvers" approval rule —
	// usernames eligible to approve, regardless of whether they are
	// currently assigned as a reviewer on this MR. Distinct from
	// ReviewerInfo.IsApprover, which only flags entries already present
	// in Reviewers.
	Approvers []string

	CreatedAt         time.Time
	UpdatedAt         time.Time // GitLab's updated_at; bumped on notes, approvals, reviewer changes, title/draft edits
	NonDraftSince     time.Time
	WaitingSince      time.Time
	ReadyToMergeSince time.Time

	OpenThreads    int
	RoundTripCount int

	ReviewerSource bool // true when this MR came only from a reviewer-source fetch

	IssueType string // populated asynchronously; "" means not yet fetched or no linked ticket
}

// DisplayAuthor returns the human-readable author name, falling back to the username.
func (mr MergeRequest) DisplayAuthor() string {
	if mr.AuthorName != "" {
		return mr.AuthorName
	}
	return mr.Author
}

// DisplayAssignee returns the human-readable assignee name, falling back to the
// assignee username, and then to DisplayAuthor when no assignee is set.
func (mr MergeRequest) DisplayAssignee() string {
	if mr.AssigneeName != "" {
		return mr.AssigneeName
	}
	if mr.Assignee != "" {
		return mr.Assignee
	}
	return mr.DisplayAuthor()
}

// ClassifyPhase determines the MRPhase from the MR's fields.
// Evaluated in priority order per docs/domain-model.md.
// mergeable is kept for caller compatibility but no longer drives phase assignment.
// The "Approved" column (PhaseReadyToMerge) is entered when all IsApprover reviewers
// have approved. If there are no designated approvers, the MR stays in NeedsReview.
func ClassifyPhase(draft bool, _ bool, reviewers []ReviewerInfo) MRPhase {
	if draft {
		return PhaseDraft
	}
	approverCount, approvedCount := 0, 0
	for _, r := range reviewers {
		if r.IsApprover {
			approverCount++
			if r.State == ReviewerApproved {
				approvedCount++
			}
		}
	}
	if approverCount > 0 && approvedCount == approverCount {
		return PhaseReadyToMerge
	}
	for _, r := range reviewers {
		if r.State == ReviewerCommented {
			return PhaseNeedsAuthorAction
		}
	}
	return PhaseNeedsReview
}

// DeriveWaitingSince computes MergeRequest.WaitingSince for a given phase, per
// docs/domain-model.md: NeedsAuthorAction tracks the latest reviewer comment;
// NeedsReview tracks the latest re-review request, falling back to the MR's
// creation time if none has occurred. Other phases have no WaitingSince.
func DeriveWaitingSince(phase MRPhase, reviewers []ReviewerInfo, createdAt time.Time) time.Time {
	switch phase {
	case PhaseNeedsAuthorAction:
		var since time.Time
		for _, r := range reviewers {
			if r.State == ReviewerCommented && r.WaitingSince.After(since) {
				since = r.WaitingSince
			}
		}
		return since
	case PhaseNeedsReview:
		since := createdAt
		for _, r := range reviewers {
			if r.State == ReviewerReReviewRequested && r.WaitingSince.After(since) {
				since = r.WaitingSince
			}
		}
		return since
	default:
		return time.Time{}
	}
}

// FormatDuration formats a duration as a human-readable string.
// Units: < 1m, Xm, Xh Ym, Xd Yh, Xmo Yd, Xy Xmo.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return "< 1m"
	}
	totalMinutes := int(d.Minutes())
	totalDays := totalMinutes / (minutesPerHour * hoursPerDay)

	if totalDays >= daysPerMonth*monthsPerYear {
		years := totalDays / (daysPerMonth * monthsPerYear)
		months := (totalDays % (daysPerMonth * monthsPerYear)) / daysPerMonth
		if months > 0 {
			return fmt.Sprintf("%dy %dmo", years, months)
		}
		return fmt.Sprintf("%dy", years)
	}

	if totalDays >= daysPerMonth {
		months := totalDays / daysPerMonth
		days := totalDays % daysPerMonth
		if days > 0 {
			return fmt.Sprintf("%dmo %dd", months, days)
		}
		return fmt.Sprintf("%dmo", months)
	}

	hours := (totalMinutes % (minutesPerHour * hoursPerDay)) / minutesPerHour
	minutes := totalMinutes % minutesPerHour

	switch {
	case totalDays > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", totalDays, hours)
	case totalDays > 0:
		return fmt.Sprintf("%dd", totalDays)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}
