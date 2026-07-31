package tui

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

// dirtyGuardMR is the MR used to reproduce the write race described in
// docs/adr/0005, "The write race that ungating creates".
func dirtyGuardMR() domain.MergeRequest {
	return domain.MergeRequest{
		ID: 1, IID: 42, ProjectID: 7, Author: editorTestApprover, ProjectPath: "org/alpha",
		Reviewers: []domain.ReviewerInfo{{Username: editorTestOther, State: domain.ReviewerNotStarted}},
	}
}

// TestFetchResultMsg_DirtyGuard_PreservesLocalWriteAndIssuesTargetedRefetch
// reproduces the exact race the guard closes: a refresh starts at T+0, a
// reviewer edit is saved at T+2 (marking the MR dirty), and the snapshot that
// started at T+0 lands at T+2.5 still carrying the pre-write reviewer list.
// The write must survive, and a targeted refetch must be issued immediately
// rather than waiting for the next refresh tick.
func TestFetchResultMsg_DirtyGuard_PreservesLocalWriteAndIssuesTargetedRefetch(t *testing.T) {
	original := dirtyGuardMR()
	src := mocks.NewMockMergeRequestSource(t)

	m := New(context.Background(), &config.Config{}, src, noopStore{}, noopSnapshotStore{},
		nil, nil, nil, "dev", Options{})
	next, _ := m.Update(FetchResultMsg{MRs: []domain.MergeRequest{original}})
	m = next.(Model)

	// T+2: save a reviewer edit — bob is now an approver.
	updated := original
	updated.Reviewers = []domain.ReviewerInfo{
		{Username: editorTestOther, State: domain.ReviewerApproved, IsApprover: true},
	}
	before := time.Now()
	next, _ = m.Update(ReviewersSavedMsg{MR: updated})
	m = next.(Model)

	writeAt, ok := m.Dirty()[updated.Key()]
	require.True(t, ok, "a saved reviewer edit must register a dirty entry")
	require.False(t, writeAt.Before(before), "writeAt must be stamped no earlier than the save call")

	// The targeted refetch issued below must be the only FetchAll call this
	// test makes; any other call is unexpected and fails the mock.
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return([]domain.MergeRequest{updated}, nil).Once()

	// T+2.5: the snapshot that started at T+0 (before the write) lands, still
	// carrying the pre-write reviewer list.
	next, cmd := m.Update(FetchResultMsg{
		MRs:            []domain.MergeRequest{original},
		FetchStartedAt: before.Add(-time.Second),
	})
	m = next.(Model)

	require.Len(t, m.AllMRs(), 1)
	assert.Equal(t, updated.Reviewers, m.AllMRs()[0].Reviewers,
		"the written reviewer edit must survive a landing snapshot that started before the write")
	assert.True(t, m.IsRefreshing(), "a targeted refetch must be started immediately")

	runCmd(t, cmd) // executes the batched Cmd, which must call the mocked FetchAll above exactly once
}

// TestFetchResultMsg_DirtyGuard_ClearsWhenConfirmingFetchLands verifies the
// other half of the guard: a fetch that started after the write is trusted
// normally, and clears the dirty entry so no further targeted refetch fires.
func TestFetchResultMsg_DirtyGuard_ClearsWhenConfirmingFetchLands(t *testing.T) {
	original := dirtyGuardMR()
	// No FetchAll expectation is set: once the write is confirmed, no further
	// targeted refetch should fire, so any FetchAll call here is unexpected
	// and fails the mock.
	src := mocks.NewMockMergeRequestSource(t)

	m := New(context.Background(), &config.Config{}, src, noopStore{}, noopSnapshotStore{},
		nil, nil, nil, "dev", Options{})
	next, _ := m.Update(FetchResultMsg{MRs: []domain.MergeRequest{original}})
	m = next.(Model)

	updated := original
	updated.Reviewers = []domain.ReviewerInfo{
		{Username: editorTestOther, State: domain.ReviewerApproved, IsApprover: true},
	}
	next, _ = m.Update(ReviewersSavedMsg{MR: updated})
	m = next.(Model)

	writeAt, ok := m.Dirty()[updated.Key()]
	require.True(t, ok, "a saved reviewer edit must register a dirty entry")

	// A fetch that started after the write confirms it and lands normally.
	next, cmd := m.Update(FetchResultMsg{
		MRs:            []domain.MergeRequest{updated},
		FetchStartedAt: writeAt.Add(time.Second),
	})
	m = next.(Model)

	require.Len(t, m.AllMRs(), 1)
	assert.Equal(t, updated.Reviewers, m.AllMRs()[0].Reviewers, "a confirming fetch applies normally")
	_, stillDirty := m.Dirty()[updated.Key()]
	assert.False(t, stillDirty, "a fetch that started after the write must clear the dirty entry")
	assert.False(t, m.IsRefreshing(), "no further targeted refetch is needed once the write is confirmed")

	runCmd(t, cmd) // no-op: cmd must be nil since ticket enrichment is unconfigured and no refetch is needed
}
