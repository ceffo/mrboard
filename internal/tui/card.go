package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// nbsp is U+00A0 NO-BREAK SPACE. Using an integer constant keeps the source
// free of non-ASCII bytes so no editor or tool can normalize it.
const nbsp rune = 0x00A0

const (
	cardBorderAndPad = 4 // 1 border + 1 padding on each side
	minInnerWidth    = 8
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
	innerWidth := max(c.width-cardBorderAndPad, minInnerWidth)

	now := time.Now()

	// Line 1: author (+ waiting if NeedsAuthorAction) left · open duration right.
	// Duration spaces are replaced with NBSP so lipgloss won't word-wrap within them.
	authorLabel := c.mr.DisplayAuthor()
	if c.mr.Phase == domain.PhaseNeedsAuthorAction && !c.mr.WaitingSince.IsZero() {
		waitDur := now.Sub(c.mr.WaitingSince)
		authorLabel += " ⏳ " + withNBSP(domain.FormatDuration(waitDur))
	}

	var openDur time.Duration
	base := c.mr.NonDraftSince
	if base.IsZero() {
		base = c.mr.CreatedAt
	}
	if !base.IsZero() {
		openDur = now.Sub(base)
	}

	rawLines := []string{c.renderLine1(authorLabel, openDur, innerWidth)}
	for _, tl := range wrapTitleLines(c.mr.Title, innerWidth) {
		rawLines = append(rawLines, c.styles.CardTitle.Render(tl))
	}
	rawLines = append(rawLines, "")

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
func (c cardWidget) renderLine1(authorLabel string, openDur time.Duration, width int) string {
	mrRef := c.styles.MRNumberBang.Render("!") +
		c.styles.CardMeta.Render(fmt.Sprintf("%d ", c.mr.IID))
	mrRefW := lip.Width(mrRef)

	rightRendered := ""
	rightW := 0
	if openDur > 0 {
		openLabel := withNBSP(domain.FormatDuration(openDur))
		rightRendered = c.durationStyle(openDur).Render(openLabel)
		rightW = lip.Width(rightRendered)
	}

	availAuthorW := max(width-mrRefW-rightW, 0)
	authorTrunc := truncateWidth(authorLabel, availAuthorW)
	authorStyled := c.styles.CardAuthor.Render(authorTrunc)
	authorW := lip.Width(authorStyled)

	pad := width - mrRefW - authorW - rightW
	if pad < 0 {
		pad = 0
	}
	return mrRef + authorStyled + strings.Repeat(" ", pad) + rightRendered
}

// durationStyle picks the appropriate style based on how old the MR is.
func (c cardWidget) durationStyle(dur time.Duration) lip.Style {
	switch {
	case c.styles.LifetimeError > 0 && dur >= c.styles.LifetimeError:
		return c.styles.DurationUrgent
	case c.styles.LifetimeWarn > 0 && dur >= c.styles.LifetimeWarn:
		return c.styles.DurationWarning
	default:
		return c.styles.DurationOk
	}
}

// renderPills returns each reviewer pill as a separately styled string.
func (c cardWidget) renderPills(now time.Time) []string {
	if len(c.mr.Reviewers) == 0 {
		return nil
	}
	parts := make([]string, 0, len(c.mr.Reviewers))
	for _, r := range c.mr.Reviewers {
		if r.State == domain.ReviewerNotStarted {
			continue
		}
		rendered := c.renderPill(r, now)
		parts = append(parts, rendered)
	}
	return parts
}

func (c cardWidget) renderPill(r domain.ReviewerInfo, now time.Time) string {
	icon := reviewerIcon(r.State)
	displayName := c.renderUsername(r.Username)
	pillStyle := pillStyle(r.State, c.styles)
	var rendered strings.Builder
	rendered.WriteString(pillStyle.Render("["))
	rendered.WriteString(displayName)
	rendered.WriteString(pillStyle.Render("."))
	rendered.WriteString(pillStyle.Render(icon))
	if !r.WaitingSince.IsZero() {
		duration := withNBSP(domain.FormatDuration(now.Sub(r.WaitingSince)))
		rendered.WriteString(" ")
		rendered.WriteString(pillStyle.Render(duration))
	}
	rendered.WriteString(pillStyle.Render("]"))
	return rendered.String()
}

func pillStyle(state domain.ReviewerState, styles Styles) lip.Style {
	switch state {
	case domain.ReviewerNotStarted:
		return styles.PillNotStarted
	case domain.ReviewerCommented:
		return styles.PillCommented
	case domain.ReviewerReReviewRequested:
		return styles.PillReReview
	case domain.ReviewerApproved:
		return styles.PillApproved
	default:
		return styles.CardMeta
	}
}

func (c cardWidget) renderUsername(username string) string {
	if username == "" {
		return c.styles.ErrorMsg.Render("<unknown>")
	}
	return c.styles.UsernameAtSign.Render("@") + c.styles.CardAuthor.Render(username)
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

// wrapTitleLines wraps title into at most two lines within width display columns.
// Line 1 breaks at the last word boundary that fits; line 2 is hard-truncated with "…".
func wrapTitleLines(title string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	if lip.Width(title) <= width {
		return []string{title}
	}
	words := strings.Fields(title)
	if len(words) == 0 {
		return []string{truncateWidth(title, width)}
	}
	line1 := ""
	splitAt := 0
	for i, w := range words {
		candidate := line1
		if candidate != "" {
			candidate += " "
		}
		candidate += w
		if lip.Width(candidate) <= width {
			line1 = candidate
			splitAt = i + 1
		} else {
			break
		}
	}
	if splitAt == 0 {
		// Even the first word is wider than the column — hard-truncate to one line.
		return []string{truncateWidth(title, width)}
	}
	if splitAt >= len(words) {
		return []string{line1}
	}
	remaining := strings.Join(words[splitAt:], " ")
	return []string{line1, truncateWidth(remaining, width)}
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
		var result strings.Builder
		w := 0
		for _, r := range s {
			rw := lip.Width(string(r))
			if w+rw > maxWidth {
				break
			}
			result.WriteString(string(r))
			w += rw
		}
		return result.String()
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
