// Package tui provides the Bubble Tea TUI for mrboard.
package tui

import (
	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

const (
	numColumns  = 4
	minColWidth = 10
)

type boardWidget struct {
	columns    [4]columnWidget
	focusedCol int
	styles     Styles
	width      int
	height     int
}

var phaseOrder = [4]domain.MRPhase{
	domain.PhaseDraft,
	domain.PhaseNeedsReview,
	domain.PhaseNeedsAuthorAction,
	domain.PhaseReadyToMerge,
}

func newBoardWidget(styles Styles, width, height int) boardWidget {
	b := boardWidget{styles: styles, width: width, height: height}
	colWidth := max(width/numColumns, minColWidth)
	for i, phase := range phaseOrder {
		b.columns[i] = newColumnWidget(phase, styles, colWidth, height)
		b.columns[i].SetActive(true)
	}
	b.columns[0].SetFocused(true)
	return b
}

// SetActive marks the board as owning keyboard focus (true) or yielding it to
// a panel (false). The focused column's card renders a dimmed highlight when inactive.
func (b *boardWidget) SetActive(v bool) {
	for i := range b.columns {
		b.columns[i].SetActive(v)
	}
}

func (b *boardWidget) SetSize(width, height int) {
	b.width = width
	b.height = height
	colWidth := max(width/numColumns, minColWidth)
	for i := range b.columns {
		b.columns[i].SetWidth(colWidth)
		b.columns[i].SetHeight(height)
	}
}

func (b *boardWidget) SetMRs(mrs []domain.MergeRequest) {
	var byPhase [numColumns][]domain.MergeRequest
	for _, mr := range mrs {
		if !hasAssignedReviewer(mr) {
			continue
		}
		idx := int(mr.Phase)
		if idx >= 0 && idx < numColumns {
			byPhase[idx] = append(byPhase[idx], mr)
		}
	}
	for i := range b.columns {
		b.columns[i].SetCards(byPhase[i])
	}
	b.setInitialFocus()
}

// hasAssignedReviewer reports whether mr has at least one reviewer with a non-empty username.
func hasAssignedReviewer(mr domain.MergeRequest) bool {
	for _, r := range mr.Reviewers {
		if r.Username != "" {
			return true
		}
	}
	return false
}

func (b *boardWidget) setInitialFocus() {
	for i := range b.columns {
		if len(b.columns[i].cards) > 0 {
			b.setFocusedCol(i)
			return
		}
	}
	b.setFocusedCol(0)
}

func (b *boardWidget) setFocusedCol(idx int) {
	b.columns[b.focusedCol].SetFocused(false)
	b.focusedCol = idx
	b.columns[b.focusedCol].SetFocused(true)
}

func (b *boardWidget) MoveLeft() {
	if b.focusedCol > 0 {
		prevIdx := b.columns[b.focusedCol].focusIdx
		b.setFocusedCol(b.focusedCol - 1)
		b.columns[b.focusedCol].ClampFocusTo(prevIdx)
	}
}

func (b *boardWidget) MoveRight() {
	if b.focusedCol < numColumns-1 {
		prevIdx := b.columns[b.focusedCol].focusIdx
		b.setFocusedCol(b.focusedCol + 1)
		b.columns[b.focusedCol].ClampFocusTo(prevIdx)
	}
}

func (b *boardWidget) MoveUp()   { b.columns[b.focusedCol].MoveUp() }
func (b *boardWidget) MoveDown() { b.columns[b.focusedCol].MoveDown() }

func (b *boardWidget) FocusedMR() *domain.MergeRequest {
	return b.columns[b.focusedCol].FocusedMR()
}

func (b boardWidget) Init() tea.Cmd                         { return nil }
func (b boardWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return b, nil }
func (b boardWidget) View() tea.View                        { return tea.NewView(b.render()) }

func (b boardWidget) render() string {
	cols := make([]string, numColumns)
	for i := range b.columns {
		cols[i] = b.columns[i].render()
	}
	return lip.JoinHorizontal(lip.Top, cols...)
}
