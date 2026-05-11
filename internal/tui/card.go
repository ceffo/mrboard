package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"
	"github.com/mrboard/mrboard/internal/domain"
)

// nbsp is U+00A0 NO-BREAK SPACE. Using an integer constant keeps the source
// free of non-ASCII bytes so no editor or tool can normalize it.
const nbsp rune = 0x00A0

const (
	cardBorderAndPad = 4 // 1 border + 1 padding on each side
	minInnerWidth    = 8
	minLeftW         = 2
	minEllipsisWidth = 3
)

type cardWidget struct {
	mr            domain.MergeRequest
	styles        Styles
	focused       bool
	focusInactive bool // focused but board does not own the keyboard (detail panel open)
	width         int
}

func newCardWidget(mr domain.MergeRequest, styles Styles, width int) cardWidget {
	return cardWidget{mr: mr, styles: styles, width: width}
}

func (c *cardWidget) SetFocused(v bool)       { c.focused = v }
func (c *cardWidget) SetFocusInactive(v bool) { c.focusInactive = v }
func (c *cardWidget) SetWidth(w int)          { c.width = w }

func (c cardWidget) Init() tea.Cmd                         { return nil }
func (c cardWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return c, nil }
func (c cardWidget) View() tea.View                        { return tea.NewView(c.render()) }

func (c cardWidget) render() string {
	innerWidth := c.width - cardBorderAndPad
	if innerWidth < minInnerWidth {
		innerWidth = minInnerWidth
	}

	now := time.Now()

	// Line 1: author (+ waiting if NeedsAuthorAction) left · open duration right.
	// Duration spaces are replaced with NBSP so lipgloss won't word-wrap within them.
	authorLabel := c.mr.Author
	if c.mr.Phase == domain.PhaseNeedsAuthorAction && !c.mr.WaitingSince.IsZero() {
		waitDur := now.Sub(c.mr.WaitingSince)
		authorLabel += " ⏳ " + withNBSP(domain.FormatDuration(waitDur))
	}

	var openLabel string
	base := c.mr.NonDraftSince
	if base.IsZero() {
		base = c.mr.CreatedAt
	}
	if !base.IsZero() {
		openLabel = withNBSP(domain.FormatDuration(now.Sub(base)))
	}

	rawLines := []string{
		c.renderLine1(authorLabel, openLabel, innerWidth),
		c.styles.CardTitle.Render(truncateWidth(c.mr.Title, innerWidth)),
		"",
	}

	rawLines = append(rawLines, c.wrapPills(now, innerWidth)...)

	// Each line is clamped to exactly innerWidth display cols: truncate if over,
	// pad with spaces if under. This keeps the card a fixed width without using
	// Width() on the card style (which triggers lipgloss word-wrap).
	padded := make([]string, len(rawLines))
	for i, l := range rawLines {
		w := lip.Width(l)
		if w > innerWidth {
			l = lip.NewStyle().MaxWidth(innerWidth).Render(l)
			w = lip.Width(l)
		}
		if w < innerWidth {
			l += strings.Repeat(" ", innerWidth-w)
		}
		padded[i] = l
	}

	style := c.styles.Card
	switch {
	case c.focused && c.focusInactive:
		style = c.styles.CardFocusedInactive
	case c.focused:
		style = c.styles.CardFocused
	}
	// No Width() — manual per-line padding above keeps the card at a consistent
	// width. Using Width() here enables lipgloss word-wrap which breaks the layout.
	return style.Render(strings.Join(padded, "\n"))
}

// renderLine1 builds the first card line: !IID + author left, duration right.
// The MR ref prefix has its own style so it can't go through renderHeaderLine.
func (c cardWidget) renderLine1(authorLabel, openLabel string, width int) string {
	mrRef := c.styles.MRNumberBang.Render("!") +
		c.styles.CardMeta.Render(fmt.Sprintf("%d ", c.mr.IID))
	mrRefW := lip.Width(mrRef)

	rightRendered := ""
	rightW := 0
	if openLabel != "" {
		rightRendered = c.styles.CardMeta.Render(openLabel)
		rightW = lip.Width(rightRendered)
	}

	availAuthorW := width - mrRefW - rightW
	if availAuthorW < 0 {
		availAuthorW = 0
	}
	authorTrunc := truncateWidth(authorLabel, availAuthorW)
	authorStyled := c.styles.CardAuthor.Render(authorTrunc)
	authorW := lip.Width(authorStyled)

	pad := width - mrRefW - authorW - rightW
	if pad < 0 {
		pad = 0
	}
	return mrRef + authorStyled + strings.Repeat(" ", pad) + rightRendered
}

// renderPills returns each reviewer pill as a separately styled string.
func (c cardWidget) renderPills(now time.Time) []string {
	if len(c.mr.Reviewers) == 0 {
		return nil
	}
	parts := make([]string, 0, len(c.mr.Reviewers))
	for _, r := range c.mr.Reviewers {
		icon := reviewerIcon(r.State)
		pill := "[" + r.Username + "·" + icon
		if !r.WaitingSince.IsZero() {
			pill += " " + withNBSP(domain.FormatDuration(now.Sub(r.WaitingSince)))
		}
		pill += "]"

		var rendered string
		switch r.State {
		case domain.ReviewerNotStarted:
			rendered = c.styles.PillNotStarted.Render(pill)
		case domain.ReviewerCommented:
			rendered = c.styles.PillCommented.Render(pill)
		case domain.ReviewerReReviewRequested:
			rendered = c.styles.PillReReview.Render(pill)
		case domain.ReviewerApproved:
			rendered = c.styles.PillApproved.Render(pill)
		default:
			rendered = c.styles.CardMeta.Render(pill)
		}
		parts = append(parts, rendered)
	}
	return parts
}

// wrapPills lays out reviewer pills into lines that each fit within width columns.
func (c cardWidget) wrapPills(now time.Time, width int) []string {
	pills := c.renderPills(now)
	if len(pills) == 0 {
		return nil
	}
	var lines []string
	line := ""
	lineW := 0
	for i, p := range pills {
		pW := lip.Width(p)
		if lineW == 0 {
			line = p
			lineW = pW
		} else if lineW+1+pW <= width {
			line += " " + p
			lineW += 1 + pW
		} else {
			lines = append(lines, line)
			line = p
			lineW = pW
		}
		if i == len(pills)-1 {
			lines = append(lines, line)
		}
	}
	return lines
}

func reviewerIcon(s domain.ReviewerState) string {
	switch s {
	case domain.ReviewerNotStarted:
		return "⏳"
	case domain.ReviewerCommented:
		return "💬"
	case domain.ReviewerReReviewRequested:
		return "🔄"
	case domain.ReviewerApproved:
		return "✓"
	default:
		return "?"
	}
}

// withNBSP replaces every ASCII space in s with U+00A0 NO-BREAK SPACE so that
// lipgloss does not word-wrap within formatted duration strings like "3y 10mo".
func withNBSP(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' {
			return nbsp
		}
		return r
	}, s)
}

// truncateWidth truncates s to fit within maxWidth display columns using
// lip.Width for correct wide-character (emoji, CJK) measurement.
func truncateWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lip.Width(s) <= maxWidth {
		return s
	}
	if maxWidth <= minEllipsisWidth {
		result := ""
		w := 0
		for _, r := range s {
			rw := lip.Width(string(r))
			if w+rw > maxWidth {
				break
			}
			result += string(r)
			w += rw
		}
		return result
	}
	runes := []rune(s)
	for i := len(runes) - 1; i >= 0; i-- {
		candidate := string(runes[:i]) + "..."
		if lip.Width(candidate) <= maxWidth {
			return candidate
		}
	}
	return "..."
}
