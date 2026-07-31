package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// ageJustNow is the threshold below which the snapshot age reads "just now"
// rather than domain.FormatDuration's "< 1m".
const ageJustNow = 10 * time.Second

type headerWidget struct {
	styles             Styles
	mrs                []domain.MergeRequest
	width              int
	title              string
	filterActive       bool
	sprintFilterActive bool
	statsOverride      string
	sortIndicator      string // current sort mode, e.g. "repo·id↑"; state moved out of key labels
	snapshotWrittenAt  time.Time
	refreshing         bool
	spinnerFrame       string
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
func (h *headerWidget) SetSort(label string)             { h.sortIndicator = label }

// SetSnapshotAge records when the currently displayed board data was captured
// and whether a fetch is in flight, so render can show "⠿ 14m ago" collapsing
// to "just now" the moment a swap lands (docs/adr/0005, "Non-blocking refresh").
// A zero writtenAt (nothing cached yet) renders no age segment at all.
func (h *headerWidget) SetSnapshotAge(writtenAt time.Time, refreshing bool, spinnerFrame string) {
	h.snapshotWrittenAt = writtenAt
	h.refreshing = refreshing
	h.spinnerFrame = spinnerFrame
}

func (h headerWidget) ageLabel() string {
	if h.snapshotWrittenAt.IsZero() {
		return ""
	}
	age := time.Since(h.snapshotWrittenAt)
	label := "just now"
	if age >= ageJustNow {
		label = domain.FormatDuration(age) + " ago"
	}
	if h.refreshing {
		return h.spinnerFrame + " " + label
	}
	return label
}

func (h headerWidget) Init() tea.Cmd                         { return nil }
func (h headerWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return h, nil }
func (h headerWidget) View() tea.View                        { return tea.NewView(h.render()) }

func (h headerWidget) render() string {
	bg := h.styles.Header
	title := h.styles.HeaderTitle.Inherit(bg).Render(h.title)
	statsStr := fmt.Sprintf("Total:%d", len(h.mrs))
	if h.sortIndicator != "" {
		statsStr = "sort " + h.sortIndicator + "  " + statsStr
	}
	if age := h.ageLabel(); age != "" {
		statsStr = age + "  " + statsStr
	}
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
