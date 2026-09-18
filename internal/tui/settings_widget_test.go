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
	return newSettingsWidget(
		[]string{"default", "solarized"},
		authors, reviewers,
		tickets, 3, 6,
		userMap,
		domain.FilterCriteria{},
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
