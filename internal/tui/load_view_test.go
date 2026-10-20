package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/adapters/demoadpt"
	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// recordingSource records the MergedSince of every FetchAll it serves.
type recordingSource struct {
	mrsvc.MergeRequestSource
	mu    sync.Mutex
	since []time.Time
}

func (s *recordingSource) FetchAll(
	ctx context.Context, opts mrsvc.FetchOptions,
) ([]domain.MergeRequest, []error) {
	s.mu.Lock()
	s.since = append(s.since, opts.MergedSince)
	s.mu.Unlock()
	return s.MergeRequestSource.FetchAll(ctx, opts)
}

func (s *recordingSource) calls() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.since...)
}

// recordingStore keeps the last saved state and serves it back.
type recordingStore struct {
	state domain.AppState
	saves int
}

func (s *recordingStore) Load() (domain.AppState, error) { return s.state, nil }
func (s *recordingStore) Save(st domain.AppState) error {
	s.state = st
	s.saves++
	return nil
}

type loadHarness struct {
	m      Model
	src    *recordingSource
	store  *recordingStore
	demo   *demoadpt.Adapter
	merged domain.MergedSnapshotStore
}

// newLoadHarness boots the TUI against the "load" demo fixture, past its first sprint
// read and board fetch.
func newLoadHarness(t *testing.T, state domain.AppState) *loadHarness {
	t.Helper()
	h := bootLoadHarness(t, state)
	h.send(h.m.sprintFetchCmd()())
	h.send(makeFetchCmd(context.Background(), h.src, false, nil)())
	return h
}

// bootLoadHarness builds the TUI against the "load" demo fixture, before any sprint read
// or fetch has answered.
func bootLoadHarness(t *testing.T, state domain.AppState) *loadHarness {
	t.Helper()
	cfg := config.DemoConfig()
	a, err := demoadpt.New(demoadpt.Config{Now: time.Now(), Fixture: "load", BaseURL: cfg.GitLab.URL, Latency: -1})
	require.NoError(t, err)
	cfg.CurrentUser = a.Settings().CurrentUser
	cfg.Sources = []config.Source{{Type: "user", IDs: a.Settings().Team}}
	h := &loadHarness{
		src: &recordingSource{MergeRequestSource: a.MRSource()}, store: &recordingStore{state: state},
		demo: a, merged: a.MergedSnapshotStore(),
	}
	h.m = New(context.Background(), cfg, h.src, h.store, a.SnapshotStore(), nil,
		a.Tickets(), nil, nil, "dev", Options{}).WithMergedSnapshots(h.merged)
	h.send(tea.WindowSizeMsg{Width: 160, Height: 30})
	return h
}

func (h *loadHarness) send(msg tea.Msg) tea.Cmd {
	next, cmd := h.m.Update(msg)
	h.m = next.(Model)
	return cmd
}

func (h *loadHarness) press(k string) tea.Cmd {
	return h.send(keyPress(k))
}

// land runs the fetch cmd returns, if any, and delivers its result.
func (h *loadHarness) land(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	msg := findMsg[FetchResultMsg](cmd)
	require.NotNil(t, msg, "no fetch was started")
	h.send(*msg)
}

// findMsg runs cmd and the cmds batched in it concurrently, returning the
// first message of type T. Cmds that do not answer within the wait (tickers)
// are abandoned.
func findMsg[T any](cmd tea.Cmd) *T {
	found := make(chan *T, 1)
	var wg sync.WaitGroup
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch m := c().(type) {
			case T:
				select {
				case found <- &m:
				default:
				}
			case tea.BatchMsg:
				for _, child := range m {
					run(child)
				}
			}
		}()
	}
	run(cmd)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case msg := <-found:
		return msg
	case <-done:
		select {
		case msg := <-found:
			return msg
		default:
			return nil
		}
	case <-time.After(5 * time.Second):
		return nil
	}
}

func keyPress(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

// screen is the rendered screen without styling.
func (h *loadHarness) screen() string { return ansi.Strip(h.m.renderContent()) }

// settle delivers the settings change cmd reports.
func (h *loadHarness) settle(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	msg := findMsg[loadSettingsChangedMsg](cmd)
	require.NotNil(t, msg, "no settings change reported")
	h.send(*msg)
}

func TestLoadView_LToggles_EscReturns_LaunchOpensBoard(t *testing.T) {
	h := newLoadHarness(t, domain.DefaultAppState())
	assert.False(t, h.m.loadOpen, "launch opens the board")
	assert.Contains(t, h.m.baseStack(), BoardCtx)
	assert.NotContains(t, h.m.baseStack(), LoadCtx)

	h.press("L")
	assert.True(t, h.m.loadOpen)
	assert.Equal(t, []*Context{BaseCtx, LoadCtx}, h.m.baseStack())
	assert.Contains(t, h.screen(), "Load Sprint")

	h.press("L")
	assert.False(t, h.m.loadOpen, "L returns to the board")

	h.press("L")
	h.press("esc")
	assert.False(t, h.m.loadOpen, "esc returns to the board")
}

func TestLoadView_MergedSincePassedOnlyWhileOpen(t *testing.T) {
	h := newLoadHarness(t, domain.DefaultAppState())
	sprint, _ := h.m.loadSprint()
	require.NotNil(t, sprint)
	start := sprint.StartDay()

	assert.True(t, h.m.loadMergedSince().IsZero(), "the board fetches no merged MRs")

	h.land(t, h.press("L"))
	h.land(t, h.press("r"))
	h.press("L")
	h.land(t, h.press("r"))

	calls := h.src.calls()
	require.Len(t, calls, 4)
	assert.True(t, calls[0].IsZero(), "boot fetch")
	assert.Equal(t, start, calls[1], "opening fetches with the merged listing")
	assert.Equal(t, start, calls[2], "refreshes keep it while open")
	assert.True(t, calls[3].IsZero(), "closed again: board only")
	assert.Empty(t, domain.MergedMRs(h.m.allMRs), "merged MRs never reach the board")
}

func TestLoadView_OpeningRendersCachedMergedSnapshotFirst(t *testing.T) {
	h := newLoadHarness(t, domain.DefaultAppState())
	sprint, _ := h.m.loadSprint()
	require.NotNil(t, sprint)
	start := sprint.StartDay()
	mrs, errs := h.demo.MRSource().FetchAll(context.Background(), mrsvc.FetchOptions{MergedSince: start})
	require.Empty(t, errs)
	require.NoError(t, h.merged.Save(start, domain.MergedMRs(mrs)))

	cmd := h.press("L")

	assert.Contains(t, h.screen(), "platform-api!702", "the cached merged MR shows before any fetch lands")
	assert.True(t, h.m.isRefreshing)
	h.land(t, cmd)
	assert.Contains(t, h.screen(), "platform-api!702")
}

func TestLoadView_ShowsMergedMRsAfterTheFetchLands(t *testing.T) {
	h := newLoadHarness(t, domain.DefaultAppState())

	h.land(t, h.press("L"))

	screen := h.screen()
	assert.Contains(t, screen, "platform-api!702")
	assert.Contains(t, screen, "platform-api!705")
	assert.NotContains(t, screen, "platform-api!703", "merged before the sprint")
}

func TestLoadView_SprintLandingAfterOpenFetchesTheMergedListing(t *testing.T) {
	h := bootLoadHarness(t, domain.DefaultAppState())
	h.send(FetchResultMsg{}) // the board is up; the sprint read has not answered
	h.m.isRefreshing = true  // a board-only fetch is still in flight

	assert.Nil(t, h.press("L"), "no sprint yet: nothing to list merged MRs since")
	assert.Contains(t, h.screen(), "Reading the active sprint")

	h.land(t, h.send(h.m.sprintFetchCmd()()))

	assert.Contains(t, h.screen(), "platform-api!702")
}

// openedLoad is a harness with the load view open and its merged fetch landed.
func openedLoad(t *testing.T, state domain.AppState) *loadHarness {
	t.Helper()
	h := newLoadHarness(t, state)
	h.land(t, h.press("L"))
	return h
}

func selectedRef(h *loadHarness) string {
	mr := h.m.focusedMR()
	if mr == nil {
		return ""
	}
	return mrRef(*mr)
}

func TestLoadView_KeysChangeSettingsAndPersistThem(t *testing.T) {
	h := openedLoad(t, domain.DefaultAppState())
	assert.Equal(t, "size · by person", h.m.loadView.Settings().indicator())

	h.settle(t, h.press("g"))
	assert.Equal(t, "jira", h.store.state.LoadGroup)
	assert.Contains(t, h.screen(), "LOAD-2  ", "grouped by ticket")

	h.settle(t, h.press("s"))
	assert.Equal(t, "person", h.store.state.LoadSort, "size → person")
	h.settle(t, h.press("s"))
	assert.Equal(t, "jira", h.store.state.LoadSort, "person → jira")
	h.settle(t, h.press("s"))
	assert.Equal(t, "size", h.store.state.LoadSort, "jira → size")

	h.settle(t, h.press("R"))
	assert.True(t, h.store.state.LoadRemaining)
	assert.Contains(t, h.screen(), "remaining only")

	h.press("esc")
	h.press("tab")
	assert.Equal(t, "jira", h.store.state.LoadGroup, "a board save keeps the load settings")
	assert.True(t, h.store.state.LoadRemaining)
}

func TestLoadView_SavedSettingsApplyOnOpen(t *testing.T) {
	state := domain.DefaultAppState()
	state.LoadGroup, state.LoadSort, state.LoadRemaining = "jira", "person", true

	h := openedLoad(t, state)

	s := h.m.loadView.Settings()
	assert.Equal(t, mrsvc.LoadByTicket, s.group)
	assert.Equal(t, mrsvc.LoadSortPerson, s.sort)
	assert.True(t, s.remaining)
	assert.Contains(t, h.screen(), "remaining only")
}

func TestLoadView_SelectionMovesOverMRLinesOnly(t *testing.T) {
	h := openedLoad(t, domain.DefaultAppState())
	// By person, heaviest first: ada (!702, !701), then grace (!702, !705).
	want := []string{"platform-api!702", "platform-api!701", "platform-api!702", "platform-api!705"}
	h.press("up") // the selection stayed on !701, the only line before the merged MRs landed

	got := []string{selectedRef(h)}
	for range 4 {
		h.press("down")
		got = append(got, selectedRef(h))
	}

	assert.Equal(t, append(want, "platform-api!705"), got, "headings are skipped and the last line holds")
	assert.Contains(t, h.screen(), "▸ platform-api!705")
	h.press("up")
	h.press("up")
	assert.Equal(t, "platform-api!701", selectedRef(h))
}

func TestLoadView_DetailAndDiffActOnTheSelectedMR(t *testing.T) {
	h := openedLoad(t, domain.DefaultAppState())
	h.press("down")
	h.press("down")
	h.press("down") // grace's !705

	h.press("enter")
	require.True(t, h.m.showDetail)
	assert.Equal(t, 705, h.m.detail.mr.IID)
	assert.Equal(t, []*Context{BaseCtx, DetailCtx}, h.m.baseStack())

	h.press("esc")
	assert.False(t, h.m.showDetail)
	assert.True(t, h.m.loadOpen, "closing the detail returns to the load view")

	h.press("d")
	require.True(t, h.m.overlay.isDiffView())
	assert.Equal(t, 705, h.m.diffView.mr.IID)
}

func TestLoadView_HeaderCountsTheLoadNotTheBoard(t *testing.T) {
	h := openedLoad(t, domain.DefaultAppState())

	header := strings.SplitN(h.screen(), "\n", 2)[0]

	assert.Contains(t, header, "3 mrs", "open !701 plus merged !702 and !705")
	assert.Contains(t, header, "review load")
	assert.Contains(t, header, "size · by person")
}

const unmovedSHA = "abc"

func TestKeepDiffStats_OnlyWhileTheHeadSHAHolds(t *testing.T) {
	stats := []domain.FileDiffStat{{Path: "a.go", Additions: 3}}
	rules := []domain.GeneratedRule{{}}
	old := domain.MergeRequest{HeadSHA: unmovedSHA, DiffStats: stats, GeneratedRules: rules}

	kept := keepDiffStats(old, domain.MergeRequest{HeadSHA: unmovedSHA, Title: "new"})
	assert.Equal(t, stats, kept.DiffStats)
	assert.Equal(t, rules, kept.GeneratedRules)
	assert.Equal(t, "new", kept.Title)

	moved := keepDiffStats(old, domain.MergeRequest{HeadSHA: "def"})
	assert.Nil(t, moved.DiffStats, "stats of an older SHA describe a diff that no longer exists")

	fresh := []domain.FileDiffStat{{Path: "b.go"}}
	assert.Equal(t, fresh, keepDiffStats(old, domain.MergeRequest{HeadSHA: unmovedSHA, DiffStats: fresh}).DiffStats)
}

func TestReviewerSaveAndUndraft_KeepTheSizeOfAnUnmovedDiff(t *testing.T) {
	stats := []domain.FileDiffStat{{Path: "a.go", Additions: 3}}
	mr := domain.MergeRequest{ProjectID: 1, IID: 7, Title: "x", HeadSHA: unmovedSHA, DiffStats: stats}
	m := makeModel(t, []domain.MergeRequest{mr}, "")

	fromRead := domain.MergeRequest{ProjectID: 1, IID: 7, Title: "x", HeadSHA: unmovedSHA}
	next, _ := m.Update(ReviewersSavedMsg{MR: fromRead})
	m = next.(Model)
	assert.Equal(t, stats, m.AllMRs()[0].DiffStats, "reviewer save")

	next, _ = m.Update(UndraftResultMsg{MR: fromRead})
	m = next.(Model)
	assert.Equal(t, stats, m.AllMRs()[0].DiffStats, "undraft")
}

func TestLoadView_ScrollKeepsTheHeaderAndReachesTheFootnotes(t *testing.T) {
	h := openedLoad(t, domain.DefaultAppState())
	h.send(tea.WindowSizeMsg{Width: 160, Height: 8}) // 6 lines of view: header + 5

	for range 4 {
		h.press("down")
	}

	view := ansi.Strip(h.m.loadView.render())
	lines := strings.Split(view, "\n")
	assert.Len(t, lines, 6)
	assert.Contains(t, lines[0], "Load Sprint", "the tree's header line stays pinned")
	assert.Contains(t, view, "▸ platform-api!705")
	assert.Contains(t, lines[len(lines)-1], "Every approver is credited", "the last line brings the footnote in")

	for range 4 {
		h.press("up")
	}
	assert.Contains(t, ansi.Strip(h.m.loadView.render()), "Ada Lovelace", "the first line brings its heading back")
}

// twoApproverLoad is an open MR !1 under ada and grace, ada's heavier !3, and
// grace's approved !4, which leaves grace's total when only remaining work shows.
func twoApproverLoad(sizeOf1 int) ([]domain.MergeRequest, mrsvc.LoadOptions) {
	mr := func(iid, lines int, reviewers ...domain.ReviewerInfo) domain.MergeRequest {
		return domain.MergeRequest{
			IID: iid, ProjectID: 1, ProjectPath: "g/r", Title: "chore: work", Author: "author",
			Phase: domain.PhaseNeedsReview, Reviewers: reviewers,
			DiffStats: []domain.FileDiffStat{{Path: "work.go", Additions: lines}},
		}
	}
	appr := func(user string, approved bool) domain.ReviewerInfo {
		state := domain.ReviewerNotStarted
		if approved {
			state = domain.ReviewerApproved
		}
		return domain.ReviewerInfo{Username: user, Name: user, IsApprover: true, State: state}
	}
	mrs := []domain.MergeRequest{
		mr(1, sizeOf1, appr("ada", false), appr("grace", false)),
		mr(3, 500, appr("ada", false)),
		mr(4, 10, appr("grace", true)),
	}
	opts := mrsvc.LoadOptions{
		SprintStart: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		Team:        []string{"ada", "grace"},
		Exclusions:  func(domain.MergeRequest) domain.SizeExclusions { return domain.NewSizeExclusions(true, nil, nil) },
		Thresholds:  domain.DefaultSizeThresholds,
	}
	return mrs, opts
}

func TestLoadView_SelectionKeepsItsGroupWhenTheHeadingChanges(t *testing.T) {
	w := newLoadViewWidget(NewLoadStyles("", "dark"), DefaultLoadKeyMap, loadSettingsFromState(domain.DefaultAppState()))
	w.SetSize(120, 30)
	mrs, opts := twoApproverLoad(100)
	w.SetData(mrs, opts, LoadSprint{Name: "S"})
	selectedIn := func() (int, string) {
		require.GreaterOrEqual(t, w.sel, 0)
		l := w.lines[w.selectable[w.sel]]
		return l.entry.MR.IID, l.group.key
	}
	for range 3 {
		if iid, group := selectedIn(); iid == 1 && group == "grace" {
			break
		}
		next, _ := w.Update(keyPress("down"))
		w = next.(loadViewWidget)
	}
	iid, group := selectedIn()
	require.Equal(t, 1, iid)
	require.Equal(t, "grace", group, "the second copy of !1 is selected")

	for _, k := range []string{"R", "s", "R", "s"} {
		next, _ := w.Update(keyPress(k))
		w = next.(loadViewWidget)
		iid, group = selectedIn()
		assert.Equal(t, 1, iid, "after %s", k)
		assert.Equal(t, "grace", group, "after %s", k)
	}

	mrs, opts = twoApproverLoad(120) // a refresh changes the group totals
	w.SetData(mrs, opts, LoadSprint{Name: "S"})
	iid, group = selectedIn()
	assert.Equal(t, 1, iid, "after a refresh")
	assert.Equal(t, "grace", group, "after a refresh")
}
