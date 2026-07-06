package tui

import (
	"strings"

	lip "charm.land/lipgloss/v2"
)

// helpModalWidget renders the '?' help card: every binding reachable in the
// current context stack, grouped by category into balanced columns. It is
// purely presentational — open/close state and key routing live in the root
// model (via HelpCtx on top of the context stack).
type helpModalWidget struct {
	styles Styles
	width  int // terminal width, used to cap the card size
	height int
}

func newHelpModalWidget(styles Styles) helpModalWidget {
	return helpModalWidget{styles: styles}
}

// SetStyles updates the modal's style set.
func (h *helpModalWidget) SetStyles(s Styles) { h.styles = s }

// SetSize records the terminal size used to cap the card dimensions.
func (h *helpModalWidget) SetSize(w, ht int) { h.width, h.height = w, ht }

const helpColumnGap = 4

// render builds the centered help card for the given stack (bottom → top,
// excluding HelpCtx itself). The title names the top context.
func (h helpModalWidget) render(stack []*Context) string {
	top := stack[len(stack)-1]
	sections := helpSections(stack)

	blocks := make([]string, len(sections))
	for i, s := range sections {
		blocks[i] = h.renderSection(s)
	}
	columns := h.packColumns(blocks)
	body := lip.JoinHorizontal(lip.Top, columns...)

	title := h.styles.HelpTitle.Render("Help · " + top.Title())
	hint := h.styles.PopupHint.Render(closeHint())
	inner := title + "\n\n" + body + "\n\n" + hint
	return h.styles.PopupBorder.Render(inner)
}

// closeHint renders the modal's own dismiss keys, sourced from HelpCtx so the
// hint can never drift from the actual bindings.
func closeHint() string {
	c := DefaultHelpKeyMap.Close
	return c.Help().Key + " " + c.Help().Desc
}

// renderSection renders one category block: title plus aligned key/label rows.
func (h helpModalWidget) renderSection(s helpSection) string {
	keyW := 0
	for _, e := range s.entries {
		if w := lip.Width(e.key); w > keyW {
			keyW = w
		}
	}
	var b strings.Builder
	b.WriteString(h.styles.HelpSection.Render(s.title))
	for _, e := range s.entries {
		pad := strings.Repeat(" ", keyW-lip.Width(e.key))
		b.WriteString("\n" + h.styles.HelpKey.Render(e.key) + pad + "  " + h.styles.HelpLabel.Render(e.label))
	}
	return b.String()
}

// packColumns distributes section blocks into up to two columns, greedily
// assigning each block to the currently shorter column while preserving
// section order within a column.
func (h helpModalWidget) packColumns(blocks []string) []string {
	if len(blocks) <= 1 {
		return blocks
	}
	var left, right []string
	leftH, rightH := 0, 0
	for _, b := range blocks {
		bh := lip.Height(b) + 1 // +1 for the blank line between sections
		if leftH <= rightH {
			left = append(left, b)
			leftH += bh
		} else {
			right = append(right, b)
			rightH += bh
		}
	}
	gap := strings.Repeat(" ", helpColumnGap)
	cols := []string{strings.Join(left, "\n\n")}
	if len(right) > 0 {
		cols = append(cols, gap, strings.Join(right, "\n\n"))
	}
	return cols
}
