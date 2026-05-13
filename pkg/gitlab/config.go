// Package gitlab provides a thin HTTP client wrapper around the GitLab REST API.
// It has no dependency on internal/* — callers pass a Config built from their own
// config layer.
package gitlab

import "time"

// Config holds the connection parameters for the GitLab HTTP client.
type Config struct {
	URL     string
	Token   string
	Timeout time.Duration
}
