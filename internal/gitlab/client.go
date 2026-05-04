package gitlab

import (
	"fmt"

	"github.com/mrboard/mrboard/internal/config"
	gl "github.com/xanzy/go-gitlab"
)

// Client wraps the xanzy/go-gitlab client and exposes the methods needed by
// the fetcher to retrieve raw MR data.
type Client struct {
	gl *gl.Client
}

// NewClient creates an authenticated GitLab client from the app config.
func NewClient(cfg *config.Config) (*Client, error) {
	c, err := gl.NewClient(cfg.GitLab.Token, gl.WithBaseURL(cfg.GitLab.URL))
	if err != nil {
		return nil, fmt.Errorf("gitlab: create client: %w", err)
	}
	return &Client{gl: c}, nil
}

// ListGroupMRs returns all open merge requests for the given group ID (name or numeric ID).
func (c *Client) ListGroupMRs(groupID string) ([]*gl.MergeRequest, error) {
	var all []*gl.MergeRequest
	opts := &gl.ListGroupMergeRequestsOptions{
		State:       gl.Ptr("opened"),
		ListOptions: gl.ListOptions{PerPage: 100},
	}
	for {
		mrs, resp, err := c.gl.MergeRequests.ListGroupMergeRequests(groupID, opts)
		if err != nil {
			return nil, fmt.Errorf("gitlab: list group MRs for %q: %w", groupID, err)
		}
		all = append(all, mrs...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ListUserMRs returns all open merge requests authored by the given username.
func (c *Client) ListUserMRs(username string) ([]*gl.MergeRequest, error) {
	var all []*gl.MergeRequest
	opts := &gl.ListMergeRequestsOptions{
		AuthorUsername: gl.Ptr(username),
		State:          gl.Ptr("opened"),
		ListOptions:    gl.ListOptions{PerPage: 100},
	}
	for {
		mrs, resp, err := c.gl.MergeRequests.ListMergeRequests(opts)
		if err != nil {
			return nil, fmt.Errorf("gitlab: list user MRs for %q: %w", username, err)
		}
		all = append(all, mrs...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// GetMRDiscussions returns all discussions (threaded notes) for an MR.
func (c *Client) GetMRDiscussions(projectID, mrIID int) ([]*gl.Discussion, error) {
	var all []*gl.Discussion
	opts := &gl.ListMergeRequestDiscussionsOptions{PerPage: 100}
	for {
		discussions, resp, err := c.gl.Discussions.ListMergeRequestDiscussions(projectID, mrIID, opts)
		if err != nil {
			return nil, fmt.Errorf("gitlab: get discussions project=%d MR=%d: %w", projectID, mrIID, err)
		}
		all = append(all, discussions...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// GetMRApprovals returns the approval status (who approved, how many required) for an MR.
func (c *Client) GetMRApprovals(projectID, mrIID int) (*gl.MergeRequestApprovals, error) {
	approvals, _, err := c.gl.MergeRequests.GetMergeRequestApprovals(projectID, mrIID)
	if err != nil {
		return nil, fmt.Errorf("gitlab: get approvals project=%d MR=%d: %w", projectID, mrIID, err)
	}
	return approvals, nil
}
