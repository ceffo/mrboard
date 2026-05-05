package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"
	"github.com/mrboard/mrboard/internal/domain"
)

// nbsp is U+00A0 NO-BREAK SPACE. Using an integer constant keeps the source
// free of non-ASCII bytes so no editor or tool can normalize it.
const nbsp rune = 0x00A0

type cardWidget struct {
	mr      domain.MergeRequest
	styles  Styles
	focused bool
	width   int
}

func newCardWidget(mr domain.MergeRequest, styles Styles, width int) cardWidget {
	return cardWidget{mr: mr, styles: styles, width: width}
}

func (c *cardWidget) SetFocused(v bool) { c.focused = v }
func (c *cardWidget) SetWidth(w int)    { c.width = w }

func (c cardWidget) Init() tea.Cmd                           { return nil }
func (c cardWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return c, nil }
func (c cardWidget) View() tea.View                          { return tea.NewView(c.render()) }

func (c cardWidget) render() string {
	innerWidth := c.width - 4
	if innerWidth < 8 {
		innerWidth = 8
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
		c.renderHeaderLine(authorLabel, openLabel, innerWidth),
		"",
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
	if c.focused {
		style = c.styles.CardFocused
	}
	// No Width() — manual per-line padding above keeps the card at a consistent
	// width. Using Width() here enables lipgloss word-wrap which breaks the layout.
	return style.Render(strings.Join(padded, "\n"))
}

// renderHeaderLine places left content left-aligned and right content right-aligned
// within width display columns. Padding is added as plain spaces directly to the
// raw string before styling so no outer Width() triggers lipgloss word-wrap.
func (c cardWidget) renderHeaderLine(left, right string, width int) string {
	if right == "" {
		return c.styles.CardAuthor.Render(left)
	}
	rightRendered := c.styles.CardMeta.Render(right)
	rightW := lip.Width(rightRendered)
	leftW := width - rightW
	if leftW < 2 {
		leftW = 2
	}
	truncated := truncateWidth(left, leftW)
	if pad := leftW - lip.Width(truncated); pad > 0 {
		truncated += strings.Repeat(" ", pad)
	}
	return c.styles.CardAuthor.Render(truncated) + rightRendered
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
	if maxWidth <= 3 {
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
