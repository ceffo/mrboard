package gitlab

import (
	"fmt"
	"sync"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
)

// enrichConcurrency caps the number of MRs being enriched simultaneously.
// Keeps us well under GitLab's rate limits and avoids thundering-herd latency.
const enrichConcurrency = 10

// mrKey uniquely identifies an MR across all sources.
type mrKey struct {
	projectID int64
	iid       int64
}

// FetchAll retrieves all open MRs from all configured sources, deduplicates by
// (ProjectID, IID), excludes configured authors, and enriches each unique MR
// with discussions and (for non-draft MRs) approvals. Enrichment is capped at
// enrichConcurrency concurrent requests. Partial results are returned alongside
// any errors.
func FetchAll(client *Client, cfg *config.Config) ([]domain.MergeRequest, []error) {
	client.logger.Debug("gitlab: fetch start", "excluded_authors", cfg.ExcludedAuthors)

	// First excluded author is applied server-side; any additional ones are
	// applied client-side below (GitLab only supports one not[author_username]).
	var primaryExclusion string
	if len(cfg.ExcludedAuthors) > 0 {
		primaryExclusion = cfg.ExcludedAuthors[0]
	}

	raw, errs := listAllMRs(client, cfg, primaryExclusion)

	excluded := make(map[string]bool, len(cfg.ExcludedAuthors))
	for _, u := range cfg.ExcludedAuthors {
		excluded[u] = true
	}

	// Dedup by (ProjectID, IID) — first occurrence wins. Client-side exclusion
	// for authors beyond the first. This is the only place MRs enter the
	// enrichment queue, so the same MR can never be enriched twice.
	seen := make(map[mrKey]bool, len(raw))
	unique := make([]*gl.BasicMergeRequest, 0, len(raw))
	for _, mr := range raw {
		authorUsername := ""
		if mr.Author != nil {
			authorUsername = mr.Author.Username
		}
		client.logger.Debug("gitlab: raw MR", "iid", mr.IID, "title", mr.Title, "author", authorUsername)
		if authorUsername != "" && excluded[authorUsername] {
			client.logger.Debug("gitlab: excluding MR", "iid", mr.IID, "author", authorUsername)
			continue
		}
		k := mrKey{projectID: mr.ProjectID, iid: mr.IID}
		if seen[k] {
			client.logger.Debug("gitlab: dedup drop", "iid", mr.IID, "project", mr.ProjectID)
			continue
		}
		seen[k] = true
		unique = append(unique, mr)
	}
	client.logger.Debug("gitlab: dedup summary", "raw", len(raw), "unique", len(unique), "dropped", len(raw)-len(unique))

	type result struct {
		mr  domain.MergeRequest
		err error
	}

	results := make([]result, len(unique))
	sem := make(chan struct{}, enrichConcurrency)
	var wg sync.WaitGroup

	for i, mr := range unique {
		i, mr := i, mr
		sem <- struct{}{} // blocks when enrichConcurrency goroutines are active
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			domainMR, err := enrichMR(client, mr, cfg.GitLab.RequiredApprovals)
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

	client.logger.Debug("gitlab: fetch summary", "total_mrs", len(mrs), "errors", len(errs))
	return mrs, errs
}

// listAllMRs iterates all sources and collects raw MR objects (may contain
// duplicates). primaryExclusion, if non-empty, is forwarded to the GitLab API
// as not[author_username] for group sources.
func listAllMRs(client *Client, cfg *config.Config, primaryExclusion string) ([]*gl.BasicMergeRequest, []error) {
	var all []*gl.BasicMergeRequest
	var errs []error

	for _, src := range cfg.Sources {
		switch src.Type {
		case "group":
			allowedProjects, err := client.ListNonArchivedProjectIDs(src.ID)
			if err != nil {
				errs = append(errs, fmt.Errorf("source group=%q: %w", src.ID, err))
				continue
			}
			mrs, err := client.ListGroupMRs(src.ID, primaryExclusion)
			if err != nil {
				errs = append(errs, fmt.Errorf("source group=%q: %w", src.ID, err))
				continue
			}
			for _, mr := range mrs {
				if allowedProjects[mr.ProjectID] {
					all = append(all, mr)
				} else {
					client.logger.Debug("gitlab: skipping MR from archived project", "iid", mr.IID, "project", mr.ProjectID)
				}
			}
		case "user":
			mrs, err := client.ListUserMRs(src.Username)
			if err != nil {
				errs = append(errs, fmt.Errorf("source user=%q: %w", src.Username, err))
				continue
			}
			for _, mr := range mrs {
				archived, err := client.IsProjectArchived(mr.ProjectID)
				if err != nil {
					errs = append(errs, fmt.Errorf("source user=%q MR=%d: %w", src.Username, mr.IID, err))
					continue
				}
				if !archived {
					all = append(all, mr)
				} else {
					client.logger.Debug("gitlab: skipping MR from archived project", "iid", mr.IID, "project", mr.ProjectID)
				}
			}
		default:
			errs = append(errs, fmt.Errorf("source: unknown type %q", src.Type))
		}
	}

	return all, errs
}

// enrichMR fetches discussions and (for non-draft MRs) approvals, then maps to
// domain.MergeRequest. Draft MRs skip the approvals call — their phase is
// always PhaseDraft regardless of approval state.
func enrichMR(client *Client, mr *gl.BasicMergeRequest, requiredApprovals int) (domain.MergeRequest, error) {
	if mr.Draft {
		discussions, err := client.GetMRDiscussions(mr.ProjectID, mr.IID)
		if err != nil {
			return domain.MergeRequest{}, fmt.Errorf("enrichMR project=%d MR=%d discussions: %w", mr.ProjectID, mr.IID, err)
		}
		// Approvals irrelevant for drafts; construct a zero-approval placeholder
		// so MapMR does not need a nil check.
		emptyApprovals := &gl.MergeRequestApprovals{
			ApprovalsRequired: int64(requiredApprovals),
			ApprovalsLeft:     int64(requiredApprovals),
		}
		return MapMR(mr, discussions, emptyApprovals, requiredApprovals), nil
	}

	// Non-draft: fetch discussions and approvals in parallel.
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
		d, err := client.GetMRDiscussions(mr.ProjectID, mr.IID)
		discCh <- discResult{d, err}
	}()
	go func() {
		a, err := client.GetMRApprovals(mr.ProjectID, mr.IID)
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

	return MapMR(mr, dr.discussions, ar.approvals, requiredApprovals), nil
}
