package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Issue is a minimal representation of a JIRA issue.
type Issue struct {
	Key  string
	Type string // issue type name, e.g. "Bug", "Story", "Task"
}

// Sprint is a minimal representation of a JIRA Agile sprint.
type Sprint struct {
	ID   int
	Name string
}

const defaultTimeout = 30 * time.Second

// Client is an authenticated JIRA REST API client.
type Client struct {
	instanceURL string
	authHeader  string // pre-encoded "Basic <base64(email:token)>"
	http        *http.Client
}

// NewClient creates an authenticated JIRA client.
func NewClient(cfg Config) *Client {
	creds := base64.StdEncoding.EncodeToString([]byte(cfg.Email + ":" + cfg.APIToken))
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &Client{
		instanceURL: strings.TrimRight(cfg.InstanceURL, "/"),
		authHeader:  "Basic " + creds,
		http:        &http.Client{Timeout: timeout},
	}
}

// GetIssue fetches the issue type for the given issue key (e.g. "OD-3345").
func (c *Client) GetIssue(ctx context.Context, issueKey string) (*Issue, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=issuetype", c.instanceURL, issueKey)
	var body struct {
		Key    string `json:"key"`
		Fields struct {
			IssueType struct {
				Name string `json:"name"`
			} `json:"issuetype"`
		} `json:"fields"`
	}
	if err := c.get(ctx, url, &body); err != nil {
		return nil, fmt.Errorf("jira: get issue %q: %w", issueKey, err)
	}
	return &Issue{Key: body.Key, Type: body.Fields.IssueType.Name}, nil
}

// GetActiveSprint returns the active sprint for the given board ID.
// Returns (nil, nil) when no active sprint exists.
func (c *Client) GetActiveSprint(ctx context.Context, boardID int) (*Sprint, error) {
	url := fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint?state=active", c.instanceURL, boardID)
	var body struct {
		Values []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"values"`
	}
	if err := c.get(ctx, url, &body); err != nil {
		return nil, fmt.Errorf("jira: get active sprint for board %d: %w", boardID, err)
	}
	if len(body.Values) == 0 {
		return nil, nil
	}
	s := body.Values[0]
	return &Sprint{ID: s.ID, Name: s.Name}, nil
}

// get performs an authenticated GET request and JSON-decodes the response body
// into dest. Returns an error on non-2xx status codes.
func (c *Client) get(ctx context.Context, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}
