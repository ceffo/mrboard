package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

// warmSnapshotStore is a SnapshotStore that reports a pre-populated, aged cache.
type warmSnapshotStore struct {
	mrs       []domain.MergeRequest
	writtenAt time.Time
	saved     []domain.MergeRequest
}

func (s *warmSnapshotStore) Load() ([]domain.MergeRequest, time.Time, error) {
	return s.mrs, s.writtenAt, nil
}

func (s *warmSnapshotStore) Save(mrs []domain.MergeRequest) error {
	s.saved = mrs
	return nil
}

// --- Non-blocking refresh: docs/adr/0005 ---

func TestModel_BaseStack_IncludesBoardWhileRefreshing(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	m.isRefreshing = true

	stack := m.BaseStack()

	require.Len(t, stack, 3, "isRefreshing must no longer collapse the stack to just [BaseCtx]")
	assert.Contains(t, stack, BoardCtx, "board context must stay reachable in the stack during a refresh")
}

func TestModel_HandleKey_MovesSelectionWhileRefreshing(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	m.isRefreshing = true
	before := m.Selected()

	next, _ := m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m2 := next.(Model)

	assert.NotEqual(t, before, m2.Selected(), "arrow-key navigation must work while a fetch is in flight")
}

// --- Boot from cache ---

func TestNew_ColdCache_StaysInLoadingState(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	m := New(context.Background(), &config.Config{}, src, noopStore{}, noopSnapshotStore{},
		nil, nil, nil, "dev", Options{})

	assert.Equal(t, StateLoading, m.State(), "a genuinely cold cache must still show the loading state")
}

func TestNew_ColdCache_RecoversOnFetchResult(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(someMRs(), nil).Maybe()

	m := New(context.Background(), &config.Config{}, src, noopStore{}, noopSnapshotStore{},
		nil, nil, nil, "dev", Options{})

	next, _ := m.Update(FetchResultMsg{MRs: someMRs()})
	m2 := next.(Model)

	assert.Equal(t, StateBoard, m2.State())
}

func TestNew_WarmCache_BootsInteractiveAtAnyAge(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	writtenAt := time.Now().Add(-72 * time.Hour) // a three-day-old snapshot
	snap := &warmSnapshotStore{mrs: someMRs(), writtenAt: writtenAt}

	m := New(context.Background(), &config.Config{}, src, noopStore{}, snap, nil, nil, nil, "dev", Options{})

	assert.Equal(t, StateBoard, m.State(), "a warm cache of any age must render the board immediately")
	assert.Len(t, m.AllMRs(), len(someMRs()))
	assert.True(t, m.IsRefreshing(), "Init always starts a fetch, so the board boots in the refreshing state")
	assert.Equal(t, writtenAt, m.SnapshotWrittenAt())

	stack := m.BaseStack()
	require.Len(t, stack, 3, "warm boot must be fully interactive, not gated by the boot-time fetch")
	assert.Contains(t, stack, BoardCtx)
}

func TestModel_FetchResultMsg_SavesSnapshot(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	snap := &warmSnapshotStore{}

	m := New(context.Background(), &config.Config{}, src, noopStore{}, snap, nil, nil, nil, "dev", Options{})

	mrs := someMRs()
	next, _ := m.Update(FetchResultMsg{MRs: mrs})
	m2 := next.(Model)

	assert.Len(t, snap.saved, len(mrs), "a successful fetch swap must save the new snapshot")
	assert.WithinDuration(t, time.Now(), m2.SnapshotWrittenAt(), time.Second)
}

// --- Auto-refresh timer (docs/adr/0005, "Refresh cadence") ---

func TestRefreshTickCmd_ZeroIntervalDisablesTimer(t *testing.T) {
	assert.Nil(t, refreshTickCmd(0, 0), "refresh_interval: 0 must disable the timer entirely")
	assert.Nil(t, refreshTickCmd(-time.Second, 0), "a negative interval must also disable the timer")
}

func TestModel_RefreshTick_FiresBackgroundFetchAndReschedules(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	m.refreshInterval = time.Minute
	m.isRefreshing = false

	next, cmd := m.Update(refreshTickMsg{gen: m.refreshGen})
	m2 := next.(Model)

	assert.True(t, m2.IsRefreshing(), "a live tick must start a background fetch")
	require.NotNil(t, cmd, "a live tick must reschedule the next one")
}

func TestModel_RefreshTick_SkippedWhileFetchInFlight(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	m.refreshInterval = time.Minute
	m.isRefreshing = true
	cancelCalled := false
	m.fetchCancel = func() { cancelCalled = true }

	next, cmd := m.Update(refreshTickMsg{gen: m.refreshGen})
	m2 := next.(Model)

	assert.True(t, m2.IsRefreshing(), "still refreshing from the original in-flight fetch")
	assert.False(t, cancelCalled, "a skipped tick must not touch the in-flight fetch's cancel func")
	assert.NotNil(t, cmd, "the recurring schedule must continue even when a tick is skipped")
}

func TestModel_RefreshTick_StaleGenerationIsDropped(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	m.refreshInterval = time.Minute
	staleGen := m.refreshGen
	m.refreshGen++ // simulate a manual refresh having superseded this schedule

	next, cmd := m.Update(refreshTickMsg{gen: staleGen})
	m2 := next.(Model)

	assert.Nil(t, cmd, "a tick from a superseded schedule must not reschedule itself")
	assert.False(t, m2.IsRefreshing(), "a stale tick must not start a fetch")
}

func TestModel_ManualRefresh_ResetsTimerGeneration(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	m.refreshInterval = time.Minute
	staleGen := m.refreshGen

	next, cmd := m.Update(tea.KeyPressMsg{Text: "r", Code: 'r'})
	m2 := next.(Model)

	require.NotNil(t, cmd)
	assert.NotEqual(t, staleGen, m2.refreshGen, "manual refresh must bump the generation")

	// A tick belonging to the pre-reset schedule must be dropped, not treated as live.
	next2, tickCmd2 := m2.Update(refreshTickMsg{gen: staleGen})
	m3 := next2.(Model)
	assert.Nil(t, tickCmd2, "a tick from the pre-reset schedule must be dropped")
	assert.Equal(t, m2.IsRefreshing(), m3.IsRefreshing(), "a dropped stale tick must not change refresh state")
}

func TestModel_RefreshTick_UnchangedDataCausesNoBoardMutationOrSelectionChange(t *testing.T) {
	m := selectSecondMR(t, makeModel(t, someMRs(), ""))
	m.refreshInterval = time.Minute
	wantSelected := m.Selected()
	wantMRs := m.AllMRs()

	ticked, _ := m.Update(refreshTickMsg{gen: m.refreshGen})
	m2 := ticked.(Model)
	require.True(t, m2.IsRefreshing())

	// Phase 1 found nothing changed: the landing snapshot is identical to what's displayed.
	landed, _ := m2.Update(FetchResultMsg{MRs: someMRs()})
	m3 := landed.(Model)

	assert.Equal(t, wantSelected, m3.Selected(), "an unchanged refresh must not disturb the selected MR")
	assert.Equal(t, wantMRs, m3.AllMRs(), "an unchanged refresh must not mutate the displayed MR set")
	assert.False(t, m3.IsRefreshing(), "the fetch must have landed and cleared the refreshing flag")
}
