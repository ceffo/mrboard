package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

type headerWidget struct {
	styles       Styles
	mrs          []domain.MergeRequest
	width        int
	title        string
	filterActive bool
}

func newHeaderWidget(styles Styles) headerWidget {
	return headerWidget{styles: styles, title: "mrboard"}
}

func (h *headerWidget) SetStyles(s Styles)               { h.styles = s }
func (h *headerWidget) SetWidth(w int)                   { h.width = w }
func (h *headerWidget) SetMRs(mrs []domain.MergeRequest) { h.mrs = mrs }
func (h *headerWidget) SetTitle(t string)                { h.title = t }
func (h *headerWidget) SetFilterActive(v bool)           { h.filterActive = v }

func (h headerWidget) Init() tea.Cmd                         { return nil }
func (h headerWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return h, nil }
func (h headerWidget) View() tea.View                        { return tea.NewView(h.render()) }

func (h headerWidget) render() string {
	counts := [4]int{}
	for _, mr := range h.mrs {
		if idx := int(mr.Phase); idx >= 0 && idx < 4 {
			counts[idx]++
		}
	}

	// Inherit propagates the background color to each segment so the full
	// header line carries a uniform background without a wrapping Render call
	// (which would be broken by inner ANSI resets from nested renders).
	bg := h.styles.Header
	title := h.styles.HeaderTitle.Inherit(bg).Render(h.title)
	stats := h.styles.HeaderStats.Inherit(bg).Render(fmt.Sprintf(
		"Draft:%d  Review:%d  Author:%d  Ready:%d  Total:%d",
		counts[0], counts[1], counts[2], counts[3], len(h.mrs),
	))
	if h.filterActive {
		stats += bg.Render("  ") + h.styles.FilterActive.Inherit(bg).Render("[filtered]")
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
