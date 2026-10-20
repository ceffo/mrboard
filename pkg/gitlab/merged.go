package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	ilog "github.com/ceffo/mrboard/internal/log"
)

// mergedListMaxPages bounds the pages one merged-since listing reads, so a
// cursor GitLab keeps reporting as unfinished cannot loop forever.
const mergedListMaxPages = 50

// gqlMergedMRFields is the thin listing's selection set (see gqlUserMRsThinQuery)
// plus mergedAt. A merged MR needs nothing more: reviewers, approvedBy and the
// approval rules decide its review load, and it never gets phase-2 discussions.
const gqlMergedMRFields = `
        id
        iid
        title
        draft
        createdAt
        updatedAt
        mergedAt
        webUrl
        detailedMergeStatus
        sourceBranch
        targetBranch
        author { username name }
        assignees { nodes { username name } }
        reviewers { nodes { username name } }
        project { id fullPath archived }
        approvedBy { nodes { username } }
        approvalState {
          rules {
            name
            type
            eligibleApprovers { username }
          }
        }
        resolvedDiscussionsCount
        resolvableDiscussionsCount
        diffHeadSha`

// mergedListQuery builds a paginated merged-since listing. owner is the root
// field with its argument, connection the MR connection field and extraArgs any
// arguments it takes beyond the shared ones. Aliasing both (owner, mrs) gives
// every listing one response shape.
func mergedListQuery(ownerVar, ownerType, owner, connection, extraArgs string) string {
	return fmt.Sprintf(`
query($%s: %s, $mergedAfter: Time!, $after: String) {
  owner: %s {
    mrs: %s(state: merged, mergedAfter: $mergedAfter, first: 100, after: $after%s) {
      pageInfo { hasNextPage endCursor }
      nodes {%s
      }
    }
  }
}`, ownerVar, ownerType, owner, connection, extraArgs, gqlMergedMRFields)
}

var (
	gqlUserMergedMRsQuery = mergedListQuery("username", "String!",
		"user(username: $username)", "authoredMergeRequests", "")
	gqlReviewerMergedMRsQuery = mergedListQuery("username", "String!",
		"user(username: $username)", "reviewRequestedMergeRequests", "")
	gqlGroupMergedMRsQuery = mergedListQuery("fullPath", "ID!",
		"group(fullPath: $fullPath)", "mergeRequests", ", includeSubgroups: true")
)

type gqlMergedListResponse struct {
	Data struct {
		Owner *struct {
			MRs struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []GQLMergeRequest `json:"nodes"`
			} `json:"mrs"`
		} `json:"owner"`
	} `json:"data"`
	Errors []gqlError `json:"errors"`
}

// FetchUserMergedMRsGraphQL lists the MRs authored by username and merged at or
// after mergedAfter, with the thin listing's fields plus mergedAt.
func (c *Client) FetchUserMergedMRsGraphQL(
	ctx context.Context, username string, mergedAfter time.Time,
) ([]GQLMergeRequest, error) {
	return c.fetchMergedList(ctx, gqlUserMergedMRsQuery, "username", username, "user merged MRs", mergedAfter)
}

// FetchReviewerMergedMRsGraphQL lists the MRs username was a requested reviewer
// on and that merged at or after mergedAfter.
func (c *Client) FetchReviewerMergedMRsGraphQL(
	ctx context.Context, username string, mergedAfter time.Time,
) ([]GQLMergeRequest, error) {
	return c.fetchMergedList(ctx, gqlReviewerMergedMRsQuery, "username", username, "reviewer merged MRs", mergedAfter)
}

// FetchGroupMergedMRsGraphQL lists the MRs of a group and its subgroups merged
// at or after mergedAfter. groupID is a full path or a numeric ID; a numeric ID
// is resolved to its path first, since GraphQL addresses groups by path only.
func (c *Client) FetchGroupMergedMRsGraphQL(
	ctx context.Context, groupID string, mergedAfter time.Time,
) ([]GQLMergeRequest, error) {
	fullPath := groupID
	if _, err := strconv.ParseInt(groupID, 10, 64); err == nil {
		// API v4 still honours with_projects; false keeps the group's project list
		// out of a lookup that needs only its path.
		opt := &gl.GetGroupOptions{WithProjects: gl.Ptr(false)} //nolint:staticcheck // see above
		g, _, err := c.gl.Groups.GetGroup(groupID, opt, gl.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: resolve group %q path: %w", groupID, err)
		}
		fullPath = g.FullPath
	}
	return c.fetchMergedList(ctx, gqlGroupMergedMRsQuery, "fullPath", fullPath, "group merged MRs", mergedAfter)
}

// fetchMergedList reads every page of a merged-since listing. An owner GitLab
// does not find yields no MRs and no error, as the open listings do.
func (c *Client) fetchMergedList(
	ctx context.Context, query, ownerVar, owner, label string, mergedAfter time.Time,
) ([]GQLMergeRequest, error) {
	start := time.Now()
	c.logger.Debug("gitlab: graphql "+label, ownerVar, owner, "merged_after", mergedAfter)

	var all []GQLMergeRequest
	var after *string
	for page := 0; page < mergedListMaxPages; page++ {
		vars := map[string]interface{}{
			ownerVar:      owner,
			"mergedAfter": mergedAfter.Format(time.RFC3339),
			"after":       after,
		}
		raw, err := c.doGQLGenericRequest(ctx, query, vars, label)
		if err != nil {
			return nil, err
		}
		var result gqlMergedListResponse
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("gitlab: graphql decode %s response: %w", label, err)
		}
		if len(result.Errors) > 0 {
			return nil, fmt.Errorf("gitlab: graphql %s errors for %q: %s", label, owner, result.Errors[0].Message)
		}
		if result.Data.Owner == nil {
			c.logger.Warn("gitlab: graphql "+label+": owner not found", ownerVar, owner)
			return nil, nil
		}
		conn := result.Data.Owner.MRs
		all = append(all, conn.Nodes...)
		if !conn.PageInfo.HasNextPage || conn.PageInfo.EndCursor == "" {
			c.logger.Debug("gitlab: graphql "+label+" done", ownerVar, owner,
				"count", len(all), "pages", page+1, "duration", ilog.FmtDur(time.Since(start)))
			return all, nil
		}
		cursor := conn.PageInfo.EndCursor
		after = &cursor
	}
	return nil, fmt.Errorf("gitlab: graphql %s for %q: more than %d pages", label, owner, mergedListMaxPages)
}
