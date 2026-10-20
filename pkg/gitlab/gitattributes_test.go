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

const (
	testAttrsProjectTwo  = "group/attrs-two"
	testAttrsProjectGone = "group/attrs-gone"
	testAttrsRef         = "main"
)

func TestFetchGitAttributesGraphQL_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		assert.Fail(t, "no request must be sent for an empty batch")
	}))
	defer srv.Close()

	results, err := newTestClient(srv).FetchGitAttributesGraphQL(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, results)
}

// TestFetchGitAttributesGraphQL_AliasedBatch checks the request is one aliased
// repository.blobs query with a variable pair per (project, ref) plus
// queryComplexity, and that results tell a file apart from a missing file and
// from a project GitLab could not find.
func TestFetchGitAttributesGraphQL_AliasedBatch(t *testing.T) {
	var capturedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"data": {
			"queryComplexity": {"score": 21, "limit": 250},
			"ga0": {"repository": {"blobs": {"nodes": [{"rawTextBlob": "gen/** linguist-generated\n"}]}}},
			"ga1": {"repository": {"blobs": {"nodes": []}}},
			"ga2": null
		}}`))
		require.NoError(t, err)
	}))
	defer srv.Close()

	reqs := []GitAttributesRequest{
		{ProjectFullPath: testProjectOne, Ref: testAttrsRef},
		{ProjectFullPath: testAttrsProjectTwo, Ref: "develop"},
		{ProjectFullPath: testAttrsProjectGone, Ref: testAttrsRef},
	}
	results, err := newTestClient(srv).FetchGitAttributesGraphQL(context.Background(), reqs)
	require.NoError(t, err)
	require.Len(t, results, 3)

	assert.Equal(t, GitAttributesResult{Found: true, Exists: true, Content: "gen/** linguist-generated\n"}, results[0])
	assert.Equal(t, GitAttributesResult{Found: true}, results[1], "a missing file is found with no content")
	assert.Equal(t, GitAttributesResult{}, results[2])

	variables, ok := capturedBody["variables"].(map[string]interface{})
	require.True(t, ok, "request body must carry a variables object")
	assert.Equal(t, testProjectOne, variables["p0"])
	assert.Equal(t, "develop", variables["r1"])

	query, ok := capturedBody["query"].(string)
	require.True(t, ok, "request body must carry a query string")
	assert.Contains(t, query, "queryComplexity { score limit }")
	assert.Contains(t, query, "ga2: project(fullPath: $p2)")
	assert.Contains(t, query, `blobs(ref: $r2, paths: [".gitattributes"], first: 1) { nodes { rawTextBlob } }`)
}

func TestFetchGitAttributesGraphQL_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"errors": [{"message": "Query has complexity of 300"}]}`))
		require.NoError(t, err)
	}))
	defer srv.Close()

	_, err := newTestClient(srv).FetchGitAttributesGraphQL(
		context.Background(), []GitAttributesRequest{{ProjectFullPath: testProjectOne, Ref: testAttrsRef}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "complexity")
}
