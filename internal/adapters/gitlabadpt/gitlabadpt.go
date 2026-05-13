// Package gitlabadpt implements mrsvc.MergeRequestSource using the GitLab REST API.
package gitlabadpt

import (
	"context"
	"fmt"
	"sync"

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
	logger.Debug("gitlab: fetch start", "excluded_authors", a.cfg.ExcludedAuthors)

	var primaryExclusion string
	if len(a.cfg.ExcludedAuthors) > 0 {
		primaryExclusion = a.cfg.ExcludedAuthors[0]
	}

	raw, errs := a.listAllMRs(ctx, primaryExclusion)

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
	logger.Debug("gitlab: dedup summary", "raw", len(raw), "unique", len(unique), "dropped", len(raw)-len(unique))

	type result struct {
		mr  domain.MergeRequest
		err error
	}

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
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		mrs = append(mrs, r.mr)
	}

	logger.Debug("gitlab: fetch summary", "total_mrs", len(mrs), "errors", len(errs))
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

func (a *GitLabAdapter) listAllMRs(ctx context.Context, primaryExclusion string) ([]*gl.BasicMergeRequest, []error) {
	var all []*gl.BasicMergeRequest
	var errs []error

	for _, src := range a.cfg.Sources {
		switch src.Type {
		case "group":
			allowedProjects, err := a.client.ListNonArchivedProjectIDs(ctx, src.ID)
			if err != nil {
				errs = append(errs, fmt.Errorf("source group=%q: %w", src.ID, err))
				continue
			}
			mrs, err := a.client.ListGroupMRs(ctx, src.ID, primaryExclusion)
			if err != nil {
				errs = append(errs, fmt.Errorf("source group=%q: %w", src.ID, err))
				continue
			}
			logger := ilog.FromContext(ctx)
			for _, mr := range mrs {
				if allowedProjects[mr.ProjectID] {
					all = append(all, mr)
				} else {
					logger.Debug("gitlab: skipping MR from archived project", "iid", mr.IID, "project", mr.ProjectID)
				}
			}
		case "user":
			mrs, err := a.client.ListUserMRs(ctx, src.Username)
			if err != nil {
				errs = append(errs, fmt.Errorf("source user=%q: %w", src.Username, err))
				continue
			}
			logger := ilog.FromContext(ctx)
			for _, mr := range mrs {
				archived, err := a.client.IsProjectArchived(ctx, mr.ProjectID)
				if err != nil {
					errs = append(errs, fmt.Errorf("source user=%q MR=%d: %w", src.Username, mr.IID, err))
					continue
				}
				if !archived {
					all = append(all, mr)
				} else {
					logger.Debug("gitlab: skipping MR from archived project", "iid", mr.IID, "project", mr.ProjectID)
				}
			}
		default:
			errs = append(errs, fmt.Errorf("source: unknown type %q", src.Type))
		}
	}

	return all, errs
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
