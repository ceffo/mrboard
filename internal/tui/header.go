package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

type headerWidget struct {
	styles Styles
	mrs    []domain.MergeRequest
	width  int
	title  string
}

func newHeaderWidget(styles Styles) headerWidget {
	return headerWidget{styles: styles, title: "mrboard"}
}

func (h *headerWidget) SetWidth(w int)                   { h.width = w }
func (h *headerWidget) SetMRs(mrs []domain.MergeRequest) { h.mrs = mrs }
func (h *headerWidget) SetTitle(t string)                { h.title = t }

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

	title := h.styles.HeaderTitle.Render(h.title)
	stats := h.styles.HeaderStats.Render(fmt.Sprintf(
		"Draft:%d  Review:%d  Author:%d  Ready:%d  Total:%d",
		counts[0], counts[1], counts[2], counts[3], len(h.mrs),
	))

	titleW := lip.Width(title)
	statsW := lip.Width(stats)

	// Build the row as plain text with inline fg-only ANSI codes; the outer
	// Header style applies the background uniformly to the whole line.
	// No Width() used — avoids lipgloss word-wrap bug.
	if h.width <= titleW+statsW+1 {
		return h.styles.Header.Render(title + " " + stats)
	}
	leftPad := (h.width - titleW) / 2 //nolint:mnd
	gap := h.width - leftPad - titleW - statsW
	if gap < 1 {
		gap = 1
	}
	row := strings.Repeat(" ", leftPad) + title + strings.Repeat(" ", gap) + stats
	return h.styles.Header.Render(row)
}
