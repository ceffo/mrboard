package gitlabadpt

import (
	"context"
	"fmt"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
	ilog "github.com/ceffo/mrboard/internal/log"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// diffGQLStage splits phase-1 GraphQL survivors into unchanged (their
// updatedAt matches the previous snapshot and the key wasn't forced stale) and
// changed. A nil previous snapshot means every MR is changed — an
// unconditional full fetch, which is what a cold cache (no snapshot file yet,
// or `mrboard fetch --cold`) produces. See docs/adr/0005, "Two-phase
// conditional fetch".
func diffGQLStage(
	toEnrichGQL []pkggitlab.GQLMergeRequest, previous []domain.MergeRequest, forceStale map[mrKey]bool,
) (unchanged, changed []pkggitlab.GQLMergeRequest, cachedByKey map[mrKey]domain.MergeRequest) {
	cachedByKey = make(map[mrKey]domain.MergeRequest, len(previous))
	for _, mr := range previous {
		cachedByKey[mr.Key()] = mr
	}

	for _, mr := range toEnrichGQL {
		k := gqlMRKey(mr)
		if cached, ok := cachedByKey[k]; ok && !forceStale[k] {
			if updatedAt, err := time.Parse(time.RFC3339, mr.UpdatedAt); err == nil && cached.UpdatedAt.Equal(updatedAt) {
				unchanged = append(unchanged, mr)
				continue
			}
		}
		changed = append(changed, mr)
	}
	return unchanged, changed, cachedByKey
}

// chunkGQLMRs splits mrs into groups of at most size, preserving order. Used
// to keep each aliased phase-2 discussions query under GitLab's per-request
// GraphQL complexity ceiling (see gqlDiscussionsBatchSize).
func chunkGQLMRs(mrs []pkggitlab.GQLMergeRequest, size int) [][]pkggitlab.GQLMergeRequest {
	if len(mrs) == 0 {
		return nil
	}
	chunks := make([][]pkggitlab.GQLMergeRequest, 0, (len(mrs)+size-1)/size)
	for len(mrs) > 0 {
		n := size
		if n > len(mrs) {
			n = len(mrs)
		}
		chunks = append(chunks, mrs[:n])
		mrs = mrs[n:]
	}
	return chunks
}

// enrichGQLMRsBatch completes N phase-1 thin GraphQL MRs with their
// discussions in a single aliased GraphQL request instead of N individual
// ones — phase 2 of the two-phase fetch for whichever MRs opts.Previous
// couldn't answer for (docs/adr/0005, "Two-phase conditional fetch").
func (a *GitLabAdapter) enrichGQLMRsBatch(
	ctx context.Context, mrs []pkggitlab.GQLMergeRequest,
) ([]domain.MergeRequest, error) {
	logger := ilog.FromContext(ctx)

	reqs := make([]pkggitlab.MRDiscussionsRequest, len(mrs))
	for i, mr := range mrs {
		reqs[i] = pkggitlab.MRDiscussionsRequest{ProjectFullPath: mr.Project.FullPath, IID: mr.IID}
	}

	discResults, err := a.client.FetchMRsDiscussionsGraphQL(ctx, reqs)
	if err != nil {
		return nil, fmt.Errorf("enrichGQLMRsBatch: %w", err)
	}

	domainMRs := make([]domain.MergeRequest, len(mrs))
	for i, mr := range mrs {
		if discResults[i].HasNextPage {
			logger.Warn("gitlab: graphql discussions overflow, thread count may be incomplete",
				"project", mr.Project.FullPath, "mr_iid", mr.IID)
		}
		mr.Discussions.Nodes = discResults[i].Discussions
		mr.Discussions.PageInfo.HasNextPage = discResults[i].HasNextPage
		domainMRs[i] = MapMRFromGraphQL(mr)
	}
	return domainMRs, nil
}
