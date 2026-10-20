package gitlabadpt

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	ilog "github.com/ceffo/mrboard/internal/log"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// gqlDiffStatsBatchSize caps how many MRs go into one aliased diffStats query.
// GitLab rejects a query whose complexity exceeds 250 (authenticated). GitLab
// scores every selected field 1, and a field that calls Gitaly 1 more; the
// discussions batch measured exactly 16 per alias, its field count. One
// diffStats alias selects project, mergeRequest, diffStats (Gitaly) and three
// scalars: an estimated 7, so 30 aliases plus queryComplexity { score limit }
// score about 213. The estimate is not yet measured against GitLab: the query
// requests queryComplexity and the debug log reports the real score, which is
// what may raise this cap. An over-limit batch fails whole, every refresh.
const gqlDiffStatsBatchSize = 30

// diffStatsTarget is an MR whose diff stats must be fetched.
type diffStatsTarget struct {
	key mrKey
	req pkggitlab.MRDiffStatsRequest
}

// liveDiffHead is a deduped survivor's identity, head SHA and GraphQL address,
// as seen by the phase-1 listing.
type liveDiffHead struct {
	key         mrKey
	headSHA     string
	projectPath string
	iid         string
}

// liveDiffHeads collects every deduped survivor's head SHA, from either source.
func liveDiffHeads(rest []*gl.BasicMergeRequest, gql []pkggitlab.GQLMergeRequest) []liveDiffHead {
	heads := make([]liveDiffHead, 0, len(rest)+len(gql))
	for _, mr := range gql {
		heads = append(heads, liveDiffHead{
			key: gqlMRKey(mr), headSHA: mr.DiffHeadSHA, projectPath: mr.Project.FullPath, iid: mr.IID,
		})
	}
	for _, mr := range rest {
		heads = append(heads, liveDiffHead{
			key:         mrKey{ProjectID: int(mr.ProjectID), IID: int(mr.IID)},
			headSHA:     mr.SHA,
			projectPath: projectPathFromRef(mr.References),
			iid:         strconv.FormatInt(mr.IID, 10),
		})
	}
	return heads
}

// planDiffStats splits the live MRs into those whose cached stats still describe
// their diff and those to fetch. Cached stats are reused only when the previous
// snapshot recorded stats for the same non-empty head SHA: the head SHA moves on
// every push and on nothing else that changes the diff. An MR with no GraphQL
// address cannot be fetched and is left unknown.
func planDiffStats(
	heads []liveDiffHead, previous []domain.MergeRequest,
) (cached map[mrKey][]domain.FileDiffStat, targets []diffStatsTarget, unaddressable int) {
	prevByKey := make(map[mrKey]domain.MergeRequest, len(previous))
	for _, mr := range previous {
		prevByKey[mr.Key()] = mr
	}
	cached = make(map[mrKey][]domain.FileDiffStat)
	for _, h := range heads {
		if prev, ok := prevByKey[h.key]; ok && h.headSHA != "" && prev.HeadSHA == h.headSHA && prev.DiffStats != nil {
			cached[h.key] = prev.DiffStats
			continue
		}
		if h.projectPath == "" || h.iid == "" {
			unaddressable++
			continue
		}
		targets = append(targets, diffStatsTarget{
			key: h.key,
			req: pkggitlab.MRDiffStatsRequest{ProjectFullPath: h.projectPath, IID: h.iid},
		})
	}
	return cached, targets, unaddressable
}

// diffStatsStage fetches the stats of every target, in aliased batches of at most
// gqlDiffStatsBatchSize running concurrently. A failed batch leaves its MRs out of
// the result (unknown size) and contributes one error.
func (a *GitLabAdapter) diffStatsStage(
	ctx context.Context, targets []diffStatsTarget,
) (map[mrKey][]domain.FileDiffStat, []error) {
	logger := ilog.FromContext(ctx)
	fetched := make(map[mrKey][]domain.FileDiffStat, len(targets))
	var errs []error
	var mu sync.Mutex
	var wg sync.WaitGroup
	for start := 0; start < len(targets); start += gqlDiffStatsBatchSize {
		batch := targets[start:min(start+gqlDiffStatsBatchSize, len(targets))]
		wg.Add(1)
		go func() {
			defer wg.Done()
			batchStart := time.Now()
			reqs := make([]pkggitlab.MRDiffStatsRequest, len(batch))
			for i, t := range batch {
				reqs[i] = t.req
			}
			results, err := a.client.FetchMRsDiffStatsGraphQL(ctx, reqs)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				logger.Error("gitlab: diff stats batch failed", "mrs", len(batch), "error", err)
				errs = append(errs, fmt.Errorf("diff stats batch of %d MRs: %w", len(batch), err))
				return
			}
			for i, t := range batch {
				if i >= len(results) || !results[i].Found || results[i].Stats == nil {
					continue
				}
				fetched[t.key] = toDomainDiffStats(results[i].Stats)
			}
			logger.Info("gitlab: diff stats batch done",
				"mrs", len(batch), "duration", ilog.FmtDur(time.Since(batchStart)))
		}()
	}
	wg.Wait()
	return fetched, errs
}

func toDomainDiffStats(stats []pkggitlab.GQLDiffStat) []domain.FileDiffStat {
	out := make([]domain.FileDiffStat, len(stats))
	for i, s := range stats {
		out[i] = domain.FileDiffStat{Path: s.Path, Additions: s.Additions, Deletions: s.Deletions}
	}
	return out
}

// applyDiffStats sets each MR's DiffStats from stats; an MR absent from stats is
// left with unknown (nil) stats.
func applyDiffStats(mrs []domain.MergeRequest, stats map[mrKey][]domain.FileDiffStat) {
	for i := range mrs {
		mrs[i].DiffStats = stats[mrs[i].Key()]
	}
}
