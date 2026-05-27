package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// gqlUserMRsQueryFull includes approvalRules — supported on GitLab.com and some self-managed versions.
const gqlUserMRsQueryFull = `
query($username: String!) {
  user(username: $username) {
    authoredMergeRequests(state: opened, first: 100) {
      nodes {
        id
        iid
        title
        draft
        createdAt
        updatedAt
        webUrl
        detailedMergeStatus
        author { username name }
        reviewers { nodes { username name } }
        project { id fullPath archived }
        approvedBy { nodes { username } }
        approvalRules {
          name
          eligibleApprovers { username }
        }
        discussions(first: 100) {
          pageInfo { hasNextPage }
          nodes {
            notes(first: 100) {
              nodes {
                author { username name }
                body
                system
                resolvable
                resolved
                createdAt
              }
            }
          }
        }
      }
    }
  }
}`

// gqlUserMRsQueryReduced omits approvalRules for GitLab instances that don't support it.
// IsApprover will not be set for MRs fetched via this query.
const gqlUserMRsQueryReduced = `
query($username: String!) {
  user(username: $username) {
    authoredMergeRequests(state: opened, first: 100) {
      nodes {
        id
        iid
        title
        draft
        createdAt
        updatedAt
        webUrl
        detailedMergeStatus
        author { username name }
        reviewers { nodes { username name } }
        project { id fullPath archived }
        approvedBy { nodes { username } }
        discussions(first: 100) {
          pageInfo { hasNextPage }
          nodes {
            notes(first: 100) {
              nodes {
                author { username name }
                body
                system
                resolvable
                resolved
                createdAt
              }
            }
          }
        }
      }
    }
  }
}`

// GQLUser is a GitLab user as returned by the GraphQL API.
type GQLUser struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

// GQLApprovalRule is a single MR approval rule as returned by the GraphQL API.
type GQLApprovalRule struct {
	Name              string    `json:"name"`
	EligibleApprovers []GQLUser `json:"eligibleApprovers"`
}

// GQLNote is a single note within a discussion.
type GQLNote struct {
	Author     GQLUser `json:"author"`
	Body       string  `json:"body"`
	System     bool    `json:"system"`
	Resolvable bool    `json:"resolvable"`
	Resolved   bool    `json:"resolved"`
	CreatedAt  string  `json:"createdAt"` // RFC3339
}

// GQLDiscussion is a discussion thread on an MR.
type GQLDiscussion struct {
	Notes struct {
		Nodes []GQLNote `json:"nodes"`
	} `json:"notes"`
}

// GQLMergeRequest is a merge request as returned by the GraphQL API.
type GQLMergeRequest struct {
	ID                  string  `json:"id"`  // "gid://gitlab/MergeRequest/456"
	IID                 string  `json:"iid"` // "42"
	Title               string  `json:"title"`
	Draft               bool    `json:"draft"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
	WebURL              string  `json:"webUrl"`
	DetailedMergeStatus string  `json:"detailedMergeStatus"`
	Author              GQLUser `json:"author"`
	Reviewers           struct {
		Nodes []GQLUser `json:"nodes"`
	} `json:"reviewers"`
	Project struct {
		ID       string `json:"id"` // "gid://gitlab/Project/123"
		FullPath string `json:"fullPath"`
		Archived bool   `json:"archived"`
	} `json:"project"`
	ApprovedBy struct {
		Nodes []GQLUser `json:"nodes"`
	} `json:"approvedBy"`
	ApprovalRules []GQLApprovalRule `json:"approvalRules"`
	Discussions   struct {
		PageInfo struct {
			HasNextPage bool `json:"hasNextPage"`
		} `json:"pageInfo"`
		Nodes []GQLDiscussion `json:"nodes"`
	} `json:"discussions"`
}

type gqlError struct {
	Message string `json:"message"`
}

type gqlUserMRsResponse struct {
	Data struct {
		User *struct {
			AuthoredMergeRequests struct {
				Nodes []GQLMergeRequest `json:"nodes"`
			} `json:"authoredMergeRequests"`
		} `json:"user"`
	} `json:"data"`
	Errors []gqlError `json:"errors"`
}

// FetchUserMRsGraphQL fetches all open MRs authored by username in a single GraphQL query.
// On the first call it tries the full query including approvalRules. If the server returns
// a field-not-found error for that field, it logs once at INFO level, records the fact, and
// retries with a reduced query. All subsequent calls use the reduced query directly.
func (c *Client) FetchUserMRsGraphQL(ctx context.Context, username string) ([]GQLMergeRequest, error) {
	start := time.Now()
	c.logger.Debug("gitlab: graphql user MRs", "username", username)

	query := gqlUserMRsQueryFull
	if c.gqlApprovalRulesMissing.Load() {
		query = gqlUserMRsQueryReduced
	}

	mrs, gqlErrs, err := c.doGQLUserMRs(ctx, username, query)
	if err != nil {
		c.logger.Error("gitlab: graphql request error", "username", username,
			"duration", time.Since(start).Round(time.Millisecond), "error", err)
		return nil, err
	}

	// Detect unsupported approvalRules field and retry once with the reduced query.
	if len(gqlErrs) > 0 && !c.gqlApprovalRulesMissing.Load() {
		for _, e := range gqlErrs {
			if strings.Contains(e.Message, "approvalRules") {
				c.gqlApprovalRulesMissing.Store(true)
				c.logger.Info("gitlab: approvalRules not supported by this GitLab instance; " +
					"IsApprover will not display for GQL-fetched MRs")
				mrs, gqlErrs, err = c.doGQLUserMRs(ctx, username, gqlUserMRsQueryReduced)
				if err != nil {
					return nil, err
				}
				break
			}
		}
	}
	if len(gqlErrs) > 0 {
		return nil, fmt.Errorf("gitlab: graphql errors for user %q: %s", username, gqlErrs[0].Message)
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	c.logger.Debug("gitlab: graphql user MRs done", "username", username, "count", len(mrs), "duration", elapsed)
	return mrs, nil
}

// doGQLUserMRs executes a GraphQL query and returns the raw MR nodes, any GQL-level errors,
// and any transport/decoding error. It does not interpret GQL errors.
func (c *Client) doGQLUserMRs(
	ctx context.Context, username, query string,
) ([]GQLMergeRequest, []gqlError, error) {
	payload := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}{
		Query:     query,
		Variables: map[string]interface{}{"username": username},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("gitlab: graphql marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("gitlab: graphql build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("gitlab: graphql request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("gitlab: graphql HTTP %d for user %q", resp.StatusCode, username)
	}

	var result gqlUserMRsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("gitlab: graphql decode response: %w", err)
	}
	if result.Data.User == nil && len(result.Errors) == 0 {
		c.logger.Warn("gitlab: graphql user not found", "username", username)
		return nil, nil, nil
	}
	if result.Data.User == nil {
		return nil, result.Errors, nil
	}
	return result.Data.User.AuthoredMergeRequests.Nodes, result.Errors, nil
}
