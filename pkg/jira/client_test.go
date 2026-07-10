package jira

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
	return NewClient(Config{InstanceURL: srv.URL, Email: "e", APIToken: "t"})
}

func TestGetRemoteLink_EmptyArray(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`[]`))
	})
	title, err := c.GetRemoteLink(t.Context(), "OD-1", "mrboard:1:1")
	require.NoError(t, err)
	assert.Empty(t, title)
}

func TestGetRemoteLink_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	title, err := c.GetRemoteLink(t.Context(), "OD-1", "mrboard:1:1")
	require.NoError(t, err)
	assert.Empty(t, title)
}

func TestGetRemoteLink_ArrayOfOne(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`[{"object":{"title":"!1 my-mr","url":"https://example.com/1"}}]`))
	})
	title, err := c.GetRemoteLink(t.Context(), "OD-1", "mrboard:1:1")
	require.NoError(t, err)
	assert.Equal(t, "!1 my-mr", title)
}

// Reproduces the production bug: when exactly one remote link matches the
// globalId filter, Jira returns a bare object instead of a one-element array.
func TestGetRemoteLink_SingleObject(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"object":{"title":"!1 my-mr","url":"https://example.com/1"}}`))
	})
	title, err := c.GetRemoteLink(t.Context(), "OD-1", "mrboard:1:1")
	require.NoError(t, err)
	assert.Equal(t, "!1 my-mr", title)
}
