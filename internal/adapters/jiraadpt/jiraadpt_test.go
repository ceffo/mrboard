package jiraadpt

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgjira "github.com/ceffo/mrboard/pkg/jira"
)

const (
	issueKeyOD10 = "OD-10"
	issueKeyOD11 = "OD-11"
)

// fakeClient is a trivial in-memory implementation of jiraClient for tests.
type fakeClient struct {
	issue             *pkgjira.Issue
	issueErr          error
	sprint            *pkgjira.Sprint
	sprintErr         error
	sprintKeys        []string
	sprintKeysErr     error
	calls             int
	remoteLinkTitle   string
	remoteLinkErr     error
	createRemoteCalls int
	createRemoteErr   error
}

func (f *fakeClient) GetIssue(_ context.Context, _ string) (*pkgjira.Issue, error) {
	f.calls++
	return f.issue, f.issueErr
}

func (f *fakeClient) GetActiveSprint(_ context.Context, _ int) (*pkgjira.Sprint, error) {
	f.calls++
	return f.sprint, f.sprintErr
}

func (f *fakeClient) GetSprintIssueKeys(_ context.Context, _ int) ([]string, error) {
	f.calls++
	return f.sprintKeys, f.sprintKeysErr
}

func (f *fakeClient) GetRemoteLink(_ context.Context, _, _ string) (string, error) {
	f.calls++
	return f.remoteLinkTitle, f.remoteLinkErr
}

func (f *fakeClient) CreateOrUpdateRemoteLink(_ context.Context, _ string, _ pkgjira.RemoteLink) error {
	f.createRemoteCalls++
	return f.createRemoteErr
}

func newTestAdapter(t *testing.T, client jiraClient, ttl time.Duration) *JiraAdapter {
	t.Helper()
	dir := t.TempDir()
	return New(client, Config{CacheDir: dir, TTL: ttl}, slog.Default())
}

func TestGetIssueType_LiveAndCached(t *testing.T) {
	fc := &fakeClient{issue: &pkgjira.Issue{Key: "OD-1", Type: "Bug"}}
	a := newTestAdapter(t, fc, time.Hour)

	typ, err := a.GetIssueType(context.Background(), "OD-1")
	require.NoError(t, err)
	assert.Equal(t, "Bug", typ)
	assert.Equal(t, 1, fc.calls)

	// second call must hit cache, not the client
	typ2, err := a.GetIssueType(context.Background(), "OD-1")
	require.NoError(t, err)
	assert.Equal(t, "Bug", typ2)
	assert.Equal(t, 1, fc.calls, "expected cache hit, client should not be called again")
}

func TestGetIssueType_CacheExpiry(t *testing.T) {
	fc := &fakeClient{issue: &pkgjira.Issue{Key: "OD-2", Type: "Story"}}
	a := newTestAdapter(t, fc, -time.Second) // negative TTL → no caching

	typ, err := a.GetIssueType(context.Background(), "OD-2")
	require.NoError(t, err)
	assert.Equal(t, "Story", typ)

	// with TTL<=0, every call is live
	_, _ = a.GetIssueType(context.Background(), "OD-2")
	assert.Equal(t, 2, fc.calls)
}

func TestGetIssueType_NotFound(t *testing.T) {
	fc := &fakeClient{issue: nil}
	a := newTestAdapter(t, fc, time.Hour)

	typ, err := a.GetIssueType(context.Background(), "MISSING-99")
	require.NoError(t, err)
	assert.Empty(t, typ)
}

func TestGetActiveSprintIssueKeys_LiveAndCached(t *testing.T) {
	fc := &fakeClient{
		sprint:     &pkgjira.Sprint{ID: 42, Name: "Sprint 1"},
		sprintKeys: []string{issueKeyOD10, issueKeyOD11},
	}
	a := newTestAdapter(t, fc, time.Hour)

	keys, err := a.GetActiveSprintIssueKeys(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, []string{issueKeyOD10, issueKeyOD11}, keys)
	assert.Equal(t, 2, fc.calls) // GetActiveSprint + GetSprintIssueKeys

	// second call must hit cache
	keys2, err := a.GetActiveSprintIssueKeys(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, []string{issueKeyOD10, issueKeyOD11}, keys2)
	assert.Equal(t, 2, fc.calls, "expected cache hit")
}

func TestGetActiveSprintIssueKeys_NoActiveSprint(t *testing.T) {
	fc := &fakeClient{sprint: nil}
	a := newTestAdapter(t, fc, time.Hour)

	keys, err := a.GetActiveSprintIssueKeys(context.Background(), 9)
	require.NoError(t, err)
	assert.Nil(t, keys)
}

func TestCacheFile_Sanitize(t *testing.T) {
	assert.Equal(t, "OD_3345", sanitizeKey("OD:3345"))
	assert.Equal(t, "OD_3345", sanitizeKey("OD/3345"))
}

func TestCacheFile_BadDir(t *testing.T) {
	// write to a path that cannot be created (file in place of dir)
	f, err := os.CreateTemp(t.TempDir(), "file")
	require.NoError(t, err)
	f.Close()

	fc := &fakeClient{issue: &pkgjira.Issue{Key: "OD-1", Type: "Bug"}}
	// point CacheDir at the file — MkdirAll will fail silently
	a := New(fc, Config{CacheDir: f.Name(), TTL: time.Hour}, slog.Default())

	// should still return live data even when cache write fails
	typ, err := a.GetIssueType(context.Background(), "OD-1")
	require.NoError(t, err)
	assert.Equal(t, "Bug", typ)
}

const (
	testGlobalID = "mrboard:123:456"
	testTitle    = "feat: add widget"
	testURL      = "https://gitlab.example.com/group/repo/-/merge_requests/456"
	testIssueKey = "OD-99"
)

func TestUpsertRemoteLink_NewLink(t *testing.T) {
	// Cold start: no disk cache, JIRA returns "" (no existing link) → should POST.
	fc := &fakeClient{remoteLinkTitle: ""}
	a := newTestAdapter(t, fc, time.Hour)

	err := a.UpsertRemoteLink(context.Background(), testIssueKey, testGlobalID, testTitle, testURL)
	require.NoError(t, err)
	assert.Equal(t, 1, fc.calls, "expected one GET remote link call")
	assert.Equal(t, 1, fc.createRemoteCalls, "expected one POST call")

	// session map must be populated — second call must be a no-op
	fc.calls = 0
	fc.createRemoteCalls = 0
	err = a.UpsertRemoteLink(context.Background(), testIssueKey, testGlobalID, testTitle, testURL)
	require.NoError(t, err)
	assert.Equal(t, 0, fc.calls, "session hit: no HTTP calls expected")
	assert.Equal(t, 0, fc.createRemoteCalls)
}

func TestUpsertRemoteLink_DiskCacheHit(t *testing.T) {
	// Pre-seed the disk cache with the current title.
	fc := &fakeClient{}
	a := newTestAdapter(t, fc, time.Hour)
	filename := a.cacheFile("remotelinks/" + sanitizeKey(testGlobalID) + ".json")
	a.writeCache(filename, testTitle)

	err := a.UpsertRemoteLink(context.Background(), testIssueKey, testGlobalID, testTitle, testURL)
	require.NoError(t, err)
	assert.Equal(t, 0, fc.calls, "disk cache hit: no HTTP calls expected")
	assert.Equal(t, 0, fc.createRemoteCalls)
}

func TestUpsertRemoteLink_JIRAAlreadyCurrent(t *testing.T) {
	// Disk cache miss, but JIRA already has the correct title → skip POST.
	fc := &fakeClient{remoteLinkTitle: testTitle}
	a := newTestAdapter(t, fc, time.Hour)

	err := a.UpsertRemoteLink(context.Background(), testIssueKey, testGlobalID, testTitle, testURL)
	require.NoError(t, err)
	assert.Equal(t, 1, fc.calls, "expected one GET call")
	assert.Equal(t, 0, fc.createRemoteCalls, "no POST when JIRA title already matches")
}

func TestUpsertRemoteLink_TitleChanged(t *testing.T) {
	// Disk cache has stale title → skip GET, go straight to POST.
	fc := &fakeClient{}
	a := newTestAdapter(t, fc, time.Hour)
	filename := a.cacheFile("remotelinks/" + sanitizeKey(testGlobalID) + ".json")
	a.writeCache(filename, "old title")

	err := a.UpsertRemoteLink(context.Background(), testIssueKey, testGlobalID, testTitle, testURL)
	require.NoError(t, err)
	assert.Equal(t, 0, fc.calls, "stale disk cache: no GET (go straight to POST)")
	assert.Equal(t, 1, fc.createRemoteCalls, "expected one POST to update title")
}

func TestUpsertRemoteLink_GetError(t *testing.T) {
	// GET fails → error returned, session map not populated.
	boom := errors.New("network error")
	fc := &fakeClient{remoteLinkErr: boom}
	a := newTestAdapter(t, fc, time.Hour)

	err := a.UpsertRemoteLink(context.Background(), testIssueKey, testGlobalID, testTitle, testURL)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)

	// retry must attempt GET again (session map untouched)
	fc.remoteLinkErr = nil
	fc.calls = 0
	err = a.UpsertRemoteLink(context.Background(), testIssueKey, testGlobalID, testTitle, testURL)
	require.NoError(t, err)
	assert.Equal(t, 1, fc.calls, "retry must re-attempt GET")
}

func TestUpsertRemoteLink_PostError(t *testing.T) {
	// POST fails → error returned, session map not populated (allows retry).
	boom := errors.New("jira 500")
	fc := &fakeClient{remoteLinkTitle: "", createRemoteErr: boom}
	a := newTestAdapter(t, fc, time.Hour)

	err := a.UpsertRemoteLink(context.Background(), testIssueKey, testGlobalID, testTitle, testURL)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)

	// retry must attempt GET + POST again
	fc.createRemoteErr = nil
	fc.calls = 0
	fc.createRemoteCalls = 0
	err = a.UpsertRemoteLink(context.Background(), testIssueKey, testGlobalID, testTitle, testURL)
	require.NoError(t, err)
	assert.Equal(t, 1, fc.calls)
	assert.Equal(t, 1, fc.createRemoteCalls)
}
