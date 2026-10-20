package snapshotstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

func TestLoad_AbsentFile_ReturnsEmptySnapshotNoError(t *testing.T) {
	store, err := New(Config{Dir: t.TempDir()})
	require.NoError(t, err)

	mrs, writtenAt, err := store.Load()

	assert.NoError(t, err)
	assert.Nil(t, mrs)
	assert.True(t, writtenAt.IsZero())
}

func TestLoad_CorruptFile_ReturnsEmptySnapshotNoError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "snapshot.json"), []byte("not json"), fileMode))
	store, err := New(Config{Dir: dir})
	require.NoError(t, err)

	mrs, writtenAt, err := store.Load()

	assert.NoError(t, err)
	assert.Nil(t, mrs)
	assert.True(t, writtenAt.IsZero())
}

func TestLoad_VersionMismatch_ReturnsEmptySnapshotNoError(t *testing.T) {
	dir := t.TempDir()
	stale := fileFormat{Version: currentVersion + 1, WrittenAt: time.Now(), MRs: []domain.MergeRequest{{ID: 1}}}
	data, err := json.Marshal(stale)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "snapshot.json"), data, fileMode))
	store, err := New(Config{Dir: dir})
	require.NoError(t, err)

	mrs, writtenAt, err := store.Load()

	assert.NoError(t, err)
	assert.Nil(t, mrs)
	assert.True(t, writtenAt.IsZero())
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	store, err := New(Config{Dir: t.TempDir()})
	require.NoError(t, err)

	updatedAt := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	original := []domain.MergeRequest{
		{ID: 1, IID: 2, ProjectID: 3, Title: "fix bug", UpdatedAt: updatedAt},
	}
	require.NoError(t, store.Save(original))

	loaded, writtenAt, err := store.Load()

	require.NoError(t, err)
	assert.Equal(t, original, loaded)
	assert.False(t, writtenAt.IsZero())
}

// TestLoad_SnapshotWithoutDiffStats_LoadsWithStatsUnknown checks a snapshot
// written before MRs carried a head SHA and diff stats still loads, with every
// MR's stats unknown rather than an empty diff.
func TestLoad_SnapshotWithoutDiffStats_LoadsWithStatsUnknown(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"version": 1, "written_at": "2026-10-01T00:00:00Z",
		"mrs": [{"ID": 1, "IID": 7, "ProjectID": 3, "Title": "old"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "snapshot.json"), []byte(legacy), fileMode))
	store, err := New(Config{Dir: dir})
	require.NoError(t, err)

	mrs, _, err := store.Load()

	require.NoError(t, err)
	require.Len(t, mrs, 1)
	assert.Equal(t, 7, mrs[0].IID)
	assert.Empty(t, mrs[0].HeadSHA)
	assert.Nil(t, mrs[0].DiffStats)
	assert.False(t, mrs[0].MeasureReviewSize(domain.NewSizeExclusions(true, nil, nil)).Size.Known)
}

// TestSaveLoad_DiffStatsRoundTrip checks head SHA and stats survive a round trip,
// and that a known empty diff stays known.
func TestSaveLoad_DiffStatsRoundTrip(t *testing.T) {
	store, err := New(Config{Dir: t.TempDir()})
	require.NoError(t, err)
	in := []domain.MergeRequest{
		{IID: 1, HeadSHA: "abc", DiffStats: []domain.FileDiffStat{{Path: "a.go", Additions: 2, Deletions: 1}}},
		{IID: 2, HeadSHA: "def", DiffStats: []domain.FileDiffStat{}},
		{IID: 3},
	}
	require.NoError(t, store.Save(in))

	out, _, err := store.Load()

	require.NoError(t, err)
	require.Len(t, out, 3)
	assert.Equal(t, in[0].DiffStats, out[0].DiffStats)
	assert.Equal(t, "abc", out[0].HeadSHA)
	assert.NotNil(t, out[1].DiffStats)
	assert.Empty(t, out[1].DiffStats)
	assert.Nil(t, out[2].DiffStats)
}
