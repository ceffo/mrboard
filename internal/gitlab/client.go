// Package gitlab provides a thin wrapper around the xanzy/go-gitlab client
// for fetching MR data needed by mrboard.
package gitlab

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/config"
)

const perPage = 100

// Client wraps the xanzy/go-gitlab client and exposes the methods needed by
// the fetcher to retrieve raw MR data.
type Client struct {
	gl     *gl.Client
	logger *slog.Logger

	// projectArchived caches archival status keyed by project ID to avoid
	// redundant API calls when filtering MRs from user sources.
	projectArchivedCache map[int64]bool
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
	return &Client{gl: c, logger: logger, projectArchivedCache: make(map[int64]bool)}, nil
}

// ListGroupMRs returns all open merge requests for the given group ID (name or numeric ID).
// excludedAuthor, if non-empty, is applied server-side as not[author_username] to reduce payload size.
func (c *Client) ListGroupMRs(ctx context.Context, groupID, excludedAuthor string) ([]*gl.BasicMergeRequest, error) {
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
		mrs, resp, err := c.gl.MergeRequests.ListGroupMergeRequests(groupID, opts, gl.WithContext(ctx))
		if err != nil {
			elapsed := time.Since(start).Round(time.Millisecond)
			c.logger.Debug("gitlab: list group MRs error", "group", groupID, "duration", elapsed, "error", err)
			return nil, fmt.Errorf("gitlab: list group MRs %q: %w", groupID, err)
		}
		all = append(all, mrs...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	elapsed := time.Since(start).Round(time.Millisecond)
	c.logger.Debug("gitlab: list group MRs done", "group", groupID, "count", len(all), "duration", elapsed)
	return all, nil
}

// ListUserMRs returns all open merge requests authored by the given username.
func (c *Client) ListUserMRs(ctx context.Context, username string) ([]*gl.BasicMergeRequest, error) {
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
		mrs, resp, err := c.gl.MergeRequests.ListMergeRequests(opts, gl.WithContext(ctx))
		if err != nil {
			elapsed := time.Since(start).Round(time.Millisecond)
			c.logger.Debug("gitlab: list user MRs error", "username", username, "duration", elapsed, "error", err)
			return nil, fmt.Errorf("gitlab: list user MRs for %q: %w", username, err)
		}
		all = append(all, mrs...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	elapsed := time.Since(start).Round(time.Millisecond)
	c.logger.Debug("gitlab: list user MRs done", "username", username, "count", len(all), "duration", elapsed)
	return all, nil
}

// GetMRDiscussions returns all discussions (threaded notes) for an MR.
func (c *Client) GetMRDiscussions(ctx context.Context, projectID, mrIID int64) ([]*gl.Discussion, error) {
	start := time.Now()
	c.logger.Debug("gitlab: get discussions", "project", projectID, "mr", mrIID)
	var all []*gl.Discussion
	opts := &gl.ListMergeRequestDiscussionsOptions{ListOptions: gl.ListOptions{PerPage: perPage}}
	for {
		discussions, resp, err := c.gl.Discussions.ListMergeRequestDiscussions(projectID, mrIID, opts, gl.WithContext(ctx))
		if err != nil {
			elapsed := time.Since(start).Round(time.Millisecond)
			c.logger.Debug("gitlab: get discussions error", "project", projectID, "mr", mrIID, "duration", elapsed, "error", err)
			return nil, fmt.Errorf("gitlab: get discussions project=%d MR=%d: %w", projectID, mrIID, err)
		}
		all = append(all, discussions...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	elapsed := time.Since(start).Round(time.Millisecond)
	c.logger.Debug("gitlab: get discussions done",
		"project", projectID, "mr", mrIID, "count", len(all), "duration", elapsed)
	return all, nil
}

// GetMRApprovals returns the approval status (who approved, how many required) for an MR.
func (c *Client) GetMRApprovals(ctx context.Context, projectID, mrIID int64) (*gl.MergeRequestApprovals, error) {
	start := time.Now()
	c.logger.Debug("gitlab: get approvals", "project", projectID, "mr", mrIID)
	approvals, _, err := c.gl.MergeRequests.GetMergeRequestApprovals(projectID, mrIID, gl.WithContext(ctx))
	if err != nil {
		elapsed := time.Since(start).Round(time.Millisecond)
		c.logger.Debug("gitlab: get approvals error", "project", projectID, "mr", mrIID, "duration", elapsed, "error", err)
		return nil, fmt.Errorf("gitlab: get approvals project=%d MR=%d: %w", projectID, mrIID, err)
	}
	elapsed := time.Since(start).Round(time.Millisecond)
	c.logger.Debug("gitlab: get approvals done", "project", projectID, "mr", mrIID, "duration", elapsed)
	return approvals, nil
}

// GetMRDescription fetches the description (body text) of a single MR.
func (c *Client) GetMRDescription(ctx context.Context, projectID, mrIID int64) (string, error) {
	start := time.Now()
	c.logger.Debug("gitlab: get MR description", "project", projectID, "mr", mrIID)
	mr, _, err := c.gl.MergeRequests.GetMergeRequest(projectID, mrIID, nil, gl.WithContext(ctx))
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		c.logger.Debug("gitlab: get MR description error",
			"project", projectID, "mr", mrIID, "duration", elapsed, "error", err)
		return "", fmt.Errorf("gitlab: get MR description project=%d MR=%d: %w", projectID, mrIID, err)
	}
	c.logger.Debug("gitlab: get MR description done", "project", projectID, "mr", mrIID, "duration", elapsed)
	return mr.Description, nil
}

// ListNonArchivedProjectIDs returns the set of non-archived project IDs for a
// group. IncludeSubGroups is set so that nested group projects are included.
func (c *Client) ListNonArchivedProjectIDs(ctx context.Context, groupID string) (map[int64]bool, error) {
	start := time.Now()
	c.logger.Debug("gitlab: list non-archived projects", "group", groupID)
	ids := make(map[int64]bool)
	opts := &gl.ListGroupProjectsOptions{
		Archived:         gl.Ptr(false),
		IncludeSubGroups: gl.Ptr(true),
		ListOptions:      gl.ListOptions{PerPage: perPage},
	}
	for {
		projects, resp, err := c.gl.Groups.ListGroupProjects(groupID, opts, gl.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: list group projects %q: %w", groupID, err)
		}
		for _, p := range projects {
			ids[p.ID] = true
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	elapsed := time.Since(start).Round(time.Millisecond).String()
	c.logger.Debug("gitlab: list non-archived projects done", "group", groupID, "count", len(ids), "duration", elapsed)
	return ids, nil
}

// IsProjectArchived reports whether the given project is archived. Results are
// cached in the client so repeated calls for the same project ID are free.
func (c *Client) IsProjectArchived(ctx context.Context, projectID int64) (bool, error) {
	if archived, ok := c.projectArchivedCache[projectID]; ok {
		return archived, nil
	}
	project, _, err := c.gl.Projects.GetProject(projectID, nil, gl.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("gitlab: get project %d: %w", projectID, err)
	}
	c.projectArchivedCache[projectID] = project.Archived
	return project.Archived, nil
}
