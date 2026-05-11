// Package service defines ports (interfaces) owned by the business layer.
// Adapters in internal/adapters/ implement these interfaces; the TUI and
// future HTTP handlers depend only on these types — never on concrete infra.
package service

import "github.com/mrboard/mrboard/internal/domain"

// MergeRequestSource is the driven port for fetching MR data.
// The TUI and any future frontend depend on this interface, not on the
// concrete GitLab client.
type MergeRequestSource interface {
	// FetchAll retrieves all open MRs from all configured sources.
	// Partial results are valid: non-nil MRs may accompany non-nil errors.
	FetchAll() ([]domain.MergeRequest, []error)

	// GetDetail fetches the description and discussion threads for a single MR.
	GetDetail(projectID, mrIID int64) (description string, threads []domain.Thread, err error)
}
