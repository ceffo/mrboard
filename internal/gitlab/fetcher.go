package gitlab

import (
	"fmt"
	"sync"

	"github.com/mrboard/mrboard/internal/config"
	"github.com/mrboard/mrboard/internal/domain"
	gl "github.com/xanzy/go-gitlab"
)

// mrKey uniquely identifies an MR across all sources.
type mrKey struct {
	projectID int
	iid       int
}

// FetchAll retrieves all open MRs from all configured sources, deduplicates by
// (ProjectID, IID), and enriches each unique MR with discussions and approvals
// fetched in parallel. Partial results are returned alongside any errors.
func FetchAll(client *Client, cfg *config.Config) ([]domain.MergeRequest, []error) {
	raw, errs := listAllMRs(client, cfg)

	// Deduplicate: first occurrence wins.
	seen := make(map[mrKey]bool, len(raw))
	unique := make([]*gl.MergeRequest, 0, len(raw))
	for _, mr := range raw {
		k := mrKey{projectID: mr.ProjectID, iid: mr.IID}
		if seen[k] {
			continue
		}
		seen[k] = true
		unique = append(unique, mr)
	}

	type result struct {
		mr  domain.MergeRequest
		err error
	}

	results := make([]result, len(unique))
	var wg sync.WaitGroup
	wg.Add(len(unique))

	for i, mr := range unique {
		i, mr := i, mr
		go func() {
			defer wg.Done()
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

	return mrs, errs
}

// listAllMRs iterates all sources and collects raw MR objects (may contain duplicates).
func listAllMRs(client *Client, cfg *config.Config) ([]*gl.MergeRequest, []error) {
	var all []*gl.MergeRequest
	var errs []error

	for _, src := range cfg.Sources {
		switch src.Type {
		case "group":
			mrs, err := client.ListGroupMRs(src.ID)
			if err != nil {
				errs = append(errs, fmt.Errorf("source group=%q: %w", src.ID, err))
				continue
			}
			all = append(all, mrs...)
		case "user":
			mrs, err := client.ListUserMRs(src.Username)
			if err != nil {
				errs = append(errs, fmt.Errorf("source user=%q: %w", src.Username, err))
				continue
			}
			all = append(all, mrs...)
		default:
			errs = append(errs, fmt.Errorf("source: unknown type %q", src.Type))
		}
	}

	return all, errs
}

// enrichMR fetches discussions and approvals in parallel, then maps to domain.MergeRequest.
func enrichMR(client *Client, mr *gl.MergeRequest, requiredApprovals int) (domain.MergeRequest, error) {
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
