package snapshotstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

var (
	testSprintStart = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	testMergedAt    = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
)

func TestSave_BoardSnapshotKeepsOpenMRsOnly(t *testing.T) {
	store, err := New(Config{Dir: t.TempDir()})
	require.NoError(t, err)

	open := domain.MergeRequest{ID: 1, IID: 1, ProjectID: 3}
	require.NoError(t, store.Save([]domain.MergeRequest{open, {ID: 2, IID: 2, ProjectID: 3, MergedAt: testMergedAt}}))

	loaded, _, err := store.Load()

	require.NoError(t, err)
	assert.Equal(t, []domain.MergeRequest{open}, loaded)
}

func TestMergedStore_RoundTripForSameSince(t *testing.T) {
	store, err := NewMerged(Config{Dir: t.TempDir()})
	require.NoError(t, err)

	merged := domain.MergeRequest{
		ID: 2, IID: 2, ProjectID: 3, MergedAt: testMergedAt, HeadSHA: "abc",
		DiffStats: []domain.FileDiffStat{{Path: "a.go", Additions: 3}},
	}
	require.NoError(t, store.Save(testSprintStart, []domain.MergeRequest{{ID: 1, IID: 1, ProjectID: 3}, merged}))

	loaded, err := store.Load(testSprintStart.In(time.FixedZone("CEST", 2*3600)))

	require.NoError(t, err)
	require.Len(t, loaded, 1, "only merged MRs are stored")
	assert.Equal(t, merged.Key(), loaded[0].Key())
	assert.True(t, loaded[0].MergedAt.Equal(testMergedAt))
	assert.Equal(t, merged.DiffStats, loaded[0].DiffStats)
}

func TestMergedStore_OtherSinceDropsTheSnapshot(t *testing.T) {
	dir := t.TempDir()
	store, err := NewMerged(Config{Dir: dir})
	require.NoError(t, err)
	require.NoError(t, store.Save(testSprintStart, []domain.MergeRequest{{ID: 2, IID: 2, MergedAt: testMergedAt}}))

	loaded, err := store.Load(testSprintStart.AddDate(0, 0, 14))

	require.NoError(t, err)
	assert.Nil(t, loaded)
	_, statErr := os.Stat(filepath.Join(dir, "merged_snapshot.json"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "a snapshot for another sprint start is dropped")
	again, err := store.Load(testSprintStart)
	require.NoError(t, err)
	assert.Nil(t, again)
}

func TestMergedStore_BoardSaveNeverTouchesIt(t *testing.T) {
	dir := t.TempDir()
	merged, err := NewMerged(Config{Dir: dir})
	require.NoError(t, err)
	board, err := New(Config{Dir: dir})
	require.NoError(t, err)
	require.NoError(t, merged.Save(testSprintStart, []domain.MergeRequest{{ID: 2, IID: 2, MergedAt: testMergedAt}}))

	require.NoError(t, board.Save([]domain.MergeRequest{{ID: 1, IID: 1}}))

	loaded, err := merged.Load(testSprintStart)
	require.NoError(t, err)
	assert.Len(t, loaded, 1)
}

func TestMergedStore_AbsentOrCorruptIsCold(t *testing.T) {
	dir := t.TempDir()
	store, err := NewMerged(Config{Dir: dir})
	require.NoError(t, err)

	absent, err := store.Load(testSprintStart)
	require.NoError(t, err)
	assert.Nil(t, absent)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "merged_snapshot.json"), []byte("{"), fileMode))
	corrupt, err := store.Load(testSprintStart)
	require.NoError(t, err)
	assert.Nil(t, corrupt)
}
