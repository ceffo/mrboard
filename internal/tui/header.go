package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"
	"github.com/mrboard/mrboard/internal/domain"
)

type headerWidget struct {
	styles Styles
	mrs    []domain.MergeRequest
	width  int
}

func newHeaderWidget(styles Styles) headerWidget {
	return headerWidget{styles: styles}
}

func (h *headerWidget) SetWidth(w int)                   { h.width = w }
func (h *headerWidget) SetMRs(mrs []domain.MergeRequest) { h.mrs = mrs }

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

	const appName = "mrboard"
	stats := fmt.Sprintf("Draft:%d  Review:%d  Author:%d  Ready:%d  Total:%d",
		counts[0], counts[1], counts[2], counts[3], len(h.mrs))

	titleW := lip.Width(appName)
	statsW := lip.Width(stats)

	// Build row manually (no Width() style — avoids lipgloss word-wrap bug).
	// Title is centered; stats are right-aligned.
	var row string
	if h.width <= titleW+statsW+1 {
		row = appName + " " + stats
	} else {
		leftPad := (h.width - titleW) / 2 //nolint:mnd
		gap := h.width - leftPad - titleW - statsW
		if gap < 1 {
			gap = 1
		}
		row = strings.Repeat(" ", leftPad) + appName + strings.Repeat(" ", gap) + stats
	}

	return h.styles.Header.Render(row)
}
