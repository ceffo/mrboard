package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mrboard/mrboard/internal/domain"
)

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

	title := truncate(c.mr.Title, innerWidth)

	now := time.Now()
	metaParts := []string{"@" + c.mr.Author}

	base := c.mr.NonDraftSince
	if base.IsZero() {
		base = c.mr.CreatedAt
	}
	if !base.IsZero() {
		metaParts = append(metaParts, "open:"+domain.FormatDuration(now.Sub(base)))
	}
	if !c.mr.WaitingSince.IsZero() {
		metaParts = append(metaParts, "phase:"+domain.FormatDuration(now.Sub(c.mr.WaitingSince)))
	}
	if c.mr.RoundTripCount > 0 {
		metaParts = append(metaParts, fmt.Sprintf("rt:%d", c.mr.RoundTripCount))
	}

	lines := []string{
		c.styles.CardTitle.Render(title),
		c.styles.CardMeta.Render(strings.Join(metaParts, "  ")),
	}
	if pills := c.renderPills(now); pills != "" {
		lines = append(lines, pills)
	}

	style := c.styles.Card
	if c.focused {
		style = c.styles.CardFocused
	}
	return style.Width(c.width - 2).Render(strings.Join(lines, "\n"))
}

func (c cardWidget) renderPills(now time.Time) string {
	if len(c.mr.Reviewers) == 0 {
		return ""
	}
	parts := make([]string, 0, len(c.mr.Reviewers))
	for _, r := range c.mr.Reviewers {
		icon := reviewerIcon(r.State)
		pill := "[" + r.Username + "·" + icon
		if !r.WaitingSince.IsZero() {
			pill += " " + domain.FormatDuration(now.Sub(r.WaitingSince))
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
	return strings.Join(parts, " ")
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

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
