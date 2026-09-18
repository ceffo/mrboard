package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

func newTestSettingsWidget() settingsWidget {
	authors := []string{"asmith", "cwu"}
	reviewers := []string{"bwilson", "jpdubois"}
	tickets := []TicketKeyCount{{Key: "OD-100", Count: 2}, {Key: "OD-200", Count: 1}}
	userMap := map[string]string{
		"asmith":   "Alexandra Smith",
		"jpdubois": "Jean-Philippe Dubois",
	}
	styles := NewStyles(LoadThemeByName("default"), true)
	viewState := filterViewWidget{myMRsAvail: true, sprintAvail: true}
	return newSettingsWidget(
		[]string{"default", "solarized"},
		authors, reviewers,
		tickets, 3, 6,
		userMap,
		domain.FilterCriteria{},
		viewState,
		false,
		sortByRepoIID, false,
		"default", themeModeAuto,
		styles, DefaultSettingsKeyMap,
		tabGeneral,
	)
}

// TestSettingsWidget_FrameSizeConsistentAcrossTabs locks in the fix for the
// settings modal's header jumping on every tab switch: each tab's body used
// to size the shared PopupBorder independently, so the frame (and the tab
// bar above it) moved on every tab press. render() now places every tab's
// body into one shared canvas, so the rendered frame must be identical
// regardless of which tab is active.
func TestSettingsWidget_FrameSizeConsistentAcrossTabs(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
	}{
		{"tall terminal", 120, 40},
		{"short terminal", 100, 14},
		{"terminal narrower than Filters' column-width floor", 40, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestSettingsWidget()
			w.SetSize(tc.width, tc.height)

			var wantW, wantH int
			for tab := settingsTab(0); tab < numSettingsTabs; tab++ {
				w.tab = tab
				out := w.render()
				gotW, gotH := lip.Width(out), lip.Height(out)
				require.Greater(t, gotW, 0, "tab %d rendered empty", tab)
				if tab == 0 {
					wantW, wantH = gotW, gotH
					continue
				}
				assert.Equal(t, wantW, gotW, "tab %d width differs from tab 0", tab)
				assert.Equal(t, wantH, gotH, "tab %d height differs from tab 0", tab)
			}
		})
	}
}

// TestSettingsWidget_ToggleCompact_AssigneeColumn verifies the n keybinding
// swaps the full display name for the bare "@username" in the Assignee
// column, and only there — Toggle is filters-tab-scoped.
func TestSettingsWidget_ToggleCompact_AssigneeColumn(t *testing.T) {
	w := newTestSettingsWidget()
	w.SetSize(120, 40)
	w.tab = tabFilters

	full := w.render()
	assert.Contains(t, full, "Alexandra Smith")

	updated, _ := w.Update(tea.KeyPressMsg{Text: "n", Code: 'n'})
	w = updated.(settingsWidget) //nolint:forcetypeassert

	compact := w.render()
	assert.NotContains(t, compact, "Alexandra Smith")
	assert.Contains(t, compact, "@asmith")
}

// TestSettingsWidget_FilterView_TogglesRoundTrip verifies the View strip's
// checkboxes flip on space and round-trip through buildApplied — the path
// handleSettingsApplied reads to update Model.viewMode/sprintFilterActive.
func TestSettingsWidget_FilterView_TogglesRoundTrip(t *testing.T) {
	w := newTestSettingsWidget()
	w.SetSize(120, 40)
	w.tab = tabFilters
	w.filterFocused = filterFocusView

	applied := w.buildApplied()
	require.False(t, applied.ViewMine)
	require.False(t, applied.SprintFilter)

	updated, _ := w.Update(tea.KeyPressMsg{Text: " ", Code: ' '})
	w = updated.(settingsWidget) //nolint:forcetypeassert
	assert.True(t, w.buildApplied().ViewMine, "toggling the first View row should set ViewMine")

	w.filterView.cursor = 1
	updated, _ = w.Update(tea.KeyPressMsg{Text: " ", Code: ' '})
	w = updated.(settingsWidget) //nolint:forcetypeassert
	assert.True(t, w.buildApplied().SprintFilter, "toggling the second View row should set SprintFilter")
}

// TestSettingsWidget_FilterView_OmittedWhenUnavailable verifies a toggle
// disappears from the strip (and can't be landed on via keyboard focus)
// when its underlying mechanism isn't available for this deployment — no
// current user configured, no JIRA board configured.
func TestSettingsWidget_FilterView_OmittedWhenUnavailable(t *testing.T) {
	w := newTestSettingsWidget()
	w.filterView = filterViewWidget{} // neither available
	w.SetSize(120, 40)
	w.tab = tabFilters

	out := w.render()
	assert.NotContains(t, out, "My MRs only")
	assert.NotContains(t, out, "Current sprint")

	// Moving down from Status must skip straight to the list columns.
	w.moveVerticalFilters(1)
	assert.Equal(t, filterFocusAssignee, w.filterFocused)
}
