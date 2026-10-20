package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ilog "github.com/ceffo/mrboard/internal/log"
)

// GitAttributesRequest identifies one (project, ref) whose root .gitattributes is read
// within an aliased batch query.
type GitAttributesRequest struct {
	ProjectFullPath string
	Ref             string
}

// GitAttributesResult is one project's root .gitattributes from an aliased batch query.
// Found is false when GitLab reported the project as not found (or not visible). A found
// project with no .gitattributes on the ref has Exists false and empty Content.
type GitAttributesResult struct {
	Found   bool
	Exists  bool
	Content string
}

// buildAliasedGitAttributesQuery builds one GraphQL document reading the root
// .gitattributes of n (project, ref) pairs:
// ga0: project(fullPath: $p0) { repository { blobs(ref: $r0, paths: [".gitattributes"]) ... } }
// Each pair gets its own variables so values never need string-escaping into the query
// text. queryComplexity reports the score GitLab computed for the document, so the batch
// size can be checked against real data.
func buildAliasedGitAttributesQuery(n int) string {
	params := make([]string, n)
	aliases := make([]string, n)
	for i := 0; i < n; i++ {
		params[i] = fmt.Sprintf("$p%d: ID!, $r%d: String!", i, i)
		aliases[i] = fmt.Sprintf(
			"  ga%d: project(fullPath: $p%d) {\n    repository {\n"+
				"      blobs(ref: $r%d, paths: [\".gitattributes\"], first: 1) { nodes { rawTextBlob } }\n"+
				"    }\n  }",
			i, i, i)
	}
	return fmt.Sprintf("query(%s) {\n  queryComplexity { score limit }\n%s\n}",
		strings.Join(params, ", "), strings.Join(aliases, "\n"))
}

type gqlAliasedGitAttributesProject struct {
	Repository *struct {
		Blobs *struct {
			Nodes []struct {
				RawTextBlob *string `json:"rawTextBlob"`
			} `json:"nodes"`
		} `json:"blobs"`
	} `json:"repository"`
}

// FetchGitAttributesGraphQL reads the root .gitattributes of len(reqs) (project, ref)
// pairs in a single aliased GraphQL request. Results are aligned by index with reqs. Any
// GraphQL-level error fails the whole batch: a partial answer cannot be told apart from a
// missing file.
func (c *Client) FetchGitAttributesGraphQL(
	ctx context.Context, reqs []GitAttributesRequest,
) ([]GitAttributesResult, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	start := time.Now()
	c.logger.Debug("gitlab: graphql aliased gitattributes", "count", len(reqs))

	const varsPerReq = 2 // one project-path variable and one ref variable per alias
	query := buildAliasedGitAttributesQuery(len(reqs))
	variables := make(map[string]interface{}, len(reqs)*varsPerReq)
	for i, r := range reqs {
		variables[fmt.Sprintf("p%d", i)] = r.ProjectFullPath
		variables[fmt.Sprintf("r%d", i)] = r.Ref
	}

	raw, err := c.doGQLGenericRequest(ctx, query, variables, "aliased gitattributes")
	if err != nil {
		return nil, err
	}

	var result gqlAliasedRawResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("gitlab: graphql decode aliased gitattributes response: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("gitlab: graphql aliased gitattributes errors: %s", result.Errors[0].Message)
	}

	if qc, ok := result.Data["queryComplexity"]; ok {
		var complexity QueryComplexity
		if err := json.Unmarshal(qc, &complexity); err == nil {
			c.logger.Debug("gitlab: graphql aliased gitattributes complexity",
				"count", len(reqs), "score", complexity.Score, "limit", complexity.Limit)
		}
	}

	results := make([]GitAttributesResult, len(reqs))
	for i, r := range reqs {
		var entry *gqlAliasedGitAttributesProject
		if rawEntry, ok := result.Data[fmt.Sprintf("ga%d", i)]; ok {
			if err := json.Unmarshal(rawEntry, &entry); err != nil {
				return nil, fmt.Errorf("gitlab: graphql decode gitattributes for %s@%s: %w", r.ProjectFullPath, r.Ref, err)
			}
		}
		if entry == nil || entry.Repository == nil {
			c.logger.Warn("gitlab: graphql aliased gitattributes: project not found",
				"project", r.ProjectFullPath, "ref", r.Ref)
			continue
		}
		results[i].Found = true
		if entry.Repository.Blobs == nil {
			continue
		}
		for _, n := range entry.Repository.Blobs.Nodes {
			if n.RawTextBlob != nil {
				results[i].Exists, results[i].Content = true, *n.RawTextBlob
				break
			}
		}
	}
	c.logger.Debug("gitlab: graphql aliased gitattributes done",
		"count", len(reqs), "duration", ilog.FmtDur(time.Since(start)))
	return results, nil
}
