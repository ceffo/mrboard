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

const testProjectOne = "group/one"

func TestFetchMRsDiffStatsGraphQL_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		assert.Fail(t, "no request must be sent for an empty batch")
	}))
	defer srv.Close()

	results, err := newTestClient(srv).FetchMRsDiffStatsGraphQL(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, results)
}

// TestFetchMRsDiffStatsGraphQL_AliasedBatch checks the request is one aliased
// query with a variable pair per MR plus queryComplexity, and that results keep
// a known empty diff ([]) apart from stats GitLab did not return (null) and from
// an MR it could not find.
func TestFetchMRsDiffStatsGraphQL_AliasedBatch(t *testing.T) {
	var capturedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"data": {
			"queryComplexity": {"score": 31, "limit": 250},
			"mr0": {"mergeRequest": {"diffStats": [{"path": "a.go", "additions": 3, "deletions": 1}]}},
			"mr1": {"mergeRequest": {"diffStats": []}},
			"mr2": {"mergeRequest": {"diffStats": null}},
			"mr3": null
		}}`))
		require.NoError(t, err)
	}))
	defer srv.Close()

	reqs := []MRDiffStatsRequest{
		{ProjectFullPath: testProjectOne, IID: "1"},
		{ProjectFullPath: "group/two", IID: "2"},
		{ProjectFullPath: "group/three", IID: "3"},
		{ProjectFullPath: "group/gone", IID: "4"},
	}
	results, err := newTestClient(srv).FetchMRsDiffStatsGraphQL(context.Background(), reqs)
	require.NoError(t, err)
	require.Len(t, results, 4)

	assert.Equal(t, MRDiffStatsResult{Found: true, Stats: []GQLDiffStat{{Path: "a.go", Additions: 3, Deletions: 1}}},
		results[0])
	assert.True(t, results[1].Found)
	assert.NotNil(t, results[1].Stats, "an empty diff is known, not unknown")
	assert.Empty(t, results[1].Stats)
	assert.True(t, results[2].Found)
	assert.Nil(t, results[2].Stats)
	assert.Equal(t, MRDiffStatsResult{}, results[3])

	variables, ok := capturedBody["variables"].(map[string]interface{})
	require.True(t, ok, "request body must carry a variables object")
	assert.Equal(t, testProjectOne, variables["p0"])
	assert.Equal(t, "4", variables["i3"])

	query, ok := capturedBody["query"].(string)
	require.True(t, ok, "request body must carry a query string")
	assert.Contains(t, query, "queryComplexity { score limit }")
	assert.Contains(t, query, "mr3: project(fullPath: $p3)")
	assert.Contains(t, query, "diffStats { path additions deletions }")
}

func TestFetchMRsDiffStatsGraphQL_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(
			`{"errors": [{"message": "Query has complexity of 300, which exceeds max complexity of 250"}]}`))
		require.NoError(t, err)
	}))
	defer srv.Close()

	_, err := newTestClient(srv).FetchMRsDiffStatsGraphQL(
		context.Background(), []MRDiffStatsRequest{{ProjectFullPath: testProjectOne, IID: "1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "complexity")
}

func TestThinQueries_CarryDiffHeadSHA(t *testing.T) {
	assert.Contains(t, gqlUserMRsThinQuery, "diffHeadSha")
	assert.Contains(t, gqlReviewerMRsThinQuery, "diffHeadSha")

	var mr GQLMergeRequest
	require.NoError(t, json.Unmarshal([]byte(`{"iid": "1", "diffHeadSha": "abc123"}`), &mr))
	assert.Equal(t, "abc123", mr.DiffHeadSHA)
}
