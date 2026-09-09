package githubadpt

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkggithub "github.com/ceffo/mrboard/pkg/github"
)

// fakeClient is a trivial in-memory implementation of releaseClient for tests.
type fakeClient struct {
	release *pkggithub.Release
	err     error
	calls   int
}

func (f *fakeClient) GetLatestRelease(_ context.Context) (*pkggithub.Release, error) {
	f.calls++
	return f.release, f.err
}

const testCacheDir = "/cache"

func newTestAdapter(t *testing.T, client releaseClient, ttl time.Duration) *GitHubAdapter {
	t.Helper()
	a, err := New(client, Config{FS: afero.NewMemMapFs(), CacheDir: testCacheDir, TTL: ttl}, slog.Default())
	require.NoError(t, err)
	return a
}

func TestCheckForUpdate_LiveAndCached(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: "v0.12.0"}}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), "0.11.0")
	require.NoError(t, err)
	assert.True(t, info.Available)
	assert.Equal(t, "v0.12.0", info.Latest)
	assert.Equal(t, 1, fc.calls)

	// second call must hit cache, not the client
	info2, err := a.CheckForUpdate(context.Background(), "0.11.0")
	require.NoError(t, err)
	assert.Equal(t, info, info2)
	assert.Equal(t, 1, fc.calls, "expected cache hit, client should not be called again")
}

func TestCheckForUpdate_CachingDisabled(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: "v0.12.0"}}
	a := newTestAdapter(t, fc, -time.Second) // negative TTL → no caching

	_, err := a.CheckForUpdate(context.Background(), "0.11.0")
	require.NoError(t, err)
	_, err = a.CheckForUpdate(context.Background(), "0.11.0")
	require.NoError(t, err)

	assert.Equal(t, 2, fc.calls)
}

func TestCheckForUpdate_UpToDate(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: "v0.11.0"}}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), "0.11.0")

	require.NoError(t, err)
	assert.False(t, info.Available)
}

func TestCheckForUpdate_NoReleasesPublished(t *testing.T) {
	fc := &fakeClient{release: nil}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), "0.11.0")

	require.NoError(t, err)
	assert.False(t, info.Available)
}

func TestCheckForUpdate_DevBuildNeverCallsClient(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: "v0.12.0"}}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), "dev")

	require.NoError(t, err)
	assert.False(t, info.Available)
	assert.Zero(t, fc.calls)
}

func TestCheckForUpdate_ClientError(t *testing.T) {
	fc := &fakeClient{err: errors.New("boom")}
	a := newTestAdapter(t, fc, time.Hour)

	_, err := a.CheckForUpdate(context.Background(), "0.11.0")

	assert.Error(t, err)
}
