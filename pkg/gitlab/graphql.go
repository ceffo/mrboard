package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const gqlUserMRsQuery = `
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
        author { username name }
        reviewers { nodes { username name } }
        project { id fullPath archived }
        approvedBy { nodes { username } }
        approvalsRequired
        approvalsLeft
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
	ID        string  `json:"id"`  // "gid://gitlab/MergeRequest/456"
	IID       string  `json:"iid"` // "42"
	Title     string  `json:"title"`
	Draft     bool    `json:"draft"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
	WebURL    string  `json:"webUrl"`
	Author    GQLUser `json:"author"`
	Reviewers struct {
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
	ApprovalsRequired int `json:"approvalsRequired"`
	ApprovalsLeft     int `json:"approvalsLeft"`
	Discussions       struct {
		PageInfo struct {
			HasNextPage bool `json:"hasNextPage"`
		} `json:"pageInfo"`
		Nodes []GQLDiscussion `json:"nodes"`
	} `json:"discussions"`
}

type gqlUserMRsResponse struct {
	Data struct {
		User *struct {
			AuthoredMergeRequests struct {
				Nodes []GQLMergeRequest `json:"nodes"`
			} `json:"authoredMergeRequests"`
		} `json:"user"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// FetchUserMRsGraphQL fetches all open MRs authored by username in a single GraphQL query,
// returning fully-populated MR data including discussions and approvals inline.
func (c *Client) FetchUserMRsGraphQL(ctx context.Context, username string) ([]GQLMergeRequest, error) {
	start := time.Now()
	c.logger.Debug("gitlab: graphql user MRs", "username", username)

	payload := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}{
		Query:     gqlUserMRsQuery,
		Variables: map[string]interface{}{"username": username},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gitlab: graphql marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gitlab: graphql build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		elapsed := time.Since(start).Round(time.Millisecond)
		c.logger.Error("gitlab: graphql request error", "username", username, "duration", elapsed, "error", err)
		return nil, fmt.Errorf("gitlab: graphql request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		elapsed := time.Since(start).Round(time.Millisecond)
		c.logger.Error("gitlab: graphql non-200", "username", username, "status", resp.StatusCode, "duration", elapsed)
		return nil, fmt.Errorf("gitlab: graphql HTTP %d for user %q", resp.StatusCode, username)
	}

	var result gqlUserMRsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("gitlab: graphql decode response: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("gitlab: graphql errors for user %q: %s", username, result.Errors[0].Message)
	}
	if result.Data.User == nil {
		elapsed := time.Since(start).Round(time.Millisecond)
		c.logger.Warn("gitlab: graphql user not found", "username", username, "duration", elapsed)
		return nil, nil
	}

	mrs := result.Data.User.AuthoredMergeRequests.Nodes
	elapsed := time.Since(start).Round(time.Millisecond)
	c.logger.Debug("gitlab: graphql user MRs done", "username", username, "count", len(mrs), "duration", elapsed)
	return mrs, nil
}
