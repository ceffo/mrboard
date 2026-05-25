package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

const (
	colBorderWidth = 2
	colChrome      = 3 // top-border(1) + header-line(1) + bottom-border(1)
)

type columnWidget struct {
	phase        domain.MRPhase
	name         string
	cards        []cardWidget
	cardHeights  []int // rendered line-count per card; kept in sync with cards
	focused      bool
	active       bool // true when the board (not a panel) owns keyboard focus
	focusIdx     int
	scrollOffset int
	styles       Styles
	width        int
	height       int
}

func newColumnWidget(phase domain.MRPhase, styles Styles, width, height int) columnWidget {
	return columnWidget{
		phase:  phase,
		name:   phaseName(phase),
		styles: styles,
		width:  width,
		height: height,
	}
}

func phaseName(p domain.MRPhase) string {
	switch p {
	case domain.PhaseDraft:
		return phaseLabelDraft
	case domain.PhaseNeedsReview:
		return phaseLabelReview
	case domain.PhaseNeedsAuthorAction:
		return phaseLabelAuthorAc
	case domain.PhaseReadyToMerge:
		return phaseLabelReady
	default:
		return "Unknown"
	}
}

func (c *columnWidget) SetFocused(v bool) {
	c.focused = v
	c.syncCardFocus()
}

func (c *columnWidget) SetActive(v bool) {
	c.active = v
	c.syncCardFocus()
}

func (c *columnWidget) SetWidth(w int) {
	c.width = w
	for i := range c.cards {
		c.cards[i].SetWidth(w - colBorderWidth)
		c.cardHeights[i] = c.cards[i].measureHeight(w - colBorderWidth)
	}
}

func (c *columnWidget) SetHeight(h int) {
	c.height = h
	c.clampScroll()
}

func (c *columnWidget) SetCards(mrs []domain.MergeRequest) {
	c.cards = make([]cardWidget, len(mrs))
	c.cardHeights = make([]int, len(mrs))
	for i, mr := range mrs {
		c.cards[i] = newCardWidget(mr, c.styles, c.width-colBorderWidth)
		c.cardHeights[i] = c.cards[i].measureHeight(c.width - colBorderWidth)
	}
	c.scrollOffset = 0
	if len(c.cards) == 0 {
		c.focusIdx = 0
	} else if c.focusIdx >= len(c.cards) {
		c.focusIdx = len(c.cards) - 1
	}
	c.syncCardFocus()
	c.clampScroll()
}

func (c *columnWidget) syncCardFocus() {
	for i := range c.cards {
		isFocused := c.focused && i == c.focusIdx
		c.cards[i].SetFocused(isFocused)
		c.cards[i].SetFocusInactive(isFocused && !c.active)
	}
}

func (c *columnWidget) MoveUp() {
	if len(c.cards) == 0 {
		return
	}
	c.focusIdx--
	if c.focusIdx < 0 {
		c.focusIdx = len(c.cards) - 1
	}
	c.syncCardFocus()
	c.clampScroll()
}

func (c *columnWidget) MoveDown() {
	if len(c.cards) == 0 {
		return
	}
	c.focusIdx++
	if c.focusIdx >= len(c.cards) {
		c.focusIdx = 0
	}
	c.syncCardFocus()
	c.clampScroll()
}

func (c *columnWidget) ClampFocusTo(idx int) {
	c.focusIdx = idx
	if len(c.cards) > 0 && c.focusIdx >= len(c.cards) {
		c.focusIdx = len(c.cards) - 1
	}
	c.syncCardFocus()
	c.clampScroll()
}

func (c *columnWidget) FocusedMR() *domain.MergeRequest {
	if len(c.cards) == 0 || c.focusIdx >= len(c.cards) {
		return nil
	}
	mr := c.cards[c.focusIdx].mr
	return &mr
}

// cardAreaH is the number of lines available for cards inside the column border.
func (c columnWidget) cardAreaH() int {
	h := c.height - colChrome
	if h < 0 {
		return 0
	}
	return h
}

// visibleEnd returns the exclusive end index of cards visible from scrollOffset.
func (c columnWidget) visibleEnd() int {
	end := c.scrollOffset
	remaining := c.cardAreaH()
	for end < len(c.cardHeights) && remaining >= c.cardHeights[end] {
		remaining -= c.cardHeights[end]
		end++
	}
	return end
}

// clampScroll adjusts scrollOffset so focusIdx is always within the visible range.
func (c *columnWidget) clampScroll() {
	if len(c.cards) == 0 || c.cardAreaH() <= 0 {
		c.scrollOffset = 0
		return
	}
	if c.focusIdx < c.scrollOffset {
		c.scrollOffset = c.focusIdx
		return
	}
	for {
		end := c.visibleEnd()
		if c.focusIdx < end {
			break
		}
		c.scrollOffset++
	}
}

func (c columnWidget) Init() tea.Cmd                         { return nil }
func (c columnWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return c, nil }
func (c columnWidget) View() tea.View                        { return tea.NewView(c.render()) }

func (c columnWidget) render() string {
	contentW := c.width - colBorderWidth

	visEnd := c.visibleEnd()
	hasAbove := c.scrollOffset > 0
	hasBelow := visEnd < len(c.cards)

	// Header: label left, scroll indicator right.
	label := c.styles.ColumnHeader.Render(fmt.Sprintf("%s (%d)", c.name, len(c.cards)))
	ind := c.styles.ScrollIndicator.Render(scrollIndicator(hasAbove, hasBelow))
	labelW := lip.Width(label)
	indW := lip.Width(ind)
	gap := contentW - labelW - indW
	if gap < 0 {
		gap = 0
	}
	header := label + strings.Repeat(" ", gap) + ind

	// Visible cards only.
	var cardLines []string
	if len(c.cards) == 0 {
		empty := c.styles.EmptyColumn.Render("(empty)")
		if w := lip.Width(empty); w < contentW {
			empty += strings.Repeat(" ", contentW-w)
		}
		cardLines = append(cardLines, empty)
	} else {
		for i := c.scrollOffset; i < visEnd; i++ {
			cardLines = append(cardLines, c.cards[i].render())
		}
	}

	content := header + "\n" + strings.Join(cardLines, "\n")

	// Hard-cap content to the available inner height so Height() only ever
	// pads (never overflows when cards fill the area exactly).
	if c.height > colBorderWidth {
		innerH := c.height - colBorderWidth
		cl := strings.SplitN(content, "\n", innerH+1)
		if len(cl) > innerH {
			cl = cl[:innerH]
		}
		content = strings.Join(cl, "\n")
	}

	var borderStyle lip.Style
	switch {
	case c.focused && !c.active:
		borderStyle = c.styles.ColumnBorderFocusedInactive
	case c.focused:
		borderStyle = c.styles.ColumnBorderFocused
	default:
		borderStyle = c.styles.ColumnBorder
	}
	if c.height > colBorderWidth {
		borderStyle = borderStyle.Height(c.height)
	}
	return borderStyle.Render(content)
}

func scrollIndicator(hasAbove, hasBelow bool) string {
	switch {
	case hasAbove && hasBelow:
		return "↑↓"
	case hasAbove:
		return "↑ "
	case hasBelow:
		return " ↓"
	default:
		return "  "
	}
}
