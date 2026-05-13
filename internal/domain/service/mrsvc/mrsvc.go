// Package mrsvc owns the MergeRequest service port and its configuration types.
// Adapters in internal/adapters/ implement MergeRequestSource; the TUI and
// future handlers depend only on this package — never on concrete infra.
package mrsvc

import (
	"context"

	"github.com/ceffo/mrboard/internal/domain"
)

// MergeRequestSource is the driven port for fetching MR data.
type MergeRequestSource interface {
	// FetchAll retrieves all open MRs from all configured sources.
	// Partial results are valid: non-nil MRs may accompany non-nil errors.
	FetchAll(ctx context.Context) ([]domain.MergeRequest, []error)

	// GetDetail fetches the description and discussion threads for a single MR.
	GetDetail(ctx context.Context, projectID, mrIID int64) (description string, threads []domain.Thread, err error)
}

// Source describes a single source of MRs (group or user).
type Source struct {
	Type     string
	ID       string
	Username string
}

// Config is the service-level configuration for the MR fetching logic.
type Config struct {
	Sources         []Source
	ExcludedAuthors []string
	CurrentUser     string
}
