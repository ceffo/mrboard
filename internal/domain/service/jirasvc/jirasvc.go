// Package jirasvc owns the JiraEnricher service port.
// Adapters in internal/adapters/ implement JiraEnricher; the TUI and
// future handlers depend only on this package — never on concrete infra.
package jirasvc

import "context"

// JiraEnricher is the driven port for fetching JIRA issue metadata.
type JiraEnricher interface {
	// GetIssueType returns the issue type name (e.g. "Bug", "Story") for the
	// given issue key. Returns ("", nil) when the issue is not found.
	GetIssueType(ctx context.Context, issueKey string) (string, error)

	// GetActiveSprintIssueKeys returns all issue keys belonging to the active
	// sprint for the given JIRA board ID. Returns nil when no active sprint exists.
	GetActiveSprintIssueKeys(ctx context.Context, boardID int) ([]string, error)
}

// JiraLinker is the driven port for writing JIRA remote issue links.
type JiraLinker interface {
	// UpsertRemoteLink writes a remote link from the JIRA issue identified by
	// issueKey to the resource at mrURL. It is idempotent: the JIRA API is only
	// called when the link title differs from the last-written value (or when
	// no link has been written yet). globalID must be stable across fetches.
	UpsertRemoteLink(ctx context.Context, issueKey, globalID, mrTitle, mrURL string) error
}
