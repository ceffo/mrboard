// Package gitlabadpt implements mrsvc.MergeRequestSource using the GitLab REST API.
package gitlabadpt

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	ilog "github.com/ceffo/mrboard/internal/log"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// enrichConcurrency caps the number of MRs being enriched simultaneously.
const enrichConcurrency = 10

// Config holds the adapter-specific configuration.
type Config struct {
	RequiredApprovals int
	Sources           []mrsvc.Source
	ExcludedAuthors   []string
}

// GitLabAdapter implements mrsvc.MergeRequestSource using a live GitLab client.
type GitLabAdapter struct {
	client *pkggitlab.Client
	cfg    Config
}

// New constructs a GitLabAdapter.
func New(client *pkggitlab.Client, cfg Config) *GitLabAdapter {
	return &GitLabAdapter{client: client, cfg: cfg}
}

// FetchAll implements mrsvc.MergeRequestSource.
func (a *GitLabAdapter) FetchAll(ctx context.Context) ([]domain.MergeRequest, []error) {
	logger := ilog.FromContext(ctx)
	fetchStart := time.Now()
	logger.Info("gitlab: fetch start", "sources", len(a.cfg.Sources), "excluded_authors", a.cfg.ExcludedAuthors)

	var primaryExclusion string
	if len(a.cfg.ExcludedAuthors) > 0 {
		primaryExclusion = a.cfg.ExcludedAuthors[0]
	}

	listStart := time.Now()
	raw, errs := a.listAllMRs(ctx, primaryExclusion)
	logger.Info("gitlab: source listing done",
		"raw", len(raw), "source_errors", len(errs),
		"duration", time.Since(listStart).Round(time.Millisecond))

	excluded := make(map[string]bool, len(a.cfg.ExcludedAuthors))
	for _, u := range a.cfg.ExcludedAuthors {
		excluded[u] = true
	}

	seen := make(map[mrKey]bool, len(raw))
	unique := make([]*gl.BasicMergeRequest, 0, len(raw))
	for _, mr := range raw {
		authorUsername := ""
		if mr.Author != nil {
			authorUsername = mr.Author.Username
		}
		logger.Debug("gitlab: raw MR", "iid", mr.IID, "title", mr.Title, "author", authorUsername)
		if authorUsername != "" && excluded[authorUsername] {
			logger.Debug("gitlab: excluding MR", "iid", mr.IID, "author", authorUsername)
			continue
		}
		k := mrKey{projectID: mr.ProjectID, iid: mr.IID}
		if seen[k] {
			logger.Debug("gitlab: dedup drop", "iid", mr.IID, "project", mr.ProjectID)
			continue
		}
		seen[k] = true
		unique = append(unique, mr)
	}
	logger.Info("gitlab: dedup summary", "raw", len(raw), "unique", len(unique))

	type result struct {
		mr  domain.MergeRequest
		err error
	}

	enrichStart := time.Now()
	results := make([]result, len(unique))
	sem := make(chan struct{}, enrichConcurrency)
	var wg sync.WaitGroup

	for i, mr := range unique {
		i, mr := i, mr
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			domainMR, err := a.enrichMR(ctx, mr)
			results[i] = result{mr: domainMR, err: err}
		}()
	}
	wg.Wait()

	var mrs []domain.MergeRequest
	var enrichErrs int
	for _, r := range results {
		if r.err != nil {
			logger.Error("gitlab: enrich MR failed", "error", r.err)
			errs = append(errs, r.err)
			enrichErrs++
			continue
		}
		mrs = append(mrs, r.mr)
	}

	logger.Info("gitlab: enrichment done",
		"mrs", len(mrs), "enrich_errors", enrichErrs,
		"duration", time.Since(enrichStart).Round(time.Millisecond))
	logger.Info("gitlab: fetch done",
		"mrs", len(mrs), "errors", len(errs),
		"total_duration", time.Since(fetchStart).Round(time.Millisecond))
	return mrs, errs
}

// GetDetail implements mrsvc.MergeRequestSource.
func (a *GitLabAdapter) GetDetail(ctx context.Context, projectID, mrIID int64) (string, []domain.Thread, error) {
	desc, err := a.client.GetMRDescription(ctx, projectID, mrIID)
	if err != nil {
		return "", nil, fmt.Errorf("get detail project=%d MR=%d description: %w", projectID, mrIID, err)
	}
	discussions, err := a.client.GetMRDiscussions(ctx, projectID, mrIID)
	if err != nil {
		return desc, nil, fmt.Errorf("get detail project=%d MR=%d discussions: %w", projectID, mrIID, err)
	}
	threads := MapDiscussionsToThreads(discussions)
	return desc, threads, nil
}

// mrKey uniquely identifies an MR across all sources.
type mrKey struct {
	projectID int64
	iid       int64
}

// sourceResult is the output of fetching a single source ID.
type sourceResult struct {
	mrs  []*gl.BasicMergeRequest
	errs []error
}

// listAllMRs fetches every source ID in parallel and merges the results.
func (a *GitLabAdapter) listAllMRs(ctx context.Context, primaryExclusion string) ([]*gl.BasicMergeRequest, []error) {
	logger := ilog.FromContext(ctx)

	type task struct {
		srcType mrsvc.SourceType
		id      string
	}
	var tasks []task
	for _, src := range a.cfg.Sources {
		for _, id := range src.IDs {
			tasks = append(tasks, task{srcType: src.Type, id: id})
		}
	}

	results := make([]sourceResult, len(tasks))
	var wg sync.WaitGroup
	for i, t := range tasks {
		i, t := i, t
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = a.fetchSourceID(ctx, t.srcType, t.id, primaryExclusion, logger)
		}()
	}
	wg.Wait()

	var all []*gl.BasicMergeRequest
	var errs []error
	for _, r := range results {
		all = append(all, r.mrs...)
		errs = append(errs, r.errs...)
	}
	return all, errs
}

func (a *GitLabAdapter) fetchSourceID(
	ctx context.Context,
	srcType mrsvc.SourceType,
	id, primaryExclusion string,
	logger *slog.Logger,
) sourceResult {
	start := time.Now()
	switch srcType {
	case mrsvc.SourceTypeGroup:
		allowedProjects, err := a.client.ListNonArchivedProjectIDs(ctx, id)
		if err != nil {
			logger.Error("gitlab: list group projects failed", "group", id, "error", err)
			return sourceResult{errs: []error{fmt.Errorf("source group=%q: %w", id, err)}}
		}
		mrs, err := a.client.ListGroupMRs(ctx, id, primaryExclusion)
		if err != nil {
			logger.Error("gitlab: list group MRs failed", "group", id, "error", err)
			return sourceResult{errs: []error{fmt.Errorf("source group=%q: %w", id, err)}}
		}
		var active []*gl.BasicMergeRequest
		for _, mr := range mrs {
			if allowedProjects[mr.ProjectID] {
				active = append(active, mr)
			} else {
				logger.Debug("gitlab: skipping MR from archived project", "iid", mr.IID, "project", mr.ProjectID)
			}
		}
		logger.Info("gitlab: group source fetched",
			"group", id, "total", len(mrs), "active", len(active),
			"duration", time.Since(start).Round(time.Millisecond))
		return sourceResult{mrs: active}

	case mrsvc.SourceTypeUser:
		mrs, err := a.client.ListUserMRs(ctx, id)
		if err != nil {
			logger.Error("gitlab: list user MRs failed", "username", id, "error", err)
			return sourceResult{errs: []error{fmt.Errorf("source user=%q: %w", id, err)}}
		}
		var active []*gl.BasicMergeRequest
		var errs []error
		for _, mr := range mrs {
			archived, err := a.client.IsProjectArchived(ctx, mr.ProjectID)
			if err != nil {
				logger.Error("gitlab: check project archived failed",
					"username", id, "mr", mr.IID, "project", mr.ProjectID, "error", err)
				errs = append(errs, fmt.Errorf("source user=%q MR=%d: %w", id, mr.IID, err))
				continue
			}
			if !archived {
				active = append(active, mr)
			} else {
				logger.Debug("gitlab: skipping MR from archived project", "iid", mr.IID, "project", mr.ProjectID)
			}
		}
		logger.Info("gitlab: user source fetched",
			"username", id, "total", len(mrs), "active", len(active),
			"duration", time.Since(start).Round(time.Millisecond))
		return sourceResult{mrs: active, errs: errs}

	default:
		logger.Error("gitlab: unknown source type", "type", srcType)
		return sourceResult{errs: []error{fmt.Errorf("source: unknown type %q", srcType)}}
	}
}

func (a *GitLabAdapter) enrichMR(ctx context.Context, mr *gl.BasicMergeRequest) (domain.MergeRequest, error) {
	if mr.Draft {
		discussions, err := a.client.GetMRDiscussions(ctx, mr.ProjectID, mr.IID)
		if err != nil {
			return domain.MergeRequest{}, fmt.Errorf("enrichMR project=%d MR=%d discussions: %w", mr.ProjectID, mr.IID, err)
		}
		emptyApprovals := &gl.MergeRequestApprovals{
			ApprovalsRequired: int64(a.cfg.RequiredApprovals),
			ApprovalsLeft:     int64(a.cfg.RequiredApprovals),
		}
		return MapMR(mr, discussions, emptyApprovals, a.cfg.RequiredApprovals), nil
	}

	type discResult struct {
		discussions []*gl.Discussion
		err         error
	}
	type apprResult struct {
		approvals *gl.MergeRequestApprovals
		err       error
	}

	discCh := make(chan discResult, 1)
	apprCh := make(chan apprResult, 1)

	go func() {
		d, err := a.client.GetMRDiscussions(ctx, mr.ProjectID, mr.IID)
		discCh <- discResult{d, err}
	}()
	go func() {
		a, err := a.client.GetMRApprovals(ctx, mr.ProjectID, mr.IID)
		apprCh <- apprResult{a, err}
	}()

	dr := <-discCh
	ar := <-apprCh

	if dr.err != nil {
		return domain.MergeRequest{}, fmt.Errorf("enrichMR project=%d MR=%d discussions: %w", mr.ProjectID, mr.IID, dr.err)
	}
	if ar.err != nil {
		return domain.MergeRequest{}, fmt.Errorf("enrichMR project=%d MR=%d approvals: %w", mr.ProjectID, mr.IID, ar.err)
	}

	return MapMR(mr, dr.discussions, ar.approvals, a.cfg.RequiredApprovals), nil
}
