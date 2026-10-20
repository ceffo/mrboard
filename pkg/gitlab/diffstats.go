package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ilog "github.com/ceffo/mrboard/internal/log"
)

// MRDiffStatsRequest identifies one MR within an aliased batch diff-stats query.
type MRDiffStatsRequest struct {
	ProjectFullPath string
	IID             string
}

// GQLDiffStat is one file's line counts from MergeRequest.diffStats.
type GQLDiffStat struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// MRDiffStatsResult is one MR's diff stats from an aliased batch query. Found is
// false when GitLab reported the MR as not found at that index. Stats is nil when
// GitLab returned no stats for a found MR, and empty for a known empty diff.
type MRDiffStatsResult struct {
	Found bool
	Stats []GQLDiffStat
}

// QueryComplexity is GitLab's own score of a GraphQL query against its limit.
type QueryComplexity struct {
	Score int `json:"score"`
	Limit int `json:"limit"`
}

// buildAliasedMRDiffStatsQuery builds one GraphQL document fetching diffStats for
// n MRs: mr0: project(fullPath: $p0) { mergeRequest(iid: $i0) { diffStats {...} } }
// mr1: ... Each MR gets its own variable pair so project/IID values never need
// string-escaping into the query text. queryComplexity reports the score GitLab
// computed for the document, so the batch size can be checked against real data.
func buildAliasedMRDiffStatsQuery(n int) string {
	params := make([]string, n)
	aliases := make([]string, n)
	for i := 0; i < n; i++ {
		params[i] = fmt.Sprintf("$p%d: ID!, $i%d: String!", i, i)
		aliases[i] = fmt.Sprintf(
			"  mr%d: project(fullPath: $p%d) {\n    mergeRequest(iid: $i%d) {\n"+
				"      diffStats { path additions deletions }\n    }\n  }",
			i, i, i)
	}
	return fmt.Sprintf("query(%s) {\n  queryComplexity { score limit }\n%s\n}",
		strings.Join(params, ", "), strings.Join(aliases, "\n"))
}

type gqlAliasedDiffStatsProject struct {
	MergeRequest *struct {
		DiffStats []GQLDiffStat `json:"diffStats"`
	} `json:"mergeRequest"`
}

type gqlAliasedRawResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []gqlError                 `json:"errors"`
}

// FetchMRsDiffStatsGraphQL fetches per-file diff stats for len(reqs) MRs in a
// single aliased GraphQL request. Results are aligned by index with reqs. Any
// GraphQL-level error fails the whole batch: a partial answer cannot be told
// apart from a missing MR.
func (c *Client) FetchMRsDiffStatsGraphQL(
	ctx context.Context, reqs []MRDiffStatsRequest,
) ([]MRDiffStatsResult, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	start := time.Now()
	c.logger.Debug("gitlab: graphql aliased MR diff stats", "count", len(reqs))

	const varsPerMR = 2 // one project-path variable and one IID variable per aliased MR
	query := buildAliasedMRDiffStatsQuery(len(reqs))
	variables := make(map[string]interface{}, len(reqs)*varsPerMR)
	for i, r := range reqs {
		variables[fmt.Sprintf("p%d", i)] = r.ProjectFullPath
		variables[fmt.Sprintf("i%d", i)] = r.IID
	}

	raw, err := c.doGQLGenericRequest(ctx, query, variables, "aliased MR diff stats")
	if err != nil {
		return nil, err
	}

	var result gqlAliasedRawResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("gitlab: graphql decode aliased MR diff stats response: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("gitlab: graphql aliased MR diff stats errors: %s", result.Errors[0].Message)
	}

	if qc, ok := result.Data["queryComplexity"]; ok {
		var complexity QueryComplexity
		if err := json.Unmarshal(qc, &complexity); err == nil {
			c.logger.Debug("gitlab: graphql aliased MR diff stats complexity",
				"count", len(reqs), "score", complexity.Score, "limit", complexity.Limit)
		}
	}

	results := make([]MRDiffStatsResult, len(reqs))
	for i, r := range reqs {
		var entry *gqlAliasedDiffStatsProject
		if rawEntry, ok := result.Data[fmt.Sprintf("mr%d", i)]; ok {
			if err := json.Unmarshal(rawEntry, &entry); err != nil {
				return nil, fmt.Errorf("gitlab: graphql decode diff stats for %s!%s: %w", r.ProjectFullPath, r.IID, err)
			}
		}
		if entry == nil || entry.MergeRequest == nil {
			c.logger.Warn("gitlab: graphql aliased MR diff stats: MR not found",
				"project", r.ProjectFullPath, "iid", r.IID)
			continue
		}
		results[i] = MRDiffStatsResult{Found: true, Stats: entry.MergeRequest.DiffStats}
	}
	c.logger.Debug("gitlab: graphql aliased MR diff stats done",
		"count", len(reqs), "duration", ilog.FmtDur(time.Since(start)))
	return results, nil
}
