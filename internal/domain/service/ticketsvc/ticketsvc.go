// Package ticketsvc owns the issue-tracker service ports. It is vendor-neutral
// by design: adapters in internal/adapters/ (currently jiraadpt, backed by JIRA)
// implement TicketEnricher and TicketLinker; the TUI and future handlers depend
// only on this package — never on a concrete vendor.
package ticketsvc

import "context"

// TicketEnricher is the driven port for fetching issue-tracker metadata.
type TicketEnricher interface {
	// GetIssueType returns the issue type name (e.g. "Bug", "Story") for the
	// given issue key. Returns ("", nil) when the issue is not found.
	GetIssueType(ctx context.Context, issueKey string) (string, error)

	// GetActiveSprintIssueKeys returns all issue keys belonging to the active
	// sprint for the given board ID. Returns nil when no active sprint exists.
	// forceRefresh bypasses any cached result, so callers get a live read.
	GetActiveSprintIssueKeys(ctx context.Context, boardID int, forceRefresh bool) ([]string, error)
}

// TicketLinker is the driven port for writing remote issue links.
type TicketLinker interface {
	// UpsertRemoteLink writes a remote link from the issue identified by
	// issueKey to the resource at mrURL. It is idempotent: the tracker's API is
	// only called when the link title differs from the last-written value (or
	// when no link has been written yet). globalID must be stable across fetches.
	UpsertRemoteLink(ctx context.Context, issueKey, globalID, mrTitle, mrURL string) error
}
