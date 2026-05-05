// Package gitlab provides a thin wrapper around the xanzy/go-gitlab client
// for fetching MR data needed by mrboard.
package gitlab

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mrboard/mrboard/internal/config"
	gl "gitlab.com/gitlab-org/api/client-go"
)

const perPage = 100

// Client wraps the xanzy/go-gitlab client and exposes the methods needed by
// the fetcher to retrieve raw MR data.
type Client struct {
	gl     *gl.Client
	logger *slog.Logger
}

// NewClient creates an authenticated GitLab client from the app config.
// timeout controls the HTTP request deadline for every API call; pass 0 for no timeout.
// logger is used to record API call telemetry; pass slog.New(slog.DiscardHandler) for silence.
func NewClient(cfg *config.Config, timeout time.Duration, logger *slog.Logger) (*Client, error) {
	httpClient := &http.Client{Timeout: timeout}
	c, err := gl.NewClient(cfg.GitLab.Token,
		gl.WithBaseURL(cfg.GitLab.URL),
		gl.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("gitlab: create client: %w", err)
	}
	return &Client{gl: c, logger: logger}, nil
}

// ListGroupMRs returns all open merge requests for the given group ID (name or numeric ID).
// excludedAuthor, if non-empty, is applied server-side as not[author_username] to reduce payload size.
func (c *Client) ListGroupMRs(groupID, excludedAuthor string) ([]*gl.BasicMergeRequest, error) {
	start := time.Now()
	c.logger.Debug("gitlab: list group MRs", "group", groupID)
	var all []*gl.BasicMergeRequest
	opts := &gl.ListGroupMergeRequestsOptions{
		State:       gl.Ptr("opened"),
		ListOptions: gl.ListOptions{PerPage: perPage},
	}
	if excludedAuthor != "" {
		opts.NotAuthorUsername = gl.Ptr(excludedAuthor)
	}
	for {
		mrs, resp, err := c.gl.MergeRequests.ListGroupMergeRequests(groupID, opts)
		if err != nil {
			c.logger.Debug("gitlab: list group MRs error", "group", groupID, "duration", time.Since(start).Round(time.Millisecond).String(), "error", err)
			return nil, fmt.Errorf("gitlab: list group MRs %q: %w", groupID, err)
		}
		all = append(all, mrs...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	c.logger.Debug("gitlab: list group MRs done", "group", groupID, "count", len(all), "duration", time.Since(start).Round(time.Millisecond).String())
	return all, nil
}

// ListUserMRs returns all open merge requests authored by the given username.
func (c *Client) ListUserMRs(username string) ([]*gl.BasicMergeRequest, error) {
	start := time.Now()
	c.logger.Debug("gitlab: list user MRs", "username", username)
	var all []*gl.BasicMergeRequest
	opts := &gl.ListMergeRequestsOptions{
		AuthorUsername: gl.Ptr(username),
		State:          gl.Ptr("opened"),
		Scope:          gl.Ptr("all"),
		ListOptions:    gl.ListOptions{PerPage: perPage},
	}
	for {
		mrs, resp, err := c.gl.MergeRequests.ListMergeRequests(opts)
		if err != nil {
			c.logger.Debug("gitlab: list user MRs error", "username", username, "duration", time.Since(start).Round(time.Millisecond).String(), "error", err)
			return nil, fmt.Errorf("gitlab: list user MRs for %q: %w", username, err)
		}
		all = append(all, mrs...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	c.logger.Debug("gitlab: list user MRs done", "username", username, "count", len(all), "duration", time.Since(start).Round(time.Millisecond).String())
	return all, nil
}

// GetMRDiscussions returns all discussions (threaded notes) for an MR.
func (c *Client) GetMRDiscussions(projectID, mrIID int64) ([]*gl.Discussion, error) {
	start := time.Now()
	c.logger.Debug("gitlab: get discussions", "project", projectID, "mr", mrIID)
	var all []*gl.Discussion
	opts := &gl.ListMergeRequestDiscussionsOptions{ListOptions: gl.ListOptions{PerPage: perPage}}
	for {
		discussions, resp, err := c.gl.Discussions.ListMergeRequestDiscussions(projectID, mrIID, opts)
		if err != nil {
			c.logger.Debug("gitlab: get discussions error", "project", projectID, "mr", mrIID, "duration", time.Since(start).Round(time.Millisecond).String(), "error", err)
			return nil, fmt.Errorf("gitlab: get discussions project=%d MR=%d: %w", projectID, mrIID, err)
		}
		all = append(all, discussions...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	c.logger.Debug("gitlab: get discussions done", "project", projectID, "mr", mrIID, "count", len(all), "duration", time.Since(start).Round(time.Millisecond).String())
	return all, nil
}

// GetMRApprovals returns the approval status (who approved, how many required) for an MR.
func (c *Client) GetMRApprovals(projectID, mrIID int64) (*gl.MergeRequestApprovals, error) {
	start := time.Now()
	c.logger.Debug("gitlab: get approvals", "project", projectID, "mr", mrIID)
	approvals, _, err := c.gl.MergeRequests.GetMergeRequestApprovals(projectID, mrIID)
	if err != nil {
		c.logger.Debug("gitlab: get approvals error", "project", projectID, "mr", mrIID, "duration", time.Since(start).Round(time.Millisecond).String(), "error", err)
		return nil, fmt.Errorf("gitlab: get approvals project=%d MR=%d: %w", projectID, mrIID, err)
	}
	c.logger.Debug("gitlab: get approvals done", "project", projectID, "mr", mrIID, "duration", time.Since(start).Round(time.Millisecond).String())
	return approvals, nil
}
