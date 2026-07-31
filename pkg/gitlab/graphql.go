package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// MRDiscussionsRequest identifies one MR within an aliased batch discussions query.
type MRDiscussionsRequest struct {
	ProjectFullPath string
	IID             string
}

// MRDiscussionsResult is one MR's discussions payload from an aliased batch
// query. A zero value means GitLab reported the MR as not found at that index.
type MRDiscussionsResult struct {
	Discussions []GQLDiscussion
	HasNextPage bool
}

// mrDiscussionsFields is the discussions selection set shared by every aliased
// entry in buildAliasedMRDiscussionsQuery.
const mrDiscussionsFields = `discussions(first: 100) {
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
      }`

// buildAliasedMRDiscussionsQuery builds a single GraphQL document fetching
// discussions for n MRs in one round trip: mr0: project(fullPath: $p0) {
// mergeRequest(iid: $i0) { discussions {...} } } mr1: ... — phase 2 of the
// two-phase fetch (docs/adr/0005, "Two-phase conditional fetch"). Each MR gets
// its own variable pair so project/IID values never need string-escaping into
// the query text.
func buildAliasedMRDiscussionsQuery(n int) string {
	params := make([]string, n)
	aliases := make([]string, n)
	for i := 0; i < n; i++ {
		params[i] = fmt.Sprintf("$p%d: ID!, $i%d: String!", i, i)
		aliases[i] = fmt.Sprintf("  mr%d: project(fullPath: $p%d) {\n    mergeRequest(iid: $i%d) {\n      %s\n    }\n  }",
			i, i, i, mrDiscussionsFields)
	}
	return fmt.Sprintf("query(%s) {\n%s\n}", strings.Join(params, ", "), strings.Join(aliases, "\n"))
}

// gqlAliasedMRProject is the per-alias response shape for the aliased
// discussions batch query — one project(fullPath:){mergeRequest(iid:){...}}.
type gqlAliasedMRProject struct {
	MergeRequest *struct {
		Discussions struct {
			PageInfo struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
			Nodes []GQLDiscussion `json:"nodes"`
		} `json:"discussions"`
	} `json:"mergeRequest"`
}

type gqlAliasedMRsResponse struct {
	Data   map[string]*gqlAliasedMRProject `json:"data"`
	Errors []gqlError                      `json:"errors"`
}

// FetchMRsDiscussionsGraphQL fetches discussions for len(reqs) MRs in a single
// aliased GraphQL request instead of one request per MR (docs/adr/0005,
// "Two-phase conditional fetch"). Results are aligned by index with reqs; an MR
// GitLab reports as not found (deleted, moved, or access revoked since phase 1)
// yields a zero-value result at its index rather than an error.
func (c *Client) FetchMRsDiscussionsGraphQL(
	ctx context.Context, reqs []MRDiscussionsRequest,
) ([]MRDiscussionsResult, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	start := time.Now()
	c.logger.Debug("gitlab: graphql aliased MR discussions", "count", len(reqs))

	const varsPerMR = 2 // one project-path variable and one IID variable per aliased MR
	query := buildAliasedMRDiscussionsQuery(len(reqs))
	variables := make(map[string]interface{}, len(reqs)*varsPerMR)
	for i, r := range reqs {
		variables[fmt.Sprintf("p%d", i)] = r.ProjectFullPath
		variables[fmt.Sprintf("i%d", i)] = r.IID
	}

	raw, err := c.doGQLGenericRequest(ctx, query, variables, "aliased MR discussions")
	if err != nil {
		return nil, err
	}

	var result gqlAliasedMRsResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("gitlab: graphql decode aliased MR discussions response: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("gitlab: graphql aliased MR discussions errors: %s", result.Errors[0].Message)
	}

	results := make([]MRDiscussionsResult, len(reqs))
	for i, r := range reqs {
		entry := result.Data[fmt.Sprintf("mr%d", i)]
		if entry == nil || entry.MergeRequest == nil {
			c.logger.Warn("gitlab: graphql aliased MR discussions: MR not found",
				"project", r.ProjectFullPath, "iid", r.IID)
			continue
		}
		results[i] = MRDiscussionsResult{
			Discussions: entry.MergeRequest.Discussions.Nodes,
			HasNextPage: entry.MergeRequest.Discussions.PageInfo.HasNextPage,
		}
	}
	c.logger.Debug("gitlab: graphql aliased MR discussions done",
		"count", len(reqs), "duration", ilog.FmtDur(time.Since(start)))
	return results, nil
}

// doGQLGenericRequest posts an arbitrary GraphQL query+variables payload and
// returns the raw response body. Unlike doGQLRequest, which always sends a
// single "username" variable, this accepts any variable set — needed for the
// aliased multi-MR discussions query, which has one variable pair per MR.
func (c *Client) doGQLGenericRequest(
	ctx context.Context, query string, variables map[string]interface{}, label string,
) ([]byte, error) {
	payload := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}{Query: query, Variables: variables}
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
		return nil, fmt.Errorf("gitlab: graphql %s HTTP %d", label, resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gitlab: graphql read %s response: %w", label, err)
	}
	return respBody, nil
}
