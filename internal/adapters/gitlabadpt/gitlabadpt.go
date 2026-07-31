// Package gitlabadpt implements mrsvc.MergeRequestSource using the GitLab REST API.
package gitlabadpt

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
	Sources           []mrsvc.Source
	ExcludedAuthors   []string
	ReviewerUsernames []string
}

// gitLabClient is the set of pkg/gitlab.Client capabilities used by the adapter.
// Declaring it as an interface here lets callers (and tests) substitute a fake
// that implements only the sub-interfaces they need.
type gitLabClient interface {
	pkggitlab.MRLister
	pkggitlab.MREnricher
	pkggitlab.MRWriter
}

// GitLabAdapter implements mrsvc.MergeRequestSource using a live GitLab client.
type GitLabAdapter struct {
	client gitLabClient
	cfg    Config
}

// New constructs a GitLabAdapter.
func New(client gitLabClient, cfg Config) *GitLabAdapter {
	return &GitLabAdapter{client: client, cfg: cfg}
}

// FetchAll implements mrsvc.MergeRequestSource.
func (a *GitLabAdapter) FetchAll(ctx context.Context, opts mrsvc.FetchOptions) ([]domain.MergeRequest, []error) {
	logger := ilog.FromContext(ctx)
	fetchStart := time.Now()
	logger.Info("gitlab: fetch start", "sources", len(a.cfg.Sources), "excluded_authors", a.cfg.ExcludedAuthors,
		"include_reviewer_mrs", opts.IncludeReviewerMRs)

	// Stage 1: list — fetch all sources via the thin (no-discussions) query, return
	// raw REST stubs, raw thin GraphQL MRs, and primary-source keys.
	rawMRs, gqlRawMRs, primaryKeys, errs := a.listStage(ctx, opts)

	// Stage 2: dedup — combine, apply exclusions, split survivors back into
	// REST stubs and GraphQL-thin stubs, each still needing discussions.
	toEnrichREST, toEnrichGQL := dedupStage(rawMRs, gqlRawMRs, a.cfg.ExcludedAuthors, logger)

	// Stage 3: enrich — fetch discussions (REST or GraphQL, matching each MR's
	// source) for every deduped survivor in parallel, exactly once per MR.
	enrichStart := time.Now()
	finalMRs, enrichErrs := a.enrichStage(ctx, toEnrichREST, toEnrichGQL)
	errs = append(errs, enrichErrs...)
	logger.Info("gitlab: enrichment done",
		"enriched", len(finalMRs), "enrich_errors", len(enrichErrs),
		"duration", ilog.FmtDur(time.Since(enrichStart)))

	// Mark MRs that came exclusively from the reviewer fetch.
	if opts.IncludeReviewerMRs {
		for i, mr := range finalMRs {
			if !primaryKeys[mrKey{ProjectID: mr.ProjectID, IID: mr.IID}] {
				finalMRs[i].ReviewerSource = true
			}
		}
	}

	logger.Info("gitlab: fetch done",
		"mrs", len(finalMRs), "errors", len(errs),
		"total_duration", ilog.FmtDur(time.Since(fetchStart)))
	return finalMRs, errs
}

// listStage fetches all configured sources (primary + reviewer) in parallel,
// via the thin (no-discussions) GraphQL query for user/reviewer sources.
// primaryKeys contains the keys of MRs from primary sources only, used later to
// identify reviewer-only MRs.
func (a *GitLabAdapter) listStage(
	ctx context.Context,
	opts mrsvc.FetchOptions,
) (rawMRs []*gl.BasicMergeRequest, gqlRawMRs []pkggitlab.GQLMergeRequest, primaryKeys map[mrKey]bool, errs []error) {
	logger := ilog.FromContext(ctx)

	var primaryExclusion string
	if len(a.cfg.ExcludedAuthors) > 0 {
		primaryExclusion = a.cfg.ExcludedAuthors[0]
	}

	listStart := time.Now()
	rawMRs, gqlRawMRs, errs = a.listAllMRs(ctx, primaryExclusion)
	logger.Info("gitlab: source listing done",
		"raw", len(rawMRs), "gql_raw", len(gqlRawMRs), "source_errors", len(errs),
		"duration", ilog.FmtDur(time.Since(listStart)))

	// Capture keys before appending reviewer MRs so we can distinguish them later.
	primaryKeys = make(map[mrKey]bool, len(rawMRs)+len(gqlRawMRs))
	for _, mr := range gqlRawMRs {
		primaryKeys[mrKey{ProjectID: parseGIDNumericSafe(mr.Project.ID), IID: parseIIDSafe(mr.IID)}] = true
	}
	for _, mr := range rawMRs {
		primaryKeys[mrKey{ProjectID: int(mr.ProjectID), IID: int(mr.IID)}] = true
	}

	if opts.IncludeReviewerMRs && len(a.cfg.ReviewerUsernames) > 0 {
		revStart := time.Now()
		revRaw, revGQLRaw, revErrs := a.listReviewerMRs(ctx)
		logger.Info("gitlab: reviewer listing done",
			"raw", len(revRaw), "gql_raw", len(revGQLRaw), "reviewer_errors", len(revErrs),
			"duration", ilog.FmtDur(time.Since(revStart)))
		rawMRs = append(rawMRs, revRaw...)
		gqlRawMRs = append(gqlRawMRs, revGQLRaw...)
		errs = append(errs, revErrs...)
	}
	return rawMRs, gqlRawMRs, primaryKeys, errs
}

// dedupStage builds a combined slice (GraphQL-thin stubs first so they win on
// collision, matching the previous mapped-wins precedence), runs the
// dedup+exclusion pass, then splits survivors back into REST stubs and
// GraphQL-thin stubs — both still need discussions, fetched in enrichStage.
func dedupStage(
	rawMRs []*gl.BasicMergeRequest,
	gqlRawMRs []pkggitlab.GQLMergeRequest,
	excludedAuthors []string,
	logger *slog.Logger,
) (toEnrichREST []*gl.BasicMergeRequest, toEnrichGQL []pkggitlab.GQLMergeRequest) {
	deduper := MRDeduplicator{ExcludedAuthors: excludedAuthors}

	restByKey := make(map[mrKey]*gl.BasicMergeRequest, len(rawMRs))
	gqlByKey := make(map[mrKey]pkggitlab.GQLMergeRequest, len(gqlRawMRs))
	combined := make([]domain.MergeRequest, 0, len(gqlRawMRs)+len(rawMRs))

	for _, mr := range gqlRawMRs {
		k := mrKey{ProjectID: parseGIDNumericSafe(mr.Project.ID), IID: parseIIDSafe(mr.IID)}
		gqlByKey[k] = mr
		combined = append(combined, domain.MergeRequest{ProjectID: k.ProjectID, IID: k.IID, Author: mr.Author.Username})
	}
	for _, mr := range rawMRs {
		k := mrKey{ProjectID: int(mr.ProjectID), IID: int(mr.IID)}
		restByKey[k] = mr
		authorUsername := ""
		if mr.Author != nil {
			authorUsername = mr.Author.Username
		}
		logger.Debug("gitlab: raw MR", "iid", mr.IID, "title", mr.Title, "author", authorUsername)
		combined = append(combined, domain.MergeRequest{ProjectID: k.ProjectID, IID: k.IID, Author: authorUsername})
	}

	deduped := deduper.Deduplicate(combined)

	for _, mr := range deduped {
		k := mrKey{ProjectID: mr.ProjectID, IID: mr.IID}
		if g, ok := gqlByKey[k]; ok {
			toEnrichGQL = append(toEnrichGQL, g)
		} else if raw, ok := restByKey[k]; ok {
			toEnrichREST = append(toEnrichREST, raw)
		}
	}
	logger.Info("gitlab: dedup summary",
		"raw", len(rawMRs), "gql_raw", len(gqlRawMRs),
		"to_enrich_rest", len(toEnrichREST), "to_enrich_gql", len(toEnrichGQL))
	return toEnrichREST, toEnrichGQL
}

// enrichStage fetches discussions for each deduped survivor in parallel, bounded
// by enrichConcurrency across both REST stubs and GraphQL-thin stubs combined.
// Errors are collected and returned alongside successful results.
func (a *GitLabAdapter) enrichStage(
	ctx context.Context,
	toEnrichREST []*gl.BasicMergeRequest,
	toEnrichGQL []pkggitlab.GQLMergeRequest,
) ([]domain.MergeRequest, []error) {
	logger := ilog.FromContext(ctx)

	type result struct {
		mr  domain.MergeRequest
		err error
	}

	results := make([]result, len(toEnrichREST)+len(toEnrichGQL))
	sem := make(chan struct{}, enrichConcurrency)
	var wg sync.WaitGroup

	for i, mr := range toEnrichREST {
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
	offset := len(toEnrichREST)
	for i, mr := range toEnrichGQL {
		i, mr := i, mr
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			domainMR, err := a.enrichGQLMR(ctx, mr)
			results[offset+i] = result{mr: domainMR, err: err}
		}()
	}
	wg.Wait()

	var mrs []domain.MergeRequest
	var errs []error
	for _, r := range results {
		if r.err != nil {
			logger.Error("gitlab: enrich MR failed", "error", r.err)
			errs = append(errs, r.err)
			continue
		}
		mrs = append(mrs, r.mr)
	}
	return mrs, errs
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
// raw holds MRs from group sources (REST) that still need discussions.
// gqlRaw holds thin (no-discussions) MRs from GraphQL user/reviewer sources
// that still need discussions, fetched via a different route in enrichStage.
type sourceResult struct {
	raw    []*gl.BasicMergeRequest
	gqlRaw []pkggitlab.GQLMergeRequest
	errs   []error
}

// listAllMRs fetches every source ID in parallel and merges the results.
func (a *GitLabAdapter) listAllMRs(
	ctx context.Context, primaryExclusion string,
) ([]*gl.BasicMergeRequest, []pkggitlab.GQLMergeRequest, []error) {
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
	var allGQLRaw []pkggitlab.GQLMergeRequest
	var errs []error
	for _, r := range results {
		allRaw = append(allRaw, r.raw...)
		allGQLRaw = append(allGQLRaw, r.gqlRaw...)
		errs = append(errs, r.errs...)
	}
	return allRaw, allGQLRaw, errs
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

// fetchUserSourceGraphQL fetches a user source via the thin GraphQL query
// (phase-1 listing; discussions are fetched later in enrichStage).
// Falls back to REST on error.
func (a *GitLabAdapter) fetchUserSourceGraphQL(
	ctx context.Context, username string, logger *slog.Logger, start time.Time,
) sourceResult {
	return a.fetchSourceViaGQL(ctx, username, "user",
		a.client.FetchUserMRsThinGraphQL, a.fetchUserSourceREST, logger, start)
}

// fetchUserSourceREST is the legacy REST fallback for user sources.
func (a *GitLabAdapter) fetchUserSourceREST(
	ctx context.Context, username string, logger *slog.Logger, start time.Time,
) sourceResult {
	return a.fetchSourceViaREST(ctx, username, "user", a.client.ListUserMRs, logger, start)
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

// SetReviewers implements mrsvc.MergeRequestSource.
func (a *GitLabAdapter) SetReviewers(ctx context.Context, projectID int64, mrIID int64, userIDs []int64) error {
	logger := ilog.FromContext(ctx)
	start := time.Now()
	logger.Info("gitlab: set reviewers", "project_id", projectID, "mr_iid", mrIID, "count", len(userIDs))
	if err := a.client.SetMRReviewers(ctx, projectID, mrIID, userIDs); err != nil {
		return err
	}
	logger.Info("gitlab: set reviewers done", "project_id", projectID, "mr_iid", mrIID,
		"duration", ilog.FmtDur(time.Since(start)))
	return nil
}

// ResolveUsers implements mrsvc.MergeRequestSource.
// Unknown usernames are omitted from the result without error.
func (a *GitLabAdapter) ResolveUsers(ctx context.Context, usernames []string) ([]domain.User, error) {
	logger := ilog.FromContext(ctx)
	start := time.Now()
	logger.Info("gitlab: resolve users", "count", len(usernames))
	result := make([]domain.User, 0, len(usernames))
	for _, username := range usernames {
		u, err := a.client.ListUsersByUsername(ctx, username)
		if err != nil {
			return result, fmt.Errorf("resolve users username=%q: %w", username, err)
		}
		if u == nil {
			logger.Warn("gitlab: resolve users: unknown username", "username", username)
			continue
		}
		result = append(result, domain.User{
			ID:       u.ID,
			Username: u.Username,
			Name:     u.Name,
		})
	}
	logger.Info("gitlab: resolve users done", "requested", len(usernames), "resolved", len(result),
		"duration", ilog.FmtDur(time.Since(start)))
	return result, nil
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

// enrichGQLMR completes a phase-1 thin GraphQL MR by fetching its discussions —
// the one field the thin listing query omits — via a single-MR GraphQL query,
// then maps the result exactly as the old fat query's response would have been.
func (a *GitLabAdapter) enrichGQLMR(ctx context.Context, mr pkggitlab.GQLMergeRequest) (domain.MergeRequest, error) {
	logger := ilog.FromContext(ctx)
	discussions, hasNextPage, err := a.client.FetchMRDiscussionsGraphQL(ctx, mr.Project.FullPath, mr.IID)
	if err != nil {
		return domain.MergeRequest{}, fmt.Errorf(
			"enrichGQLMR project=%q MR=%s discussions: %w", mr.Project.FullPath, mr.IID, err)
	}
	if hasNextPage {
		logger.Warn("gitlab: graphql discussions overflow, thread count may be incomplete",
			"project", mr.Project.FullPath, "mr_iid", mr.IID)
	}
	mr.Discussions.Nodes = discussions
	mr.Discussions.PageInfo.HasNextPage = hasNextPage
	return MapMRFromGraphQL(mr), nil
}

// GetDiff implements mrsvc.MergeRequestSource.
// Fetches file diffs and diff refs (BaseSHA/HeadSHA) in parallel.
func (a *GitLabAdapter) GetDiff(ctx context.Context, projectID, mrIID int64) (domain.MRDiff, error) {
	logger := ilog.FromContext(ctx)
	start := time.Now()
	logger.Info("gitlab: get MR diff", "project_id", projectID, "mr_iid", mrIID)

	type diffsResult struct {
		diffs []*gl.MergeRequestDiff
		err   error
	}
	type refsResult struct {
		baseSHA string
		headSHA string
		err     error
	}

	diffsCh := make(chan diffsResult, 1)
	refsCh := make(chan refsResult, 1)

	go func() {
		d, err := a.client.GetMRDiffs(ctx, projectID, mrIID)
		diffsCh <- diffsResult{d, err}
	}()
	go func() {
		base, head, err := a.client.GetMRDiffRefs(ctx, projectID, mrIID)
		refsCh <- refsResult{base, head, err}
	}()

	dr := <-diffsCh
	rr := <-refsCh

	if dr.err != nil {
		return domain.MRDiff{}, dr.err
	}
	if rr.err != nil {
		return domain.MRDiff{}, rr.err
	}

	files := make([]domain.FileDiff, 0, len(dr.diffs))
	for _, d := range dr.diffs {
		added, removed := countDiffLines(d.Diff)
		files = append(files, domain.FileDiff{
			OldPath:      d.OldPath,
			NewPath:      d.NewPath,
			NewFile:      d.NewFile,
			DeletedFile:  d.DeletedFile,
			RenamedFile:  d.RenamedFile,
			TooLarge:     d.TooLarge,
			Diff:         d.Diff,
			LinesAdded:   added,
			LinesRemoved: removed,
		})
	}
	logger.Info("gitlab: get MR diff done", "project_id", projectID, "mr_iid", mrIID,
		"files", len(files), "base", rr.baseSHA, "duration", ilog.FmtDur(time.Since(start)))
	return domain.MRDiff{
		BaseSHA: rr.baseSHA,
		HeadSHA: rr.headSHA,
		Files:   files,
	}, nil
}

// GetFileContent implements mrsvc.MergeRequestSource.
func (a *GitLabAdapter) GetFileContent(ctx context.Context, projectID int64, path, ref string) ([]byte, error) {
	return a.client.GetRawFileContent(ctx, projectID, path, ref)
}

// listReviewerMRs fetches MRs for all configured reviewer usernames in parallel.
func (a *GitLabAdapter) listReviewerMRs(
	ctx context.Context,
) ([]*gl.BasicMergeRequest, []pkggitlab.GQLMergeRequest, []error) {
	logger := ilog.FromContext(ctx)
	results := make([]sourceResult, len(a.cfg.ReviewerUsernames))
	var wg sync.WaitGroup
	for i, username := range a.cfg.ReviewerUsernames {
		i, username := i, username
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = a.fetchReviewerSourceGraphQL(ctx, username, logger, time.Now())
		}()
	}
	wg.Wait()

	var allRaw []*gl.BasicMergeRequest
	var allGQLRaw []pkggitlab.GQLMergeRequest
	var errs []error
	for _, r := range results {
		allRaw = append(allRaw, r.raw...)
		allGQLRaw = append(allGQLRaw, r.gqlRaw...)
		errs = append(errs, r.errs...)
	}
	return allRaw, allGQLRaw, errs
}

// fetchReviewerSourceGraphQL fetches reviewer-requested MRs for username via the
// thin GraphQL query. Falls back to REST on error.
func (a *GitLabAdapter) fetchReviewerSourceGraphQL(
	ctx context.Context, username string, logger *slog.Logger, start time.Time,
) sourceResult {
	return a.fetchSourceViaGQL(ctx, username, "reviewer",
		a.client.FetchReviewerMRsThinGraphQL, a.fetchReviewerSourceREST, logger, start)
}

// fetchReviewerSourceREST is the REST fallback for reviewer-requested MRs.
func (a *GitLabAdapter) fetchReviewerSourceREST(
	ctx context.Context, username string, logger *slog.Logger, start time.Time,
) sourceResult {
	return a.fetchSourceViaREST(ctx, username, "reviewer", a.client.ListReviewerMRs, logger, start)
}

// fetchSourceViaGQL is the shared implementation for GQL-first fetches (user or reviewer).
// label is used in log messages ("user" or "reviewer").
func (a *GitLabAdapter) fetchSourceViaGQL(
	ctx context.Context,
	username, label string,
	gqlFetch func(context.Context, string) ([]pkggitlab.GQLMergeRequest, error),
	restFallback func(context.Context, string, *slog.Logger, time.Time) sourceResult,
	logger *slog.Logger,
	start time.Time,
) sourceResult {
	gqlMRs, err := gqlFetch(ctx, username)
	if err != nil {
		logger.Warn("gitlab: graphql "+label+" fetch failed, falling back to REST", "username", username, "error", err)
		return restFallback(ctx, username, logger, start)
	}

	var active []pkggitlab.GQLMergeRequest
	for _, mr := range gqlMRs {
		if mr.Project.Archived {
			logger.Debug("gitlab: skipping "+label+" MR from archived project (graphql)",
				"iid", mr.IID, "project", mr.Project.FullPath)
			continue
		}
		active = append(active, mr)
	}
	logger.Info("gitlab: "+label+" source fetched (graphql, thin)",
		"username", username, "total", len(gqlMRs), "active", len(active),
		"duration", ilog.FmtDur(time.Since(start)))
	return sourceResult{gqlRaw: active}
}

// fetchSourceViaREST is the shared REST-fallback implementation for user and reviewer sources.
// label is used in log and error messages ("user" or "reviewer").
func (a *GitLabAdapter) fetchSourceViaREST(
	ctx context.Context,
	username, label string,
	restFetch func(context.Context, string) ([]*gl.BasicMergeRequest, error),
	logger *slog.Logger,
	start time.Time,
) sourceResult {
	mrs, err := restFetch(ctx, username)
	if err != nil {
		logger.Error("gitlab: list "+label+" MRs failed (REST)", "username", username, "error", err)
		return sourceResult{errs: []error{fmt.Errorf("%s user=%q: %w", label, username, err)}}
	}
	var active []*gl.BasicMergeRequest
	var errs []error
	for _, mr := range mrs {
		archived, err := a.client.IsProjectArchived(ctx, mr.ProjectID)
		if err != nil {
			logger.Error("gitlab: check project archived failed",
				"username", username, "mr", mr.IID, "project", mr.ProjectID, "error", err)
			errs = append(errs, fmt.Errorf("%s user=%q MR=%d: %w", label, username, mr.IID, err))
			continue
		}
		if !archived {
			active = append(active, mr)
		} else {
			logger.Debug("gitlab: skipping "+label+" MR from archived project (REST)",
				"iid", mr.IID, "project", mr.ProjectID)
		}
	}
	logger.Info("gitlab: "+label+" source fetched (REST fallback)",
		"username", username, "total", len(mrs), "active", len(active),
		"duration", ilog.FmtDur(time.Since(start)))
	return sourceResult{raw: active, errs: errs}
}

// countDiffLines counts added and removed lines in a unified diff string.
func countDiffLines(diff string) (added, removed int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return added, removed
}
