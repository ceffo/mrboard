package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultTimeout = 10 * time.Second
	defaultBaseURL = "https://api.github.com"
)

// Release is a minimal representation of a GitHub release.
type Release struct {
	TagName string // e.g. "v0.12.0"
	HTMLURL string // release page URL
}

// Client is an unauthenticated GitHub REST API client, limited to the single
// endpoint this package needs.
type Client struct {
	owner, repo string
	// baseURL defaults to defaultBaseURL; tests override it to point at an
	// httptest server.
	baseURL string
	http    *http.Client
}

// NewClient creates a GitHub client for the given repository.
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &Client{
		owner:   cfg.Owner,
		repo:    cfg.Repo,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

// GetLatestRelease fetches the repository's latest published release.
// Returns (nil, nil) when the repository has no releases (HTTP 404).
func (c *Client) GetLatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, c.owner, c.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github: get latest release for %s/%s: HTTP %d", c.owner, c.repo, resp.StatusCode)
	}

	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("github: decode latest release for %s/%s: %w", c.owner, c.repo, err)
	}
	return &Release{TagName: body.TagName, HTMLURL: body.HTMLURL}, nil
}
