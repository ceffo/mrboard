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
	Sources         []mrsvc.Source
	ExcludedAuthors []string
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

	// Phase 1: fetch all sources in parallel.
	listStart := time.Now()
	rawMRs, mappedMRs, errs := a.listAllMRs(ctx, primaryExclusion)
	logger.Info("gitlab: source listing done",
		"raw", len(rawMRs), "mapped", len(mappedMRs), "source_errors", len(errs),
		"duration", ilog.FmtDur(time.Since(listStart)))

	// Phase 2: deduplicate — build a combined slice (mapped first so they win
	// on collision, then raw stubs with just key fields), run a single
	// dedup+exclusion pass, then separate survivors by source.
	deduper := MRDeduplicator{ExcludedAuthors: a.cfg.ExcludedAuthors}

	rawByKey := make(map[mrKey]*gl.BasicMergeRequest, len(rawMRs))
	combined := make([]domain.MergeRequest, 0, len(mappedMRs)+len(rawMRs))
	combined = append(combined, mappedMRs...)
	for _, mr := range rawMRs {
		k := mrKey{projectID: int(mr.ProjectID), iid: int(mr.IID)}
		rawByKey[k] = mr
		authorUsername := ""
		if mr.Author != nil {
			authorUsername = mr.Author.Username
		}
		logger.Debug("gitlab: raw MR", "iid", mr.IID, "title", mr.Title, "author", authorUsername)
		combined = append(combined, domain.MergeRequest{
			ProjectID: int(mr.ProjectID),
			IID:       int(mr.IID),
			Author:    authorUsername,
		})
	}

	deduped := deduper.Deduplicate(combined)

	mappedKeys := make(map[mrKey]bool, len(mappedMRs))
	for _, mr := range mappedMRs {
		mappedKeys[mrKey{projectID: mr.ProjectID, iid: mr.IID}] = true
	}

	var finalMRs []domain.MergeRequest
	var unique []*gl.BasicMergeRequest
	for _, mr := range deduped {
		k := mrKey{projectID: mr.ProjectID, iid: mr.IID}
		if mappedKeys[k] {
			finalMRs = append(finalMRs, mr)
		} else if raw, ok := rawByKey[k]; ok {
			unique = append(unique, raw)
		}
	}
	logger.Info("gitlab: dedup summary",
		"raw", len(rawMRs), "mapped", len(mappedMRs),
		"unique_to_enrich", len(unique), "already_mapped", len(finalMRs))

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

	var enrichErrs int
	for _, r := range results {
		if r.err != nil {
			logger.Error("gitlab: enrich MR failed", "error", r.err)
			errs = append(errs, r.err)
			enrichErrs++
			continue
		}
		finalMRs = append(finalMRs, r.mr)
	}

	logger.Info("gitlab: enrichment done",
		"enriched", len(unique)-enrichErrs, "enrich_errors", enrichErrs,
		"duration", ilog.FmtDur(time.Since(enrichStart)))
	logger.Info("gitlab: fetch done",
		"mrs", len(finalMRs), "errors", len(errs),
		"total_duration", ilog.FmtDur(time.Since(fetchStart)))
	return finalMRs, errs
}

// GetDetail implements mrsvc.MergeRequestSource.
func (a *GitLabAdapter) GetDetail(ctx context.Context, projectID, mrIID int64) (string, []domain.Thread, error) {
	logger := ilog.FromContext(ctx)
	start := time.Now()
	logger.Info("gitlab: get detail", "project_id", projectID, "mr_iid", mrIID)
	desc, err := a.client.GetMRDescription(ctx, projectID, mrIID)
	if err != nil {
		return "", nil, fmt.Errorf("get detail project=%d MR=%d description: %w", projectID, mrIID, err)
	}
	discussions, err := a.client.GetMRDiscussions(ctx, projectID, mrIID)
	if err != nil {
		return desc, nil, fmt.Errorf("get detail project=%d MR=%d discussions: %w", projectID, mrIID, err)
	}
	threads := MapDiscussionsToThreads(discussions)
	logger.Info("gitlab: get detail done", "project_id", projectID, "mr_iid", mrIID,
		"threads", len(threads), "duration", ilog.FmtDur(time.Since(start)))
	return desc, threads, nil
}

// sourceResult is the output of fetching a single source ID.
// raw holds MRs from group sources that still need enrichment.
// mapped holds fully-mapped MRs from GraphQL user sources.
type sourceResult struct {
	raw    []*gl.BasicMergeRequest
	mapped []domain.MergeRequest
	errs   []error
}

// listAllMRs fetches every source ID in parallel and merges the results.
func (a *GitLabAdapter) listAllMRs(
	ctx context.Context, primaryExclusion string,
) ([]*gl.BasicMergeRequest, []domain.MergeRequest, []error) {
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

	var allRaw []*gl.BasicMergeRequest
	var allMapped []domain.MergeRequest
	var errs []error
	for _, r := range results {
		allRaw = append(allRaw, r.raw...)
		allMapped = append(allMapped, r.mapped...)
		errs = append(errs, r.errs...)
	}
	return allRaw, allMapped, errs
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
			"duration", ilog.FmtDur(time.Since(start)))
		return sourceResult{raw: active}

	case mrsvc.SourceTypeUser:
		return a.fetchUserSourceGraphQL(ctx, id, logger, start)

	default:
		logger.Error("gitlab: unknown source type", "type", srcType)
		return sourceResult{errs: []error{fmt.Errorf("source: unknown type %q", srcType)}}
	}
}

// fetchUserSourceGraphQL fetches a user source via GraphQL (single query, no enrichment phase).
// Falls back to REST on error.
func (a *GitLabAdapter) fetchUserSourceGraphQL(
	ctx context.Context,
	username string,
	logger *slog.Logger,
	start time.Time,
) sourceResult {
	gqlMRs, err := a.client.FetchUserMRsGraphQL(ctx, username)
	if err != nil {
		logger.Warn("gitlab: graphql user fetch failed, falling back to REST", "username", username, "error", err)
		return a.fetchUserSourceREST(ctx, username, logger, start)
	}

	var mapped []domain.MergeRequest
	for _, mr := range gqlMRs {
		if mr.Project.Archived {
			logger.Debug("gitlab: skipping MR from archived project (graphql)", "iid", mr.IID, "project", mr.Project.FullPath)
			continue
		}
		if mr.Discussions.PageInfo.HasNextPage {
			logger.Warn("gitlab: graphql discussions overflow, thread count may be incomplete",
				"username", username, "mr_iid", mr.IID)
		}
		mapped = append(mapped, MapMRFromGraphQL(mr))
	}
	logger.Info("gitlab: user source fetched (graphql)",
		"username", username, "total", len(gqlMRs), "active", len(mapped),
		"duration", ilog.FmtDur(time.Since(start)))
	return sourceResult{mapped: mapped}
}

// fetchUserSourceREST is the legacy REST fallback for user sources.
func (a *GitLabAdapter) fetchUserSourceREST(
	ctx context.Context,
	username string,
	logger *slog.Logger,
	start time.Time,
) sourceResult {
	mrs, err := a.client.ListUserMRs(ctx, username)
	if err != nil {
		logger.Error("gitlab: list user MRs failed (REST)", "username", username, "error", err)
		return sourceResult{errs: []error{fmt.Errorf("source user=%q: %w", username, err)}}
	}
	var active []*gl.BasicMergeRequest
	var errs []error
	for _, mr := range mrs {
		archived, err := a.client.IsProjectArchived(ctx, mr.ProjectID)
		if err != nil {
			logger.Error("gitlab: check project archived failed",
				"username", username, "mr", mr.IID, "project", mr.ProjectID, "error", err)
			errs = append(errs, fmt.Errorf("source user=%q MR=%d: %w", username, mr.IID, err))
			continue
		}
		if !archived {
			active = append(active, mr)
		} else {
			logger.Debug("gitlab: skipping MR from archived project (REST)", "iid", mr.IID, "project", mr.ProjectID)
		}
	}
	logger.Info("gitlab: user source fetched (REST fallback)",
		"username", username, "total", len(mrs), "active", len(active),
		"duration", ilog.FmtDur(time.Since(start)))
	return sourceResult{raw: active, errs: errs}
}

// FetchMR implements mrsvc.MergeRequestSource.
func (a *GitLabAdapter) FetchMR(ctx context.Context, projectID int64, mrIID int64) (domain.MergeRequest, error) {
	logger := ilog.FromContext(ctx)
	start := time.Now()
	logger.Info("gitlab: fetch MR", "project_id", projectID, "mr_iid", mrIID)
	mr, err := a.client.GetMR(ctx, projectID, mrIID)
	if err != nil {
		return domain.MergeRequest{}, err
	}
	result, err := a.enrichMR(ctx, mr)
	if err != nil {
		return domain.MergeRequest{}, err
	}
	logger.Info("gitlab: fetch MR done", "project_id", projectID, "mr_iid", mrIID,
		"duration", ilog.FmtDur(time.Since(start)))
	return result, nil
}

// GetProjectMembers implements mrsvc.MergeRequestSource.
func (a *GitLabAdapter) GetProjectMembers(ctx context.Context, projectID int64) ([]domain.ProjectMember, error) {
	logger := ilog.FromContext(ctx)
	start := time.Now()
	logger.Info("gitlab: get project members", "project_id", projectID)
	const developerLevel = 40
	members, err := a.client.GetProjectMembers(ctx, projectID, developerLevel)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ProjectMember, len(members))
	for i, m := range members {
		result[i] = domain.ProjectMember{
			UserID:   m.ID,
			Username: m.Username,
			Name:     m.Name,
		}
	}
	logger.Info("gitlab: get project members done", "project_id", projectID,
		"members", len(result), "duration", ilog.FmtDur(time.Since(start)))
	return result, nil
}

// SaveApprovers implements mrsvc.MergeRequestSource.
func (a *GitLabAdapter) SaveApprovers(ctx context.Context, projectID, mrIID int64, userIDs []int64) error {
	logger := ilog.FromContext(ctx)
	start := time.Now()
	logger.Info("gitlab: save approvers", "project_id", projectID, "mr_iid", mrIID,
		"user_ids", userIDs, "required", len(userIDs))
	rules, err := a.client.GetMRApprovalRules(ctx, projectID, mrIID)
	if err != nil {
		return err
	}
	required := len(userIDs)
	payload := pkggitlab.MRApprovalRulePayload{
		Name:              approversRuleName,
		ApprovalsRequired: required,
		UserIDs:           userIDs,
	}
	for _, r := range rules {
		if r.Name == approversRuleName {
			err = a.client.UpdateMRApprovalRule(ctx, projectID, mrIID, r.ID, payload)
			if err != nil {
				return err
			}
			logger.Info("gitlab: approval rule updated", "project_id", projectID, "mr_iid", mrIID,
				"rule_id", r.ID, "required", required, "duration", ilog.FmtDur(time.Since(start)))
			return nil
		}
	}
	_, err = a.client.CreateMRApprovalRule(ctx, projectID, mrIID, payload)
	if err != nil {
		return err
	}
	logger.Info("gitlab: approval rule created", "project_id", projectID, "mr_iid", mrIID,
		"required", required, "duration", ilog.FmtDur(time.Since(start)))
	return nil
}

func (a *GitLabAdapter) enrichMR(ctx context.Context, mr *gl.BasicMergeRequest) (domain.MergeRequest, error) {
	if mr.Draft {
		discussions, err := a.client.GetMRDiscussions(ctx, mr.ProjectID, mr.IID)
		if err != nil {
			return domain.MergeRequest{}, fmt.Errorf("enrichMR project=%d MR=%d discussions: %w", mr.ProjectID, mr.IID, err)
		}
		return MapMR(mr, discussions, &gl.MergeRequestApprovals{}, nil), nil
	}

	type discResult struct {
		discussions []*gl.Discussion
		err         error
	}
	type apprResult struct {
		approvals *gl.MergeRequestApprovals
		err       error
	}
	type rulesResult struct {
		rules []*gl.MergeRequestApprovalRule
		err   error
	}

	discCh := make(chan discResult, 1)
	apprCh := make(chan apprResult, 1)
	rulesCh := make(chan rulesResult, 1)

	go func() {
		d, err := a.client.GetMRDiscussions(ctx, mr.ProjectID, mr.IID)
		discCh <- discResult{d, err}
	}()
	go func() {
		a, err := a.client.GetMRApprovals(ctx, mr.ProjectID, mr.IID)
		apprCh <- apprResult{a, err}
	}()
	go func() {
		r, err := a.client.GetMRApprovalRules(ctx, mr.ProjectID, mr.IID)
		rulesCh <- rulesResult{r, err}
	}()

	dr := <-discCh
	ar := <-apprCh
	rr := <-rulesCh

	if dr.err != nil {
		return domain.MergeRequest{}, fmt.Errorf("enrichMR project=%d MR=%d discussions: %w", mr.ProjectID, mr.IID, dr.err)
	}
	if ar.err != nil {
		return domain.MergeRequest{}, fmt.Errorf("enrichMR project=%d MR=%d approvals: %w", mr.ProjectID, mr.IID, ar.err)
	}
	if rr.err != nil {
		return domain.MergeRequest{}, fmt.Errorf("enrichMR project=%d MR=%d approval_rules: %w", mr.ProjectID, mr.IID, rr.err)
	}

	return MapMR(mr, dr.discussions, ar.approvals, rr.rules), nil
}
