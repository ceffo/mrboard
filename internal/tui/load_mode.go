package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/ticketsvc"
)

// The root model's side of the review load view (docs/adr/0015, "TUI display
// mode"): opening and closing it, the merged listing it adds to fetches, and
// the inputs it computes the load from. The view itself is loadViewWidget.

// WithMergedSnapshots persists the merged MRs the load view lists, so opening
// it renders the sprint's merged MRs at once instead of after a fetch.
func (m Model) WithMergedSnapshots(store domain.MergedSnapshotStore) Model {
	m.mergedStore = store
	return m
}

// focusedMR is the MR the MR actions apply to: the load view's selected line
// while it is open, else the board's focused card.
func (m Model) focusedMR() *domain.MergeRequest {
	if m.loadOpen {
		return m.loadView.SelectedMR()
	}
	return m.board.FocusedMR()
}

// loadSprint returns the sprint the load is scoped to, or why there is none.
// A sprint without issue keys or a start date cannot scope a load: every
// ticketed MR would drop out, or merged MRs could not be bounded.
func (m Model) loadSprint() (*ticketsvc.Sprint, string) {
	s := m.sprint
	switch {
	case s == nil && !m.sprintRead:
		return nil, "Reading the active sprint…"
	case s == nil && m.sprintErr != nil:
		return nil, "Could not read the active sprint: " + m.sprintErr.Error()
	case s == nil:
		return nil, "The board has no active sprint, so there is no load to show."
	case len(s.IssueKeys) == 0:
		return nil, fmt.Sprintf("Sprint %q has no issues, so there is no load to show.", s.Name)
	case s.Start.IsZero():
		return nil, fmt.Sprintf("Sprint %q reports no start date, so merged MRs cannot be bounded.", s.Name)
	}
	return s, ""
}

// loadMergedSince is the merged listing a fetch adds: the sprint start while
// the load view is open, zero otherwise, so board-only use costs no request.
func (m Model) loadMergedSince() time.Time {
	if !m.loadOpen {
		return time.Time{}
	}
	s, _ := m.loadSprint()
	if s == nil {
		return time.Time{}
	}
	return s.StartDay()
}

// openLoad shows the load view with the cached merged snapshot, then fetches
// with the merged listing.
func (m *Model) openLoad() tea.Cmd {
	m.loadOpen = true
	m.loadView.SetFocused(true)
	m.resizeBoard()
	since := m.loadMergedSince()
	if since.IsZero() {
		m.refreshLoad()
		return nil
	}
	if !since.Equal(m.mergedSince) {
		m.restoreMerged(since)
	}
	m.refreshLoad()
	m.isRefreshing = true
	return tea.Batch(m.sp.Init(), m.startFetch())
}

func (m *Model) closeLoad() {
	m.loadOpen = false
	m.resizeBoard()
}

// syncLoadSprint follows a sprint change while the view is open: the merged
// MRs of the new sprint replace the old ones, cached first, then fetched. The
// fetch supersedes one in flight, which lists no merged MRs for this sprint.
func (m *Model) syncLoadSprint() tea.Cmd {
	since := m.loadMergedSince()
	if since.IsZero() || since.Equal(m.mergedSince) {
		return nil
	}
	m.restoreMerged(since)
	m.isRefreshing = true
	return tea.Batch(m.sp.Init(), m.startFetch())
}

// restoreMerged replaces the merged MRs in memory with the snapshot listed
// for since; a missing or unreadable snapshot leaves none.
func (m *Model) restoreMerged(since time.Time) {
	m.mergedMRs = nil
	m.mergedSince = since
	if m.mergedStore == nil {
		return
	}
	mrs, err := m.mergedStore.Load(since)
	if err != nil {
		m.logger.Error("mergedsnapshot: load failed", "err", err)
		return
	}
	m.mergedMRs = mrs
}

// landMerged keeps the merged MRs a fetch listed and persists them, unless the
// fetch failed with none: an empty failed listing would wipe the sprint's
// cached merged MRs.
func (m *Model) landMerged(msg FetchResultMsg) {
	if msg.MergedSince.IsZero() {
		return
	}
	merged := domain.MergedMRs(msg.MRs)
	if len(merged) == 0 && len(msg.Errors) > 0 {
		return
	}
	m.mergedMRs = merged
	m.mergedSince = msg.MergedSince
	if m.mergedStore == nil {
		return
	}
	if err := m.mergedStore.Save(msg.MergedSince, merged); err != nil {
		m.logger.Error("mergedsnapshot: save failed", "err", err)
	}
}

// refreshLoad recomputes the open view from the current MRs, sprint and
// header filters. Every filter but sprint applies, as in `mrboard load`.
func (m *Model) refreshLoad() {
	if !m.loadOpen {
		return
	}
	sprint, status := m.loadSprint()
	if sprint == nil {
		m.loadView.SetStatus(status)
		return
	}
	start := sprint.StartDay()
	mrs := m.inFetchScope(m.allMRs)
	if start.Equal(m.mergedSince) {
		mrs = append(append([]domain.MergeRequest(nil), mrs...), m.inFetchScope(m.mergedMRs)...)
	}
	keys := make(map[string]bool, len(sprint.IssueKeys))
	for _, k := range sprint.IssueKeys {
		keys[k] = true
	}
	rs := m.cfg.ReviewSize
	m.loadView.SetData(mrs, mrsvc.LoadOptions{
		SprintStart: start,
		SprintKeys:  keys,
		KeyMatcher:  m.keyMatcher,
		Team:        config.TeamUsernames(m.cfg.Sources),
		Filter:      m.filterOptions(),
		Exclusions:  rs.ExclusionsFor(),
		Thresholds:  rs.Buckets.Thresholds(),
	}, LoadSprint{Name: sprint.Name, Start: start, End: sprint.End})
}

// handleKeyLoad handles keys while the load view owns focus. Selection and
// the view's settings belong to the widget; the MR actions are the board's.
func (m Model) handleKeyLoad(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.loadKeys
	switch {
	case k.Close.Match(msg):
		m.closeLoad()
		return m, nil
	case k.Detail.Match(msg):
		return m.requireFocusedMR("detail", func(mr *domain.MergeRequest) (tea.Model, tea.Cmd) {
			m.openDetail(mr)
			return m, m.fetchDetailCmd(mr)
		})
	case k.Open.Match(msg):
		return m.requireFocusedMR("open", func(mr *domain.MergeRequest) (tea.Model, tea.Cmd) {
			return m, openBrowser(mr.WebURL)
		})
	case k.CopyURL.Match(msg):
		return m.requireFocusedMR("copy URL", func(mr *domain.MergeRequest) (tea.Model, tea.Cmd) {
			return m, copyURLCmd(mr.WebURL)
		})
	case k.Diff.Match(msg):
		return m.requireFocusedMR("diff", func(mr *domain.MergeRequest) (tea.Model, tea.Cmd) {
			return m, m.openDiffView(mr)
		})
	case k.Refresh.Match(msg):
		return m.manualRefresh()
	case k.ToggleView.Match(msg):
		m.toggleView()
		return m, nil
	}
	updated, cmd := m.loadView.Update(msg)
	m.loadView = updated.(loadViewWidget) //nolint:forcetypeassert // loadViewWidget.Update always returns loadViewWidget
	return m, cmd
}

// loadHeader is the header while the load view is open: its count is the
// load's MRs in scope against the sprint's population, and the sprint filter
// is not shown, since sprint scoping is what the view is.
func (m Model) loadHeader() headerWidget {
	h := m.header
	h.SetTitle("review load")
	h.SetSort(m.loadView.Settings().indicator())
	state := m.headerFilterState()
	state.Sprint = false
	load, ready := m.loadView.Load()
	if !ready {
		load = mrsvc.Load{}
	}
	h.SetCount(load.MRCount, load.PopulationCount)
	h.SetFilterState(load.PopulationCount, state)
	return h
}

// keepDiffStats carries the size inputs of old into updated when updated
// comes from a single-MR read that has none and its diff has not moved: the
// size is then unchanged, and dropping it would show an unknown size until
// the next fetch.
func keepDiffStats(old, updated domain.MergeRequest) domain.MergeRequest {
	if updated.DiffStats != nil || updated.HeadSHA != old.HeadSHA {
		return updated
	}
	updated.DiffStats = old.DiffStats
	if updated.GeneratedRules == nil && !updated.GeneratedRulesUnknown {
		updated.GeneratedRules = old.GeneratedRules
		updated.GeneratedRulesUnknown = old.GeneratedRulesUnknown
	}
	return updated
}
