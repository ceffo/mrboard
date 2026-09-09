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

	"github.com/ceffo/mrboard/internal/domain/service/updatesvc"
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

const (
	testCacheDir       = "/cache"
	testCurrentVersion = "0.11.0"
	testLatestTag      = "v0.12.0"
)

func newTestAdapter(t *testing.T, client releaseClient, ttl time.Duration) *GitHubAdapter {
	t.Helper()
	a, err := New(client, Config{FS: afero.NewMemMapFs(), CacheDir: testCacheDir, TTL: ttl}, slog.Default())
	require.NoError(t, err)
	return a
}

func TestCheckForUpdate_LiveAndCached(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: testLatestTag}}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), testCurrentVersion, updatesvc.CheckOptions{})
	require.NoError(t, err)
	assert.True(t, info.Available)
	assert.Equal(t, testLatestTag, info.Latest)
	assert.Equal(t, 1, fc.calls)

	// second call must hit cache, not the client
	info2, err := a.CheckForUpdate(context.Background(), testCurrentVersion, updatesvc.CheckOptions{})
	require.NoError(t, err)
	assert.Equal(t, info, info2)
	assert.Equal(t, 1, fc.calls, "expected cache hit, client should not be called again")
}

func TestCheckForUpdate_ForceBypassesCache(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: testLatestTag}}
	a := newTestAdapter(t, fc, time.Hour)

	_, err := a.CheckForUpdate(context.Background(), testCurrentVersion, updatesvc.CheckOptions{})
	require.NoError(t, err)

	fc.release = &pkggithub.Release{TagName: "v0.13.0"}
	info, err := a.CheckForUpdate(context.Background(), testCurrentVersion, updatesvc.CheckOptions{Force: true})

	require.NoError(t, err)
	assert.Equal(t, 2, fc.calls, "want Force to skip the still-valid cache entry")
	assert.Equal(t, "v0.13.0", info.Latest)
}

func TestCheckForUpdate_UnreadableTagIsNotNewer(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: "nightly"}}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), testCurrentVersion, updatesvc.CheckOptions{})

	require.NoError(t, err)
	assert.False(t, info.Available)
}

func TestCheckForUpdate_PrereleaseIsOlderThanRelease(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: "v0.12.0-rc.1"}}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), "0.12.0", updatesvc.CheckOptions{})

	require.NoError(t, err)
	assert.False(t, info.Available, "want a prerelease to rank below the matching release")
}

func TestCheckForUpdate_CachingDisabled(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: testLatestTag}}
	a := newTestAdapter(t, fc, -time.Second) // negative TTL → no caching

	_, err := a.CheckForUpdate(context.Background(), testCurrentVersion, updatesvc.CheckOptions{})
	require.NoError(t, err)
	_, err = a.CheckForUpdate(context.Background(), testCurrentVersion, updatesvc.CheckOptions{})
	require.NoError(t, err)

	assert.Equal(t, 2, fc.calls)
}

func TestCheckForUpdate_UpToDate(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: "v0.11.0"}}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), testCurrentVersion, updatesvc.CheckOptions{})

	require.NoError(t, err)
	assert.False(t, info.Available)
}

func TestCheckForUpdate_NoReleasesPublished(t *testing.T) {
	fc := &fakeClient{release: nil}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), testCurrentVersion, updatesvc.CheckOptions{})

	require.NoError(t, err)
	assert.False(t, info.Available)
}

func TestCheckForUpdate_DevBuildNeverCallsClient(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: testLatestTag}}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), "dev", updatesvc.CheckOptions{})

	require.NoError(t, err)
	assert.False(t, info.Available)
	assert.Zero(t, fc.calls)
}

// TestCheckForUpdate_GitDescribeBuildNeverCallsClient covers the version
// `just build` stamps once the checkout has moved past its tag: it parses as
// semver but is not a published release, so it must behave like "dev".
func TestCheckForUpdate_GitDescribeBuildNeverCallsClient(t *testing.T) {
	fc := &fakeClient{release: &pkggithub.Release{TagName: testLatestTag}}
	a := newTestAdapter(t, fc, time.Hour)

	info, err := a.CheckForUpdate(context.Background(), "v0.11.0-3-gabc1234-dirty", updatesvc.CheckOptions{})

	require.NoError(t, err)
	assert.False(t, info.Available)
	assert.Zero(t, fc.calls)
}

func TestCheckForUpdate_ClientError(t *testing.T) {
	fc := &fakeClient{err: errors.New("boom")}
	a := newTestAdapter(t, fc, time.Hour)

	_, err := a.CheckForUpdate(context.Background(), testCurrentVersion, updatesvc.CheckOptions{})

	assert.Error(t, err)
}
