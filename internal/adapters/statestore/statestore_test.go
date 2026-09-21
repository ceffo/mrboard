package statestore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

func TestYAMLStore_SaveLoad_RoundTripsExclusionFilter(t *testing.T) {
	store, err := New(Config{Dir: t.TempDir()})
	require.NoError(t, err)

	want := domain.AppState{
		SortField: "repo_iid",
		ViewMode:  domain.ViewAll,
		ThemeMode: "auto",
		Filter: domain.FilterCriteria{
			ExcludedAssignees: []string{"bob"},
			ExcludeTicketless: true,
		},
	}
	require.NoError(t, store.Save(want))

	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestYAMLStore_Load_PreUpgradeInclusionFilterResetsToShowAll covers the
// migration off the old inclusion-based Filters tab: a state.yaml written by
// that version uses the same keys under different names, so it must load
// cleanly with the MR filter reset to "show all" rather than being
// reinterpreted as its own opposite (see domain.FilterCriteria).
func TestYAMLStore_Load_PreUpgradeInclusionFilterResetsToShowAll(t *testing.T) {
	dir := t.TempDir()
	legacy := "sort_field: repo_iid\n" +
		"view_mode: 0\n" +
		"filter:\n" +
		"  assignees: [alice]\n" +
		"  reviewers: [bob]\n" +
		"  ticket_keys: [OD-100]\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.yaml"), []byte(legacy), 0o600))

	store, err := New(Config{Dir: dir})
	require.NoError(t, err)

	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "repo_iid", got.SortField, "unrelated fields still load")
	assert.Equal(t, domain.FilterCriteria{}, got.Filter, "the pre-upgrade filter resets to show-all")
}

func TestYAMLStore_Load_AbsentFileReturnsDefaultState(t *testing.T) {
	store, err := New(Config{Dir: t.TempDir()})
	require.NoError(t, err)

	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, domain.DefaultAppState(), got)
}
