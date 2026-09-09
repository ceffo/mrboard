// Package github provides a thin, unauthenticated HTTP client for the parts
// of the public GitHub REST API mrboard needs (checking its own latest
// release). It has no dependency on internal/* — callers pass a Config built
// from their own config layer.
package github

import "time"

// Config holds the connection parameters for the GitHub HTTP client.
type Config struct {
	Owner   string        // repository owner, e.g. "ceffo"
	Repo    string        // repository name, e.g. "mrboard"
	Timeout time.Duration // HTTP request timeout; 0 → defaultTimeout
}
