package github

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := NewClient(Config{Owner: "ceffo", Repo: "mrboard"})
	client.http = srv.Client()
	client.baseURL = srv.URL
	return client
}

func TestGetLatestRelease_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"tag_name":"v0.12.0","html_url":"https://github.com/ceffo/mrboard/releases/tag/v0.12.0"}`))
	})

	rel, err := c.GetLatestRelease(t.Context())

	require.NoError(t, err)
	require.NotNil(t, rel)
	assert.Equal(t, "v0.12.0", rel.TagName)
	assert.Equal(t, "https://github.com/ceffo/mrboard/releases/tag/v0.12.0", rel.HTMLURL)
}

func TestGetLatestRelease_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	rel, err := c.GetLatestRelease(t.Context())

	require.NoError(t, err)
	assert.Nil(t, rel)
}

func TestGetLatestRelease_ServerError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.GetLatestRelease(t.Context())

	assert.Error(t, err)
}

func TestGetLatestRelease_MalformedJSON(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{not json`))
	})

	_, err := c.GetLatestRelease(t.Context())

	assert.Error(t, err)
}
