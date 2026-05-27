package gitlab

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	ilog "github.com/ceffo/mrboard/internal/log"
)

const perPage = 100

// Client wraps the go-gitlab API client and exposes the methods needed to
// retrieve raw MR data.
type Client struct {
	gl         *gl.Client
	logger     *slog.Logger
	token      string
	apiURL     string
	httpClient *http.Client

	// projectArchivedCache caches archival status keyed by project ID to avoid
	// redundant API calls when filtering MRs from user sources.
	// cacheMu guards concurrent access when sources are fetched in parallel.
	cacheMu              sync.RWMutex
	projectArchivedCache map[int64]bool
}

// NewClient creates an authenticated GitLab client.
// logger is used for API call telemetry; pass slog.New(slog.DiscardHandler) for silence.
func NewClient(cfg Config, logger *slog.Logger) (*Client, error) {
	httpClient := &http.Client{Timeout: cfg.Timeout}
	c, err := gl.NewClient(cfg.Token,
		gl.WithBaseURL(cfg.URL),
		gl.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("gitlab: create client: %w", err)
	}
	return &Client{
		gl:                   c,
		logger:               logger,
		token:                cfg.Token,
		apiURL:               cfg.URL,
		httpClient:           httpClient,
		projectArchivedCache: make(map[int64]bool),
	}, nil
}

// ListGroupMRs returns all open merge requests for the given group ID.
// excludedAuthor, if non-empty, is applied server-side as not[author_username].
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
			elapsed := time.Since(start)
			c.logger.Error("gitlab: list group MRs error", "group", groupID, "duration", ilog.FmtDur(elapsed), "error", err)
			return nil, fmt.Errorf("gitlab: list group MRs %q: %w", groupID, err)
		}
		all = append(all, mrs...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	elapsed := time.Since(start)
	c.logger.Debug("gitlab: list group MRs done", "group", groupID, "count", len(all), "duration", ilog.FmtDur(elapsed))
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
			elapsed := time.Since(start)
			c.logger.Error("gitlab: list user MRs error", "username", username, "duration", ilog.FmtDur(elapsed), "error", err)
			return nil, fmt.Errorf("gitlab: list user MRs for %q: %w", username, err)
		}
		all = append(all, mrs...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	elapsed := time.Since(start)
	c.logger.Debug("gitlab: list user MRs done", "username", username, "count", len(all), "duration", ilog.FmtDur(elapsed))
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
			elapsed := time.Since(start)
			c.logger.Error("gitlab: get discussions error",
				"project", projectID, "mr", mrIID, "duration", ilog.FmtDur(elapsed), "error", err)
			return nil, fmt.Errorf("gitlab: get discussions project=%d MR=%d: %w", projectID, mrIID, err)
		}
		all = append(all, discussions...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	elapsed := time.Since(start)
	c.logger.Debug("gitlab: get discussions done",
		"project", projectID, "mr", mrIID, "count", len(all), "duration", ilog.FmtDur(elapsed))
	return all, nil
}

// MRApprovalRulePayload holds the fields for creating or updating an MR approval rule.
type MRApprovalRulePayload struct {
	Name              string
	ApprovalsRequired int
	UserIDs           []int64
}

// GetMRApprovalRules returns the approval rules for an MR.
func (c *Client) GetMRApprovalRules(
	ctx context.Context, projectID, mrIID int64,
) ([]*gl.MergeRequestApprovalRule, error) {
	start := time.Now()
	c.logger.Debug("gitlab: get approval rules", "project", projectID, "mr", mrIID)
	rules, _, err := c.gl.MergeRequestApprovals.GetApprovalRules(projectID, mrIID, gl.WithContext(ctx))
	elapsed := time.Since(start)
	if err != nil {
		c.logger.Error("gitlab: get approval rules error",
			"project", projectID, "mr", mrIID, "duration", ilog.FmtDur(elapsed), "error", err)
		return nil, fmt.Errorf("gitlab: get approval rules project=%d MR=%d: %w", projectID, mrIID, err)
	}
	c.logger.Debug("gitlab: get approval rules done",
		"project", projectID, "mr", mrIID, "count", len(rules), "duration", ilog.FmtDur(elapsed))
	return rules, nil
}

// GetProjectMembers returns all project members (inherited) with access level >= minAccessLevel.
func (c *Client) GetProjectMembers(
	ctx context.Context, projectID int64, minAccessLevel int,
) ([]*gl.ProjectMember, error) {
	start := time.Now()
	c.logger.Debug("gitlab: get project members", "project", projectID, "min_level", minAccessLevel)
	var all []*gl.ProjectMember
	opts := &gl.ListProjectMembersOptions{ListOptions: gl.ListOptions{PerPage: perPage}}
	for {
		members, resp, err := c.gl.ProjectMembers.ListAllProjectMembers(projectID, opts, gl.WithContext(ctx))
		if err != nil {
			elapsed := time.Since(start)
			c.logger.Error("gitlab: get project members error",
				"project", projectID, "duration", ilog.FmtDur(elapsed), "error", err)
			return nil, fmt.Errorf("gitlab: get project members project=%d: %w", projectID, err)
		}
		for _, m := range members {
			if int(m.AccessLevel) >= minAccessLevel {
				all = append(all, m)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	elapsed := time.Since(start)
	c.logger.Debug("gitlab: get project members done",
		"project", projectID, "count", len(all), "duration", ilog.FmtDur(elapsed))
	return all, nil
}

// CreateMRApprovalRule creates a new approval rule on an MR.
func (c *Client) CreateMRApprovalRule(
	ctx context.Context, projectID, mrIID int64, payload MRApprovalRulePayload,
) (*gl.MergeRequestApprovalRule, error) {
	start := time.Now()
	c.logger.Debug("gitlab: create approval rule",
		"project", projectID, "mr", mrIID, "name", payload.Name)
	approvsReq := int64(payload.ApprovalsRequired)
	rule, _, err := c.gl.MergeRequestApprovals.CreateApprovalRule(projectID, mrIID,
		&gl.CreateMergeRequestApprovalRuleOptions{
			Name:              gl.Ptr(payload.Name),
			ApprovalsRequired: &approvsReq,
			UserIDs:           &payload.UserIDs,
		}, gl.WithContext(ctx))
	elapsed := time.Since(start)
	if err != nil {
		c.logger.Error("gitlab: create approval rule error",
			"project", projectID, "mr", mrIID, "duration", ilog.FmtDur(elapsed), "error", err)
		return nil, fmt.Errorf("gitlab: create approval rule project=%d MR=%d: %w", projectID, mrIID, err)
	}
	c.logger.Debug("gitlab: create approval rule done",
		"project", projectID, "mr", mrIID, "rule_id", rule.ID, "duration", ilog.FmtDur(elapsed))
	return rule, nil
}

// UpdateMRApprovalRule updates an existing approval rule on an MR.
func (c *Client) UpdateMRApprovalRule(
	ctx context.Context, projectID, mrIID, ruleID int64, payload MRApprovalRulePayload,
) error {
	start := time.Now()
	c.logger.Debug("gitlab: update approval rule",
		"project", projectID, "mr", mrIID, "rule_id", ruleID)
	approvsReq := int64(payload.ApprovalsRequired)
	_, _, err := c.gl.MergeRequestApprovals.UpdateApprovalRule(projectID, mrIID, ruleID,
		&gl.UpdateMergeRequestApprovalRuleOptions{
			Name:              gl.Ptr(payload.Name),
			ApprovalsRequired: &approvsReq,
			UserIDs:           &payload.UserIDs,
		}, gl.WithContext(ctx))
	elapsed := time.Since(start)
	if err != nil {
		c.logger.Error("gitlab: update approval rule error",
			"project", projectID, "mr", mrIID, "duration", ilog.FmtDur(elapsed), "error", err)
		return fmt.Errorf("gitlab: update approval rule project=%d MR=%d rule=%d: %w",
			projectID, mrIID, ruleID, err)
	}
	c.logger.Debug("gitlab: update approval rule done",
		"project", projectID, "mr", mrIID, "duration", ilog.FmtDur(elapsed))
	return nil
}

// GetMRApprovals returns the approval status for an MR.
func (c *Client) GetMRApprovals(ctx context.Context, projectID, mrIID int64) (*gl.MergeRequestApprovals, error) {
	start := time.Now()
	c.logger.Debug("gitlab: get approvals", "project", projectID, "mr", mrIID)
	approvals, _, err := c.gl.MergeRequests.GetMergeRequestApprovals(projectID, mrIID, gl.WithContext(ctx))
	if err != nil {
		elapsed := time.Since(start)
		c.logger.Error("gitlab: get approvals error",
			"project", projectID, "mr", mrIID, "duration", ilog.FmtDur(elapsed), "error", err)
		return nil, fmt.Errorf("gitlab: get approvals project=%d MR=%d: %w", projectID, mrIID, err)
	}
	elapsed := time.Since(start)
	c.logger.Debug("gitlab: get approvals done", "project", projectID, "mr", mrIID, "duration", ilog.FmtDur(elapsed))
	return approvals, nil
}

// GetMR returns a single MR as a BasicMergeRequest, fetched by project ID and IID.
func (c *Client) GetMR(ctx context.Context, projectID, mrIID int64) (*gl.BasicMergeRequest, error) {
	iids := []int64{mrIID}
	mrs, _, err := c.gl.MergeRequests.ListProjectMergeRequests(projectID, &gl.ListProjectMergeRequestsOptions{
		IIDs: &iids,
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("gitlab: get MR project=%d MR=%d: %w", projectID, mrIID, err)
	}
	if len(mrs) == 0 {
		return nil, fmt.Errorf("gitlab: MR !%d not found in project %d", mrIID, projectID)
	}
	return mrs[0], nil
}

// GetMRDescription fetches the description of a single MR.
func (c *Client) GetMRDescription(ctx context.Context, projectID, mrIID int64) (string, error) {
	start := time.Now()
	c.logger.Debug("gitlab: get MR description", "project", projectID, "mr", mrIID)
	mr, _, err := c.gl.MergeRequests.GetMergeRequest(projectID, mrIID, nil, gl.WithContext(ctx))
	elapsed := time.Since(start)
	if err != nil {
		c.logger.Error("gitlab: get MR description error",
			"project", projectID, "mr", mrIID, "duration", ilog.FmtDur(elapsed), "error", err)
		return "", fmt.Errorf("gitlab: get MR description project=%d MR=%d: %w", projectID, mrIID, err)
	}
	c.logger.Debug("gitlab: get MR description done", "project", projectID, "mr", mrIID, "duration", ilog.FmtDur(elapsed))
	return mr.Description, nil
}

// ListNonArchivedProjectIDs returns the set of non-archived project IDs for a group.
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
	elapsed := ilog.FmtDur(time.Since(start))
	c.logger.Debug("gitlab: list non-archived projects done", "group", groupID, "count", len(ids), "duration", elapsed)
	return ids, nil
}

// GetMRDiffs fetches all file diffs for an MR, requesting unified diff format.
func (c *Client) GetMRDiffs(ctx context.Context, projectID, mrIID int64) ([]*gl.MergeRequestDiff, error) {
	start := time.Now()
	c.logger.Debug("gitlab: get MR diffs", "project_id", projectID, "mr_iid", mrIID)
	unidiff := true
	diffs, _, err := c.gl.MergeRequests.ListMergeRequestDiffs(projectID, mrIID,
		&gl.ListMergeRequestDiffsOptions{Unidiff: &unidiff},
		gl.WithContext(ctx))
	if err != nil {
		c.logger.Error("gitlab: get MR diffs error", "project_id", projectID, "mr_iid", mrIID,
			"duration", ilog.FmtDur(time.Since(start)), "error", err)
		return nil, fmt.Errorf("gitlab: get MR diffs project=%d MR=%d: %w", projectID, mrIID, err)
	}
	c.logger.Info("gitlab: got MR diffs", "project_id", projectID, "mr_iid", mrIID,
		"files", len(diffs), "duration", ilog.FmtDur(time.Since(start)))
	return diffs, nil
}

// IsProjectArchived reports whether the given project is archived. Results are cached.
// Safe for concurrent use.
func (c *Client) IsProjectArchived(ctx context.Context, projectID int64) (bool, error) {
	c.cacheMu.RLock()
	archived, ok := c.projectArchivedCache[projectID]
	c.cacheMu.RUnlock()
	if ok {
		return archived, nil
	}

	project, _, err := c.gl.Projects.GetProject(projectID, nil, gl.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("gitlab: get project %d: %w", projectID, err)
	}

	c.cacheMu.Lock()
	c.projectArchivedCache[projectID] = project.Archived
	c.cacheMu.Unlock()
	return project.Archived, nil
}
