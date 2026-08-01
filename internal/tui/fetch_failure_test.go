package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

// recordingSnapshotStore counts Save calls so a test can assert whether a
// landing fetch result was persisted to the cache.
type recordingSnapshotStore struct {
	saves int
	last  []domain.MergeRequest
}

func (s *recordingSnapshotStore) Load() ([]domain.MergeRequest, time.Time, error) {
	return nil, time.Time{}, nil
}

func (s *recordingSnapshotStore) Save(mrs []domain.MergeRequest) error {
	s.saves++
	s.last = mrs
	return nil
}

// makeModelWithSnapshotStore builds a board-state Model over snap, then resets
// snap's counter so a test observes only the saves it triggers itself.
func makeModelWithSnapshotStore(t *testing.T, snap *recordingSnapshotStore) Model {
	t.Helper()
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(someMRs(), nil).Maybe()

	m := New(context.Background(), &config.Config{}, src, noopStore{}, snap, nil, nil, nil, "dev", Options{})
	next, _ := m.Update(FetchResultMsg{MRs: someMRs()})
	snap.saves = 0
	snap.last = nil
	return next.(Model)
}

// --- Snapshot cache must survive a failed fetch ---

// TestModel_TotalFetchFailureDoesNotOverwriteSnapshot is the regression test for
// the observed cache poisoning: an all-errors fetch wrote {"mrs":null} over the
// last-known-good snapshot that the next launch boots from.
func TestModel_TotalFetchFailureDoesNotOverwriteSnapshot(t *testing.T) {
	snap := &recordingSnapshotStore{}
	m := makeModelWithSnapshotStore(t, snap)
	before := m.AllMRs()

	next, _ := m.Update(FetchResultMsg{MRs: nil, Errors: []error{errors.New("context deadline exceeded")}})
	m2 := next.(Model)

	assert.Zero(t, snap.saves, "a wholly failed fetch must not overwrite the cache")
	assert.Equal(t, StateBoard, m2.State(), "the board must still be shown, with the errors surfaced")
	assert.Len(t, m2.Errors(), 1, "the fetch errors must reach a user-visible surface")
	assert.NotEqual(t, before, m2.AllMRs(), "the failed result still replaces the in-memory board")
}

// TestModel_PartialFetchFailureDoesNotOverwriteSnapshot covers the quieter half:
// when some sources error, the surviving MRs are not an authoritative board and
// must not be persisted as one.
func TestModel_PartialFetchFailureDoesNotOverwriteSnapshot(t *testing.T) {
	snap := &recordingSnapshotStore{}
	m := makeModelWithSnapshotStore(t, snap)

	partial := someMRs()[:1]
	next, _ := m.Update(FetchResultMsg{MRs: partial, Errors: []error{errors.New("user bjean: boom")}})
	m2 := next.(Model)

	assert.Zero(t, snap.saves, "a truncated board must not be persisted as authoritative")
	assert.Len(t, m2.AllMRs(), 1)
}

func TestModel_CleanFetchSavesSnapshot(t *testing.T) {
	snap := &recordingSnapshotStore{}
	m := makeModelWithSnapshotStore(t, snap)

	next, _ := m.Update(FetchResultMsg{MRs: someMRs()})
	m2 := next.(Model)

	assert.Equal(t, 1, snap.saves, "a clean fetch must refresh the cache")
	assert.Equal(t, m2.AllMRs(), snap.last)
}

// --- A superseded fetch result must not land ---

// TestModel_SupersededFetchResultIsDropped covers the double-refresh race: the
// first fetch is cancelled by the second and returns a degraded partial result,
// which must not replace the board or the cache when it lands late.
func TestModel_SupersededFetchResultIsDropped(t *testing.T) {
	snap := &recordingSnapshotStore{}
	m := makeModelWithSnapshotStore(t, snap)
	before := m.AllMRs()

	// A newer fetch (seq 2) is in flight; the cancelled older one (seq 1) lands.
	m.fetchSeq = 2
	m.isRefreshing = true

	next, cmd := m.Update(FetchResultMsg{MRs: nil, Errors: []error{context.Canceled}, Seq: 1})
	m2 := next.(Model)

	assert.Nil(t, cmd, "a superseded result must not schedule follow-up work")
	assert.Equal(t, before, m2.AllMRs(), "a superseded result must not replace the board")
	assert.Zero(t, snap.saves, "a superseded result must not touch the cache")
	assert.True(t, m2.IsRefreshing(), "the newer fetch is still in flight")
}

func TestModel_CurrentFetchResultLands(t *testing.T) {
	snap := &recordingSnapshotStore{}
	m := makeModelWithSnapshotStore(t, snap)
	m.fetchSeq = 2
	m.isRefreshing = true

	next, _ := m.Update(FetchResultMsg{MRs: someMRs()[:1], Seq: 2})
	m2 := next.(Model)

	assert.Len(t, m2.AllMRs(), 1, "the current fetch's result must land")
	assert.False(t, m2.IsRefreshing())
}

// --- Manual refresh must not stack fetches ---

// TestModel_ManualRefreshWhileFetchInFlightDoesNotStartSecondFetch pins the
// guard commit 9491963 dropped: startFetch cancels the in-flight fetch, so a
// second manual refresh would abort a good fetch and land a degraded result.
func TestModel_ManualRefreshWhileFetchInFlightDoesNotStartSecondFetch(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	m.refreshInterval = time.Minute
	m.isRefreshing = true
	seqBefore := m.fetchSeq
	genBefore := m.refreshGen

	next, cmd := m.Update(tea.KeyPressMsg{Text: "r", Code: 'r'})
	m2 := next.(Model)

	assert.Equal(t, seqBefore, m2.fetchSeq, "no second fetch may be dispatched while one is in flight")
	assert.NotEqual(t, genBefore, m2.refreshGen, "the cadence is still reset")
	assert.NotNil(t, cmd, "the refreshed timer schedule must still be returned")
	assert.True(t, m2.IsRefreshing())
}

func TestModel_ManualRefreshWhenIdleStartsFetch(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	m.refreshInterval = time.Minute
	seqBefore := m.fetchSeq

	next, cmd := m.Update(tea.KeyPressMsg{Text: "r", Code: 'r'})
	m2 := next.(Model)

	assert.Equal(t, seqBefore+1, m2.fetchSeq, "an idle manual refresh must dispatch a fetch")
	assert.True(t, m2.IsRefreshing())
	assert.NotNil(t, cmd)
}
