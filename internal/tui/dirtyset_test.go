package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

func dirtySetTestMR() domain.MergeRequest {
	return domain.MergeRequest{ID: 1, IID: 1, ProjectID: 7}
}

func TestDirtySet_KeysEmptyWhenClean(t *testing.T) {
	d := newDirtySet()
	assert.Nil(t, d.Keys())
}

func TestDirtySet_MarkAndKeys(t *testing.T) {
	d := newDirtySet()
	key := dirtySetTestMR().Key()
	writeAt := time.Now()

	d.Mark(key, writeAt)

	assert.Equal(t, []domain.MRKey{key}, d.Keys())
}

// TestDirtySet_Resolve_KeepsLiveEntryOnStaleLanding reproduces the write race
// from docs/adr/0005, "The write race that ungating creates": a fetch that
// started before a local write must not clobber it.
func TestDirtySet_Resolve_KeepsLiveEntryOnStaleLanding(t *testing.T) {
	d := newDirtySet()
	mr := dirtySetTestMR()
	writeAt := time.Now()
	d.Mark(mr.Key(), writeAt)

	live := []domain.MergeRequest{mr}
	landingStale := dirtySetTestMR()
	landingStale.Author = "stale-snapshot"

	result := d.Resolve(live, []domain.MergeRequest{landingStale}, writeAt.Add(-time.Second))

	require.Len(t, result, 1)
	assert.Equal(t, mr, result[0], "a landing snapshot started before the write must not overwrite it")
	_, stillDirty := d[mr.Key()]
	assert.True(t, stillDirty, "an unconfirmed write must remain dirty")
}

// TestDirtySet_Resolve_ClearsEntryConfirmedByLanding verifies the other half
// of the guard: a fetch started after the write confirms it and clears the
// dirty entry.
func TestDirtySet_Resolve_ClearsEntryConfirmedByLanding(t *testing.T) {
	d := newDirtySet()
	mr := dirtySetTestMR()
	writeAt := time.Now()
	d.Mark(mr.Key(), writeAt)

	landing := dirtySetTestMR()
	landing.Author = "confirmed"

	result := d.Resolve([]domain.MergeRequest{mr}, []domain.MergeRequest{landing}, writeAt.Add(time.Second))

	require.Len(t, result, 1)
	assert.Equal(t, landing, result[0], "a confirming fetch's value must win")
	_, stillDirty := d[mr.Key()]
	assert.False(t, stillDirty, "a confirmed write must be cleared")
}

// TestDirtySet_Resolve_KeepsLiveEntryAbsentFromLanding covers a dirty-and-stale
// key missing from the landing snapshot entirely (e.g. a concurrent phase-1
// hiccup) — the live entry must be kept rather than silently dropped.
func TestDirtySet_Resolve_KeepsLiveEntryAbsentFromLanding(t *testing.T) {
	d := newDirtySet()
	mr := dirtySetTestMR()
	writeAt := time.Now()
	d.Mark(mr.Key(), writeAt)

	result := d.Resolve([]domain.MergeRequest{mr}, nil, writeAt.Add(-time.Second))

	require.Len(t, result, 1)
	assert.Equal(t, mr, result[0])
}

func TestDirtySet_Resolve_NoopWhenClean(t *testing.T) {
	d := newDirtySet()
	landing := []domain.MergeRequest{dirtySetTestMR()}

	result := d.Resolve(nil, landing, time.Now())

	assert.Equal(t, landing, result)
}
