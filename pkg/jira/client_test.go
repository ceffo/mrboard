package jira

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestGetActiveSprint_Dates(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantNil   bool
		wantStart time.Time
		wantEnd   time.Time
		wantErr   bool
	}{
		{
			name: "dates parsed",
			body: `{"values":[{"id":3,"name":"S","startDate":"2026-10-05T08:00:00.000Z",` +
				`"endDate":"2026-10-19T08:00:00.000+02:00"}]}`,
			wantStart: time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC),
		},
		{name: "missing dates are zero", body: `{"values":[{"id":3,"name":"S"}]}`},
		{name: "no active sprint", body: `{"values":[]}`, wantNil: true},
		{name: "malformed date", body: `{"values":[{"id":3,"name":"S","startDate":"yesterday"}]}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(tc.body))
			})
			got, err := c.GetActiveSprint(t.Context(), 1)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.True(t, tc.wantStart.Equal(got.Start), "start %v", got.Start)
			assert.True(t, tc.wantEnd.Equal(got.End), "end %v", got.End)
		})
	}
}
