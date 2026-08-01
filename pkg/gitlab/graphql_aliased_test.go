package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient builds a Client pointed at srv, bypassing NewClient's
// go-gitlab wiring — FetchMRsDiscussionsGraphQL only uses apiURL, token, and
// httpClient.
func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		logger:     discardLogger(),
		token:      "test-token",
		apiURL:     srv.URL,
		httpClient: srv.Client(),
	}
}

func TestFetchMRsDiscussionsGraphQL_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		assert.Fail(t, "no request must be sent for an empty batch")
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.FetchMRsDiscussionsGraphQL(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, results)
}

// TestFetchMRsDiscussionsGraphQL_AliasedBatch verifies the request sent for N
// MRs is a single query with one alias and one variable pair per MR, and that
// the response is decoded back into results aligned by index — the core of
// phase 2's "one round trip for N MRs" (docs/adr/0005).
func TestFetchMRsDiscussionsGraphQL_AliasedBatch(t *testing.T) {
	var capturedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		assert.Equal(t, "test-token", r.Header.Get("PRIVATE-TOKEN"))

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{
			"data": {
				"mr0": {"mergeRequest": {"discussions": {"pageInfo": {"hasNextPage": false},
					"nodes": [{"notes": {"nodes": [{"author": {"username": "alice"}, "body": "hi",
						"system": false, "resolvable": true, "resolved": false, "createdAt": "2026-07-30T00:00:00Z"}]}}]}}},
				"mr1": {"mergeRequest": {"discussions": {"pageInfo": {"hasNextPage": true}, "nodes": []}}}
			}
		}`))
		require.NoError(t, err)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	reqs := []MRDiscussionsRequest{
		{ProjectFullPath: "group/one", IID: "1"},
		{ProjectFullPath: "group/two", IID: "2"},
	}
	results, err := c.FetchMRsDiscussionsGraphQL(context.Background(), reqs)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.False(t, results[0].HasNextPage)
	require.Len(t, results[0].Discussions, 1)
	assert.Equal(t, "alice", results[0].Discussions[0].Notes.Nodes[0].Author.Username)

	assert.True(t, results[1].HasNextPage)
	assert.Empty(t, results[1].Discussions)

	variables, ok := capturedBody["variables"].(map[string]interface{})
	require.True(t, ok, "request body must carry a variables object")
	assert.Equal(t, "group/one", variables["p0"])
	assert.Equal(t, "1", variables["i0"])
	assert.Equal(t, "group/two", variables["p1"])
	assert.Equal(t, "2", variables["i1"])

	query, ok := capturedBody["query"].(string)
	require.True(t, ok, "request body must carry a query string")
	assert.Contains(t, query, "mr0: project(fullPath: $p0)")
	assert.Contains(t, query, "mr1: project(fullPath: $p1)")
}

// TestFetchMRsDiscussionsGraphQL_NotFound verifies an MR GitLab can't find at
// its alias yields a zero-value result at that index rather than an error —
// the batched equivalent of the old single-MR "not found" behavior.
func TestFetchMRsDiscussionsGraphQL_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"data": {"mr0": null}}`))
		require.NoError(t, err)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.FetchMRsDiscussionsGraphQL(
		context.Background(), []MRDiscussionsRequest{{ProjectFullPath: "group/gone", IID: "9"}})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, MRDiscussionsResult{}, results[0])
}

func TestFetchMRsDiscussionsGraphQL_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"errors": [{"message": "boom"}]}`))
		require.NoError(t, err)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.FetchMRsDiscussionsGraphQL(
		context.Background(), []MRDiscussionsRequest{{ProjectFullPath: "group/one", IID: "1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}
