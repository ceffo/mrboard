package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testMergedAfter = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

type gqlCapture struct {
	mu     sync.Mutex
	bodies []map[string]interface{}
}

func (c *gqlCapture) record(t *testing.T, r *http.Request) map[string]interface{} {
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, body)
	return body
}

func writeJSON(t *testing.T, w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(s))
	require.NoError(t, err)
}

// TestFetchUserMergedMRsGraphQL_Paginates checks the listing asks for merged MRs
// after the bound with the thin fields plus mergedAt, and follows the cursor
// until GitLab reports no next page.
func TestFetchUserMergedMRsGraphQL_Paginates(t *testing.T) {
	capture := &gqlCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := capture.record(t, r)
		vars, _ := body["variables"].(map[string]interface{})
		if vars["after"] == nil {
			writeJSON(t, w, `{"data": {"owner": {"mrs": {
				"pageInfo": {"hasNextPage": true, "endCursor": "c1"},
				"nodes": [{"iid": "1", "mergedAt": "2026-10-13T09:00:00Z"}]}}}}`)
			return
		}
		writeJSON(t, w, `{"data": {"owner": {"mrs": {
			"pageInfo": {"hasNextPage": false, "endCursor": "c2"},
			"nodes": [{"iid": "2", "mergedAt": "2026-10-14T09:00:00Z"}]}}}}`)
	}))
	defer srv.Close()

	mrs, err := newTestClient(srv).FetchUserMergedMRsGraphQL(context.Background(), "priya", testMergedAfter)

	require.NoError(t, err)
	require.Len(t, mrs, 2)
	assert.Equal(t, "2026-10-13T09:00:00Z", mrs[0].MergedAt)
	assert.Equal(t, "2", mrs[1].IID)

	require.Len(t, capture.bodies, 2)
	query, _ := capture.bodies[0]["query"].(string)
	assert.Contains(t, query, "user(username: $username)")
	assert.Contains(t, query,
		"authoredMergeRequests(state: merged, mergedAfter: $mergedAfter, first: 100, after: $after)")
	for _, field := range []string{"mergedAt", "diffHeadSha", "approvedBy", "approvalState", "reviewers"} {
		assert.Contains(t, query, field)
	}
	assert.NotContains(t, query, "discussions(", "the merged listing is thin")
	vars0, _ := capture.bodies[0]["variables"].(map[string]interface{})
	assert.Equal(t, "priya", vars0["username"])
	assert.Equal(t, "2026-10-12T00:00:00Z", vars0["mergedAfter"])
	vars1, _ := capture.bodies[1]["variables"].(map[string]interface{})
	assert.Equal(t, "c1", vars1["after"])
}

func TestFetchReviewerMergedMRsGraphQL_UsesReviewerConnection(t *testing.T) {
	capture := &gqlCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.record(t, r)
		writeJSON(t, w, `{"data": {"owner": {"mrs": {"pageInfo": {"hasNextPage": false}, "nodes": []}}}}`)
	}))
	defer srv.Close()

	mrs, err := newTestClient(srv).FetchReviewerMergedMRsGraphQL(context.Background(), "alice", testMergedAfter)

	require.NoError(t, err)
	assert.Empty(t, mrs)
	require.Len(t, capture.bodies, 1)
	query, _ := capture.bodies[0]["query"].(string)
	assert.Contains(t, query, "reviewRequestedMergeRequests(state: merged, mergedAfter: $mergedAfter")
}

// TestFetchGroupMergedMRsGraphQL_ResolvesNumericID checks a numeric group ID is
// resolved to its full path over REST, since GraphQL addresses groups by path,
// and that subgroups are included as in the open group listing.
func TestFetchGroupMergedMRsGraphQL_ResolvesNumericID(t *testing.T) {
	capture := &gqlCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v4/groups/4815") {
			assert.Equal(t, "false", r.URL.Query().Get("with_projects"))
			writeJSON(t, w, `{"id": 4815, "full_path": "acme/team"}`)
			return
		}
		capture.record(t, r)
		writeJSON(t, w, `{"data": {"owner": {"mrs": {"pageInfo": {"hasNextPage": false},
			"nodes": [{"iid": "3", "mergedAt": "2026-10-13T00:00:00Z"}]}}}}`)
	}))
	defer srv.Close()
	c, err := NewClient(Config{URL: srv.URL, Token: "t"}, discardLogger())
	require.NoError(t, err)

	mrs, err := c.FetchGroupMergedMRsGraphQL(context.Background(), "4815", testMergedAfter)

	require.NoError(t, err)
	assert.Len(t, mrs, 1)
	require.Len(t, capture.bodies, 1)
	query, _ := capture.bodies[0]["query"].(string)
	assert.Contains(t, query, "group(fullPath: $fullPath)")
	assert.Contains(t, query, "mergeRequests(state: merged, mergedAfter: $mergedAfter, first: 100, "+
		"after: $after, includeSubgroups: true)")
	vars, _ := capture.bodies[0]["variables"].(map[string]interface{})
	assert.Equal(t, "acme/team", vars["fullPath"])
}

func TestFetchGroupMergedMRsGraphQL_PathUsedAsIs(t *testing.T) {
	capture := &gqlCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.record(t, r)
		writeJSON(t, w, `{"data": {"owner": null}}`)
	}))
	defer srv.Close()

	mrs, err := newTestClient(srv).FetchGroupMergedMRsGraphQL(context.Background(), "acme/team", testMergedAfter)

	require.NoError(t, err, "an owner GitLab does not find yields no MRs, as the open listings do")
	assert.Empty(t, mrs)
	require.Len(t, capture.bodies, 1)
	vars, _ := capture.bodies[0]["variables"].(map[string]interface{})
	assert.Equal(t, "acme/team", vars["fullPath"])
}

func TestFetchMergedList_GraphQLErrorFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, `{"data": {"owner": null}, "errors": [{"message": "Field 'mergedAfter' doesn't exist"}]}`)
	}))
	defer srv.Close()

	_, err := newTestClient(srv).FetchUserMergedMRsGraphQL(context.Background(), "priya", testMergedAfter)

	assert.ErrorContains(t, err, "mergedAfter")
}

func TestFetchMergedList_StopsAfterMaxPages(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeJSON(t, w, `{"data": {"owner": {"mrs": {"pageInfo": {"hasNextPage": true, "endCursor": "again"},
			"nodes": []}}}}`)
	}))
	defer srv.Close()

	_, err := newTestClient(srv).FetchUserMergedMRsGraphQL(context.Background(), "priya", testMergedAfter)

	require.Error(t, err)
	assert.Equal(t, int32(mergedListMaxPages), calls.Load())
}
