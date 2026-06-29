// Package jira provides a thin HTTP client for the Atlassian JIRA REST API.
// It has no dependency on internal/* — callers pass a Config built from their
// own config layer.
package jira

import "time"

// Config holds the connection parameters for the JIRA HTTP client.
type Config struct {
	InstanceURL string        // e.g. "https://yourco.atlassian.net"
	Email       string        // Atlassian account email for Basic Auth
	APIToken    string        // Atlassian API token (or $JIRA_TOKEN)
	Timeout     time.Duration // HTTP request timeout; 0 → no timeout
}
