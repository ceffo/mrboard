package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

type headerWidget struct {
	styles             Styles
	mrs                []domain.MergeRequest
	width              int
	title              string
	filterActive       bool
	sprintFilterActive bool
	statsOverride      string
}

func newHeaderWidget(styles Styles) headerWidget {
	return headerWidget{styles: styles, title: "mrboard"}
}

func (h *headerWidget) SetStyles(s Styles)               { h.styles = s }
func (h *headerWidget) SetWidth(w int)                   { h.width = w }
func (h *headerWidget) SetMRs(mrs []domain.MergeRequest) { h.mrs = mrs }
func (h *headerWidget) SetTitle(t string)                { h.title = t }
func (h *headerWidget) SetFilterActive(v bool)           { h.filterActive = v }
func (h *headerWidget) SetSprintFilterActive(v bool)     { h.sprintFilterActive = v }
func (h *headerWidget) SetStats(s string)                { h.statsOverride = s }

func (h headerWidget) Init() tea.Cmd                         { return nil }
func (h headerWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return h, nil }
func (h headerWidget) View() tea.View                        { return tea.NewView(h.render()) }

func (h headerWidget) render() string {
	bg := h.styles.Header
	title := h.styles.HeaderTitle.Inherit(bg).Render(h.title)
	statsStr := fmt.Sprintf("Total:%d", len(h.mrs))
	if h.statsOverride != "" {
		statsStr = h.statsOverride
	}
	stats := h.styles.HeaderStats.Inherit(bg).Render(statsStr)
	if h.filterActive {
		stats += bg.Render("  ") + h.styles.FilterActive.Inherit(bg).Render("[filtered]")
	}
	if h.sprintFilterActive {
		stats += bg.Render("  ") + h.styles.FilterActive.Inherit(bg).Render("[sprint]")
	}

	titleW := lip.Width(title)
	statsW := lip.Width(stats)

	if h.width <= titleW+statsW+1 {
		return title + bg.Render(" ") + stats
	}
	leftPad := (h.width - titleW) / 2 //nolint:mnd
	gap := h.width - leftPad - titleW - statsW
	if gap < 1 {
		gap = 1
	}
	return bg.Render(strings.Repeat(" ", leftPad)) +
		title +
		bg.Render(strings.Repeat(" ", gap)) +
		stats
}
