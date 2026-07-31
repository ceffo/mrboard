package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	ilog "github.com/ceffo/mrboard/internal/log"
)

// gqlUserMRsQuery fetches open MRs authored by a user, including approval rules
// via approvalState.rules (supported on GitLab self-managed and GitLab.com).
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
            eligibleApprovers { username }
          }
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

// gqlUserMRsThinQuery is gqlUserMRsQuery minus the discussions block — the entire
// cost of the fat query (up to 100 discussions x 100 notes with bodies per MR).
// approvalState.rules stays in the thin query deliberately: SaveApprovers writes a
// separate GitLab resource that almost certainly does not bump the MR's updatedAt,
// so dropping it here would make a teammate's approver edit in the web UI invisible
// to an updatedAt-keyed cache (see docs/adr/0005). resolvedDiscussionsCount and
// resolvableDiscussionsCount are cheap scalars added for a future cache-diffing phase.
const gqlUserMRsThinQuery = `
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
            eligibleApprovers { username }
          }
        }
        resolvedDiscussionsCount
        resolvableDiscussionsCount
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
	SourceBranch        string  `json:"sourceBranch"`
	TargetBranch        string  `json:"targetBranch"`
	Author              GQLUser `json:"author"`
	Assignees           struct {
		Nodes []GQLUser `json:"nodes"`
	} `json:"assignees"`
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
	ApprovalState struct {
		Rules []GQLApprovalRule `json:"rules"`
	} `json:"approvalState"`
	// ResolvedDiscussionsCount and ResolvableDiscussionsCount are cheap scalars
	// (no note bodies) requested by the thin query only; a future incremental-fetch
	// phase uses them as a second cache-invalidation signal alongside updatedAt.
	ResolvedDiscussionsCount   int `json:"resolvedDiscussionsCount"`
	ResolvableDiscussionsCount int `json:"resolvableDiscussionsCount"`
	Discussions                struct {
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
// FetchUserMRsGraphQL fetches all open MRs authored by username in a single GraphQL query.
func (c *Client) FetchUserMRsGraphQL(ctx context.Context, username string) ([]GQLMergeRequest, error) {
	start := time.Now()
	c.logger.Debug("gitlab: graphql user MRs", "username", username)

	mrs, gqlErrs, err := c.doGQLUserMRs(ctx, username, gqlUserMRsQuery)
	if err != nil {
		c.logger.Error("gitlab: graphql request error", "username", username,
			"duration", ilog.FmtDur(time.Since(start)), "error", err)
		return nil, err
	}
	if len(gqlErrs) > 0 {
		return nil, fmt.Errorf("gitlab: graphql errors for user %q: %s", username, gqlErrs[0].Message)
	}

	elapsed := ilog.FmtDur(time.Since(start))
	c.logger.Debug("gitlab: graphql user MRs done", "username", username, "count", len(mrs), "duration", elapsed)
	return mrs, nil
}

// FetchUserMRsThinGraphQL fetches open MRs authored by username via the thin
// GraphQL query (no discussions) used for phase-1 listing.
func (c *Client) FetchUserMRsThinGraphQL(ctx context.Context, username string) ([]GQLMergeRequest, error) {
	start := time.Now()
	c.logger.Debug("gitlab: graphql user MRs (thin)", "username", username)

	mrs, gqlErrs, err := c.doGQLUserMRs(ctx, username, gqlUserMRsThinQuery)
	if err != nil {
		c.logger.Error("gitlab: graphql request error", "username", username,
			"duration", ilog.FmtDur(time.Since(start)), "error", err)
		return nil, err
	}
	if len(gqlErrs) > 0 {
		return nil, fmt.Errorf("gitlab: graphql errors for user %q: %s", username, gqlErrs[0].Message)
	}

	elapsed := ilog.FmtDur(time.Since(start))
	c.logger.Debug("gitlab: graphql user MRs (thin) done", "username", username, "count", len(mrs), "duration", elapsed)
	return mrs, nil
}

// gqlReviewerMRsQuery fetches open MRs where the user is a requested reviewer.
const gqlReviewerMRsQuery = `
query($username: String!) {
  user(username: $username) {
    reviewRequestedMergeRequests(state: opened, first: 100) {
      nodes {
        id
        iid
        title
        draft
        createdAt
        updatedAt
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
            eligibleApprovers { username }
          }
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

type gqlReviewerMRsResponse struct {
	Data struct {
		User *struct {
			ReviewRequestedMergeRequests struct {
				Nodes []GQLMergeRequest `json:"nodes"`
			} `json:"reviewRequestedMergeRequests"`
		} `json:"user"`
	} `json:"data"`
	Errors []gqlError `json:"errors"`
}

// gqlReviewerMRsThinQuery is gqlReviewerMRsQuery minus the discussions block.
// See gqlUserMRsThinQuery for why approvalState.rules and the discussion-count
// scalars are kept.
const gqlReviewerMRsThinQuery = `
query($username: String!) {
  user(username: $username) {
    reviewRequestedMergeRequests(state: opened, first: 100) {
      nodes {
        id
        iid
        title
        draft
        createdAt
        updatedAt
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
            eligibleApprovers { username }
          }
        }
        resolvedDiscussionsCount
        resolvableDiscussionsCount
      }
    }
  }
}`

// FetchReviewerMRsGraphQL fetches all open MRs where username is a requested reviewer.
func (c *Client) FetchReviewerMRsGraphQL(ctx context.Context, username string) ([]GQLMergeRequest, error) {
	start := time.Now()
	c.logger.Debug("gitlab: graphql reviewer MRs", "username", username)

	mrs, gqlErrs, err := c.doGQLReviewerMRs(ctx, username, gqlReviewerMRsQuery)
	if err != nil {
		return nil, err
	}
	if len(gqlErrs) > 0 {
		return nil, fmt.Errorf("gitlab: graphql reviewer MRs errors for user %q: %s", username, gqlErrs[0].Message)
	}

	c.logger.Debug("gitlab: graphql reviewer MRs done",
		"username", username, "count", len(mrs), "duration", ilog.FmtDur(time.Since(start)))
	return mrs, nil
}

// FetchReviewerMRsThinGraphQL fetches open MRs where username is a requested
// reviewer via the thin GraphQL query (no discussions) used for phase-1 listing.
func (c *Client) FetchReviewerMRsThinGraphQL(ctx context.Context, username string) ([]GQLMergeRequest, error) {
	start := time.Now()
	c.logger.Debug("gitlab: graphql reviewer MRs (thin)", "username", username)

	mrs, gqlErrs, err := c.doGQLReviewerMRs(ctx, username, gqlReviewerMRsThinQuery)
	if err != nil {
		return nil, err
	}
	if len(gqlErrs) > 0 {
		return nil, fmt.Errorf("gitlab: graphql reviewer MRs errors for user %q: %s", username, gqlErrs[0].Message)
	}

	c.logger.Debug("gitlab: graphql reviewer MRs (thin) done",
		"username", username, "count", len(mrs), "duration", ilog.FmtDur(time.Since(start)))
	return mrs, nil
}

// doGQLReviewerMRs executes a reviewer-requested-MRs GraphQL query and returns the
// raw MR nodes, any GQL-level errors, and any transport/decoding error. It does not
// interpret GQL errors. Mirrors doGQLUserMRs for the reviewRequestedMergeRequests shape.
func (c *Client) doGQLReviewerMRs(
	ctx context.Context, username, query string,
) ([]GQLMergeRequest, []gqlError, error) {
	raw, err := c.doGQLRequest(ctx, query, username, "reviewer MRs")
	if err != nil {
		return nil, nil, err
	}

	var result gqlReviewerMRsResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, nil, fmt.Errorf("gitlab: graphql decode reviewer MRs response: %w", err)
	}
	if result.Data.User == nil && len(result.Errors) == 0 {
		c.logger.Warn("gitlab: graphql user not found for reviewer MRs", "username", username)
		return nil, nil, nil
	}
	if result.Data.User == nil {
		return nil, result.Errors, nil
	}
	return result.Data.User.ReviewRequestedMergeRequests.Nodes, result.Errors, nil
}

// doGQLUserMRs executes a GraphQL query and returns the raw MR nodes, any GQL-level errors,
// and any transport/decoding error. It does not interpret GQL errors.
func (c *Client) doGQLUserMRs(
	ctx context.Context, username, query string,
) ([]GQLMergeRequest, []gqlError, error) {
	raw, err := c.doGQLRequest(ctx, query, username, "request")
	if err != nil {
		return nil, nil, err
	}

	var result gqlUserMRsResponse
	if err := json.Unmarshal(raw, &result); err != nil {
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

// doGQLRequest posts a GraphQL query with a single "username" variable and
// returns the raw response body. label distinguishes error messages between
// the user-MRs and reviewer-MRs callers. Shared by doGQLUserMRs and
// doGQLReviewerMRs, which differ only in which response shape they decode.
func (c *Client) doGQLRequest(ctx context.Context, query, username, label string) ([]byte, error) {
	payload := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}{
		Query:     query,
		Variables: map[string]interface{}{"username": username},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gitlab: graphql marshal %s: %w", label, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gitlab: graphql build %s: %w", label, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: graphql %s: %w", label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab: graphql %s HTTP %d for user %q", label, resp.StatusCode, username)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gitlab: graphql read %s response: %w", label, err)
	}
	return respBody, nil
}

// gqlMRDiscussionsQuery fetches only the discussions block for a single MR,
// identified by project full path and IID. Used to enrich a phase-1 thin result
// after early dedup, one MR at a time (see docs/adr/0005, "Two-phase conditional
// fetch" — phase 2's aliased multi-MR variant of this query is a separate ticket).
const gqlMRDiscussionsQuery = `
query($fullPath: ID!, $iid: String!) {
  project(fullPath: $fullPath) {
    mergeRequest(iid: $iid) {
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
}`

type gqlMRDiscussionsResponse struct {
	Data struct {
		Project *struct {
			MergeRequest *struct {
				Discussions struct {
					PageInfo struct {
						HasNextPage bool `json:"hasNextPage"`
					} `json:"pageInfo"`
					Nodes []GQLDiscussion `json:"nodes"`
				} `json:"discussions"`
			} `json:"mergeRequest"`
		} `json:"project"`
	} `json:"data"`
	Errors []gqlError `json:"errors"`
}

// FetchMRDiscussionsGraphQL fetches the discussion threads for a single MR,
// identified by project full path and IID. The bool return reports whether the
// discussions connection has more pages (thread count may be incomplete).
func (c *Client) FetchMRDiscussionsGraphQL(
	ctx context.Context, projectFullPath, iid string,
) ([]GQLDiscussion, bool, error) {
	start := time.Now()
	c.logger.Debug("gitlab: graphql MR discussions", "project", projectFullPath, "iid", iid)

	payload := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}{
		Query:     gqlMRDiscussionsQuery,
		Variables: map[string]interface{}{"fullPath": projectFullPath, "iid": iid},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, false, fmt.Errorf("gitlab: graphql marshal MR discussions request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("gitlab: graphql build MR discussions request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("gitlab: graphql MR discussions request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("gitlab: graphql MR discussions HTTP %d for project %q iid %q",
			resp.StatusCode, projectFullPath, iid)
	}

	var result gqlMRDiscussionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, false, fmt.Errorf("gitlab: graphql decode MR discussions response: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, false, fmt.Errorf("gitlab: graphql MR discussions errors for project %q iid %q: %s",
			projectFullPath, iid, result.Errors[0].Message)
	}
	if result.Data.Project == nil || result.Data.Project.MergeRequest == nil {
		c.logger.Warn("gitlab: graphql MR discussions: MR not found", "project", projectFullPath, "iid", iid)
		return nil, false, nil
	}

	discussions := result.Data.Project.MergeRequest.Discussions.Nodes
	hasNextPage := result.Data.Project.MergeRequest.Discussions.PageInfo.HasNextPage
	c.logger.Debug("gitlab: graphql MR discussions done", "project", projectFullPath, "iid", iid,
		"discussions", len(discussions), "duration", ilog.FmtDur(time.Since(start)))
	return discussions, hasNextPage, nil
}
