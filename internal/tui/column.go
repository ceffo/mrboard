package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"
	"github.com/mrboard/mrboard/internal/domain"
)

const colBorderWidth = 2

type columnWidget struct {
	phase    domain.MRPhase
	name     string
	cards    []cardWidget
	focused  bool
	focusIdx int
	styles   Styles
	width    int
	height   int
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
		return "Draft"
	case domain.PhaseNeedsReview:
		return "Needs Review"
	case domain.PhaseNeedsAuthorAction:
		return "Needs Author Action"
	case domain.PhaseReadyToMerge:
		return "Ready to Merge"
	default:
		return "Unknown"
	}
}

func (c *columnWidget) SetFocused(v bool) {
	c.focused = v
	c.syncCardFocus()
}

func (c *columnWidget) SetWidth(w int) {
	c.width = w
	for i := range c.cards {
		c.cards[i].SetWidth(w - colBorderWidth)
	}
}

func (c *columnWidget) SetHeight(h int) { c.height = h }

func (c *columnWidget) SetCards(mrs []domain.MergeRequest) {
	c.cards = make([]cardWidget, len(mrs))
	for i, mr := range mrs {
		c.cards[i] = newCardWidget(mr, c.styles, c.width-colBorderWidth)
	}
	if len(c.cards) == 0 {
		c.focusIdx = 0
	} else if c.focusIdx >= len(c.cards) {
		c.focusIdx = len(c.cards) - 1
	}
	c.syncCardFocus()
}

func (c *columnWidget) syncCardFocus() {
	for i := range c.cards {
		c.cards[i].SetFocused(c.focused && i == c.focusIdx)
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
}

func (c *columnWidget) ClampFocusTo(idx int) {
	c.focusIdx = idx
	if len(c.cards) > 0 && c.focusIdx >= len(c.cards) {
		c.focusIdx = len(c.cards) - 1
	}
	c.syncCardFocus()
}

func (c *columnWidget) FocusedMR() *domain.MergeRequest {
	if len(c.cards) == 0 || c.focusIdx >= len(c.cards) {
		return nil
	}
	mr := c.cards[c.focusIdx].mr
	return &mr
}

func (c columnWidget) Init() tea.Cmd                         { return nil }
func (c columnWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return c, nil }
func (c columnWidget) View() tea.View                        { return tea.NewView(c.render()) }

func (c columnWidget) render() string {
	// contentW is the column's inner content width (excluding border).
	contentW := c.width - colBorderWidth

	header := c.styles.ColumnHeader.Render(fmt.Sprintf("%s (%d)", c.name, len(c.cards)))
	// Pad header to contentW so the column border is always the right width.
	if w := lip.Width(header); w < contentW {
		header += strings.Repeat(" ", contentW-w)
	}

	var cardLines []string
	if len(c.cards) == 0 {
		empty := c.styles.EmptyColumn.Render("(empty)")
		if w := lip.Width(empty); w < contentW {
			empty += strings.Repeat(" ", contentW-w)
		}
		cardLines = append(cardLines, empty)
	} else {
		for i := range c.cards {
			cardLines = append(cardLines, c.cards[i].render())
		}
	}

	content := header + "\n" + strings.Join(cardLines, "\n")
	borderStyle := c.styles.ColumnBorder
	if c.focused {
		borderStyle = c.styles.ColumnBorderFocused
	}
	// No Width() — content is manually padded to contentW above so the border
	// adapts to the correct width without enabling lipgloss word-wrap.
	return borderStyle.Render(content)
}
