package gitlabadpt

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	ilog "github.com/ceffo/mrboard/internal/log"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// mergedListing is one merged-since listing call: a source ID, or a reviewer
// username when reviewer MRs are included.
type mergedListing struct {
	label    string
	id       string
	reviewer bool
	list     func(context.Context, string, time.Time) ([]pkggitlab.GQLMergeRequest, error)
}

// mergedListings lists the calls covering the board's sources: every configured
// source ID, plus every reviewer username when reviewer MRs are included.
func (a *GitLabAdapter) mergedListings(opts mrsvc.FetchOptions) ([]mergedListing, error) {
	var out []mergedListing
	for _, src := range a.cfg.Sources {
		for _, id := range src.IDs {
			switch src.Type {
			case mrsvc.SourceTypeGroup:
				out = append(out, mergedListing{label: "group", id: id, list: a.client.FetchGroupMergedMRsGraphQL})
			case mrsvc.SourceTypeUser:
				out = append(out, mergedListing{label: "user", id: id, list: a.client.FetchUserMergedMRsGraphQL})
			default:
				return nil, fmt.Errorf("merged source: unknown type %q", src.Type)
			}
		}
	}
	if opts.IncludeReviewerMRs {
		for _, u := range a.cfg.ReviewerUsernames {
			out = append(out, mergedListing{
				label: "reviewer", id: u, reviewer: true, list: a.client.FetchReviewerMergedMRsGraphQL,
			})
		}
	}
	return out, nil
}

// listMergedStage lists the MRs merged at or after opts.MergedSince over the
// board's sources, in parallel. It skips archived projects, as the open listings
// do, and any MR GitLab reports merged before the bound. primaryKeys holds the
// MRs some non-reviewer listing returned. A failed listing contributes one
// error; GraphQL is the only route, the REST equivalent being too recent.
func (a *GitLabAdapter) listMergedStage(
	ctx context.Context, opts mrsvc.FetchOptions,
) (merged []pkggitlab.GQLMergeRequest, primaryKeys map[mrKey]bool, errs []error) {
	primaryKeys = make(map[mrKey]bool)
	if opts.MergedSince.IsZero() {
		return nil, primaryKeys, nil
	}
	logger := ilog.FromContext(ctx)
	start := time.Now()

	listings, err := a.mergedListings(opts)
	if err != nil {
		return nil, primaryKeys, []error{err}
	}
	type result struct {
		mrs []pkggitlab.GQLMergeRequest
		err error
	}
	results := make([]result, len(listings))
	var wg sync.WaitGroup
	for i, l := range listings {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mrs, err := l.list(ctx, l.id, opts.MergedSince)
			if err != nil {
				err = fmt.Errorf("merged %s=%q: %w", l.label, l.id, err)
			}
			results[i] = result{mrs: mrs, err: err}
		}()
	}
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			logger.Error("gitlab: merged listing failed", "error", r.err)
			errs = append(errs, r.err)
			continue
		}
		for _, mr := range r.mrs {
			if mr.Project.Archived || !mergedOnOrAfter(mr, opts.MergedSince) {
				continue
			}
			merged = append(merged, mr)
			if !listings[i].reviewer {
				primaryKeys[gqlMRKey(mr)] = true
			}
		}
	}
	logger.Info("gitlab: merged listing done",
		"since", opts.MergedSince, "listings", len(listings), "merged", len(merged), "errors", len(errs),
		"duration", ilog.FmtDur(time.Since(start)))
	return merged, primaryKeys, errs
}

// mergedOnOrAfter reports whether mr carries a merge time at or after since.
func mergedOnOrAfter(mr pkggitlab.GQLMergeRequest, since time.Time) bool {
	at, err := time.Parse(time.RFC3339, mr.MergedAt)
	return err == nil && !at.Before(since)
}

// mapMergedMRs maps merged survivors from their thin listing alone: no
// discussions are fetched, so reviewer states derive from approvedBy only.
func mapMergedMRs(mrs []pkggitlab.GQLMergeRequest) []domain.MergeRequest {
	out := make([]domain.MergeRequest, 0, len(mrs))
	for _, mr := range mrs {
		out = append(out, MapMRFromGraphQL(mr))
	}
	return out
}
