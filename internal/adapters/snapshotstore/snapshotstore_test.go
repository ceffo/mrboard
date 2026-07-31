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
