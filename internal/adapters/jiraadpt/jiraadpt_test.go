package jiraadpt

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	pkgjira "github.com/ceffo/mrboard/pkg/jira"
)

const (
	issueKeyOD10 = "OD-10"
	issueKeyOD11 = "OD-11"
	sprintName1  = "Sprint 1"
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

const testCacheDir = "/cache"

func newTestAdapter(t *testing.T, client jiraClient, ttl time.Duration) *JiraAdapter {
	t.Helper()
	return newTestAdapterWithConfig(t, client, Config{TTL: ttl, SprintCacheTTL: ttl})
}

func newTestAdapterWithConfig(t *testing.T, client jiraClient, cfg Config) *JiraAdapter {
	t.Helper()
	cfg.FS = afero.NewMemMapFs()
	cfg.CacheDir = testCacheDir
	a, err := New(client, cfg, slog.Default())
	require.NoError(t, err)
	return a
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

func TestGetIssueType_CaseInsensitiveMatch_SharesCacheAcrossCase(t *testing.T) {
	fc := &fakeClient{issue: &pkgjira.Issue{Key: "OD-3", Type: "Task"}}
	a := newTestAdapterWithConfig(t, fc, Config{TTL: time.Hour, KeyMatcher: domain.NewTicketKeyMatcher(true)})

	_, err := a.GetIssueType(context.Background(), "od-3")
	require.NoError(t, err)

	// a subsequent lookup in a different case must hit the same cache entry
	_, err = a.GetIssueType(context.Background(), "OD-3")
	require.NoError(t, err)
	assert.Equal(t, 1, fc.calls, "differently-cased lookups of the same key should share one cache entry")
}

func TestGetIssueType_CaseSensitive_KeepsKeyAsIs(t *testing.T) {
	fc := &fakeClient{issue: &pkgjira.Issue{Key: "od-4", Type: "Task"}}
	a := newTestAdapterWithConfig(t, fc, Config{TTL: time.Hour, KeyMatcher: domain.NewTicketKeyMatcher(false)})

	_, err := a.GetIssueType(context.Background(), "od-4")
	require.NoError(t, err)
	_, err = a.GetIssueType(context.Background(), "OD-4")
	require.NoError(t, err)
	assert.Equal(t, 2, fc.calls, "differently-cased lookups must not share a cache entry when case-sensitive")
}

var (
	sprintStart = time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	sprintEnd   = time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
)

func TestGetActiveSprint(t *testing.T) {
	tests := []struct {
		name      string
		sprint    *pkgjira.Sprint
		ttl       time.Duration
		wantNil   bool
		wantCalls []int // cumulative client calls after each ask
	}{
		{
			name:      "cached within the sprint TTL",
			sprint:    &pkgjira.Sprint{ID: 42, Name: sprintName1, Start: sprintStart, End: sprintEnd},
			ttl:       time.Hour,
			wantCalls: []int{2, 2},
		},
		{
			name:      "expired TTL revalidates every ask",
			sprint:    &pkgjira.Sprint{ID: 42, Name: sprintName1, Start: sprintStart, End: sprintEnd},
			ttl:       -time.Second,
			wantCalls: []int{2, 4},
		},
		{
			name:      "no active sprint is nil without error and is not cached",
			sprint:    nil,
			ttl:       time.Hour,
			wantNil:   true,
			wantCalls: []int{1, 2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakeClient{sprint: tc.sprint, sprintKeys: []string{issueKeyOD10, issueKeyOD11}}
			a := newTestAdapterWithConfig(t, fc, Config{TTL: time.Hour, SprintCacheTTL: tc.ttl})

			for i, wantCalls := range tc.wantCalls {
				got, err := a.GetActiveSprint(context.Background(), 7)
				require.NoError(t, err)
				if tc.wantNil {
					assert.Nil(t, got)
				} else {
					require.NotNil(t, got)
					assert.Equal(t, sprintName1, got.Name)
					assert.True(t, sprintStart.Equal(got.Start), "start %v", got.Start)
					assert.True(t, sprintEnd.Equal(got.End), "end %v", got.End)
					assert.Equal(t, []string{issueKeyOD10, issueKeyOD11}, got.IssueKeys)
				}
				assert.Equal(t, wantCalls, fc.calls, "client calls after ask %d", i+1)
			}
		})
	}
}

func TestGetActiveSprint_ExpiredCacheObservesRollover(t *testing.T) {
	fc := &fakeClient{
		sprint:     &pkgjira.Sprint{ID: 42, Name: sprintName1},
		sprintKeys: []string{issueKeyOD10},
	}
	// A long general TTL alongside an already-expired SprintCacheTTL: the sprint
	// cache is never trusted, without the caller passing a forceRefresh flag.
	a := newTestAdapterWithConfig(t, fc, Config{TTL: time.Hour, SprintCacheTTL: -time.Second})

	got, err := a.GetActiveSprint(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, []string{issueKeyOD10}, got.IssueKeys)

	fc.sprint = &pkgjira.Sprint{ID: 43, Name: "Sprint 2"}
	fc.sprintKeys = []string{issueKeyOD11}

	got, err = a.GetActiveSprint(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, "Sprint 2", got.Name)
	assert.Equal(t, []string{issueKeyOD11}, got.IssueKeys)
}

func TestGetActiveSprint_CaseInsensitiveMatch_UppercasesKeys(t *testing.T) {
	fc := &fakeClient{
		sprint:     &pkgjira.Sprint{ID: 42, Name: "Active Sprint"},
		sprintKeys: []string{"od-10", "Od-11"},
	}
	a := newTestAdapterWithConfig(t, fc, Config{
		TTL: time.Hour, SprintCacheTTL: time.Hour, KeyMatcher: domain.NewTicketKeyMatcher(true),
	})

	got, err := a.GetActiveSprint(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{issueKeyOD10, issueKeyOD11}, got.IssueKeys,
		"sprint issue keys should be uppercased to match the shared TicketKeyMatcher's canonical output")
}

func TestNew_BadCacheDir(t *testing.T) {
	// A read-only filesystem can never satisfy the store's MkdirAll, so
	// construction fails fast instead of silently degrading on every write.
	fs := afero.NewReadOnlyFs(afero.NewMemMapFs())
	fc := &fakeClient{issue: &pkgjira.Issue{Key: "OD-1", Type: "Bug"}}

	_, err := New(fc, Config{FS: fs, CacheDir: "/cache", TTL: time.Hour}, slog.Default())
	require.Error(t, err)
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
	a.setCache(context.Background(), remoteLinkCacheKey(testGlobalID), testTitle)

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
	a.setCache(context.Background(), remoteLinkCacheKey(testGlobalID), "old title")

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

func TestGetActiveSprint_IgnoresLegacyCacheEntry(t *testing.T) {
	fc := &fakeClient{
		sprint:     &pkgjira.Sprint{ID: 42, Name: sprintName1, Start: sprintStart, End: sprintEnd},
		sprintKeys: []string{issueKeyOD10},
	}
	a := newTestAdapter(t, fc, time.Hour)

	// The pre-dates cache stored a bare key list under the unversioned key.
	require.NoError(t, a.cache.Set(context.Background(), "sprint_board_7", []string{"OLD-1"}))

	got, err := a.GetActiveSprint(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{issueKeyOD10}, got.IssueKeys, "the legacy entry must not be served")
	assert.Equal(t, 2, fc.calls, "first call must fetch live")

	got, err = a.GetActiveSprint(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, sprintName1, got.Name)
	assert.Equal(t, 2, fc.calls, "the v2 entry must have been written")
}
