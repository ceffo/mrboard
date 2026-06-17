// Package mrsvc owns the MergeRequest service port and its configuration types.
// Adapters in internal/adapters/ implement MergeRequestSource; the TUI and
// future handlers depend only on this package — never on concrete infra.
package mrsvc

import (
	"context"

	"github.com/ceffo/mrboard/internal/domain"
)

// FetchOptions controls what FetchAll returns.
type FetchOptions struct {
	// IncludeReviewerMRs, when true, also fetches MRs where the current user
	// is a reviewer (in addition to MRs authored by configured sources).
	IncludeReviewerMRs bool
}

// MergeRequestSource is the driven port for fetching MR data.
type MergeRequestSource interface {
	// FetchAll retrieves all open MRs from all configured sources.
	// Partial results are valid: non-nil MRs may accompany non-nil errors.
	FetchAll(ctx context.Context, opts FetchOptions) ([]domain.MergeRequest, []error)

	// GetDetail fetches the description and discussion threads for a single MR.
	GetDetail(ctx context.Context, projectID, mrIID int64) (description string, threads []domain.Thread, err error)

	// FetchMR fetches a single MR by project ID and MR IID.
	FetchMR(ctx context.Context, projectID int64, mrIID int64) (domain.MergeRequest, error)

	// GetProjectMembers returns all project members with Developer (40) or higher access.
	GetProjectMembers(ctx context.Context, projectID int64) ([]domain.ProjectMember, error)

	// SaveApprovers writes the "Approvers" approval rule with the given user IDs.
	// Creates the rule if it doesn't exist; updates it if it does.
	SaveApprovers(ctx context.Context, projectID int64, mrIID int64, userIDs []int64) error

	// GetDiff fetches diff refs and per-file diffs for a single MR.
	GetDiff(ctx context.Context, projectID, mrIID int64) (domain.MRDiff, error)

	// GetFileContent fetches the raw content of a file at a given ref (SHA or branch).
	GetFileContent(ctx context.Context, projectID int64, path, ref string) ([]byte, error)

	// SetReviewers replaces the MR's reviewer set with the given user IDs.
	// An empty slice clears all reviewers.
	SetReviewers(ctx context.Context, projectID int64, mrIID int64, userIDs []int64) error

	// ResolveUsers looks up GitLab users by username (instance-wide).
	// Unknown usernames are omitted from the result; callers can diff against
	// the input to detect invalid entries.
	ResolveUsers(ctx context.Context, usernames []string) ([]domain.User, error)
}

// SourceType identifies the kind of GitLab entity a source represents.
type SourceType string

// Valid SourceType values.
const (
	SourceTypeGroup SourceType = "group"
	SourceTypeUser  SourceType = "user"
)

// Source describes a single source of MRs.
// IDs holds one or more group paths (for SourceTypeGroup) or usernames (for SourceTypeUser).
type Source struct {
	Type SourceType
	IDs  []string
}

// Config is the service-level configuration for the MR fetching logic.
type Config struct {
	Sources         []Source
	ExcludedAuthors []string
	CurrentUser     string
}
