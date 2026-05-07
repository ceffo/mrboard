package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	lip "charm.land/lipgloss/v2"
	"github.com/mrboard/mrboard/internal/domain"
)

const (
	detailBorderWidth   = 2
	detailPadWidth      = 2
	detailMinInnerWidth = 20
	maxDescriptionLines = 40
	maxThreadBodyLines  = 10
	threadBodyIndent    = 2 // indent thread body text from the left edge
)

type detailWidget struct {
	mr             *domain.MergeRequest
	threads        []domain.Thread
	descExpanded   bool
	threadExpanded []bool // collapsed state per thread index
	scrollOffset   int
	styles         Styles
	width          int
	height         int
	loading        bool
}

func newDetailWidget(styles Styles) detailWidget {
	return detailWidget{styles: styles, descExpanded: true}
}

func (d *detailWidget) SetMR(mr *domain.MergeRequest) {
	d.mr = mr
	d.threads = nil
	d.threadExpanded = nil
	d.descExpanded = true
	d.scrollOffset = 0
	d.loading = true
}

func (d *detailWidget) SetThreads(threads []domain.Thread) {
	d.threads = threads
	d.threadExpanded = make([]bool, len(threads))
	d.loading = false
}

func (d *detailWidget) SetSize(width, height int) {
	d.width = width
	d.height = height
	d.clampScroll()
}

func (d *detailWidget) ScrollUp() {
	if d.scrollOffset > 0 {
		d.scrollOffset--
	}
}

func (d *detailWidget) ScrollDown() {
	total := d.totalContentLines()
	visible := d.visibleLines()
	if d.scrollOffset < total-visible {
		d.scrollOffset++
	}
}

func (d detailWidget) visibleLines() int {
	v := d.height - detailBorderWidth
	if v < 0 {
		return 0
	}
	return v
}

func (d detailWidget) totalContentLines() int {
	return strings.Count(d.buildContent(), "\n") + 1
}

func (d *detailWidget) clampScroll() {
	total := d.totalContentLines()
	visible := d.visibleLines()
	maxOffset := total - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if d.scrollOffset > maxOffset {
		d.scrollOffset = maxOffset
	}
}

func (d detailWidget) Init() tea.Cmd                         { return nil }
func (d detailWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return d, nil }
func (d detailWidget) View() tea.View                        { return tea.NewView(d.render()) }

func (d detailWidget) render() string {
	if d.mr == nil {
		return ""
	}

	full := d.buildContent()
	lines := strings.Split(full, "\n")
	total := len(lines)
	visible := d.visibleLines()

	// Apply scroll window.
	offset := d.scrollOffset
	if offset > total-visible {
		offset = total - visible
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + visible
	if end > total {
		end = total
	}
	window := strings.Join(lines[offset:end], "\n")

	// Scroll indicator line in the top-right corner of the border.
	hasAbove := offset > 0
	hasBelow := end < total
	ind := d.styles.ScrollIndicator.Render(scrollIndicator(hasAbove, hasBelow))

	innerWidth := d.width - detailBorderWidth - detailPadWidth
	if innerWidth < detailMinInnerWidth {
		innerWidth = detailMinInnerWidth
	}
	indW := lip.Width(ind)
	if indW < innerWidth {
		// Overlay indicator on the first visible line (right-aligned).
		firstLineEnd := strings.Index(window, "\n")
		if firstLineEnd < 0 {
			firstLineEnd = len(window)
		}
		first := window[:firstLineEnd]
		rest := window[firstLineEnd:]
		fw := lip.Width(first)
		pad := innerWidth - fw - indW
		if pad < 0 {
			pad = 0
		}
		window = first + strings.Repeat(" ", pad) + ind + rest
	}

	return d.styles.DetailPanel.Width(d.width - detailBorderWidth).Render(window)
}

// buildContent builds the full scrollable content string (no height limit applied).
func (d detailWidget) buildContent() string {
	innerWidth := d.width - detailBorderWidth - detailPadWidth
	if innerWidth < detailMinInnerWidth {
		innerWidth = detailMinInnerWidth
	}

	var sb strings.Builder

	// Title
	title := wordWrap(d.mr.Title, innerWidth)
	sb.WriteString(d.styles.DetailTitle.Width(innerWidth).Render(title))
	sb.WriteByte('\n')

	// Meta: author · phase · approvals
	phaseLbl := mrPhaseLabel(d.mr.Phase)
	approvals := fmt.Sprintf("%d/%d approvals", d.mr.ApprovalCount, d.mr.RequiredApprovals)
	meta := fmt.Sprintf("%s  ·  %s  ·  %s", d.mr.Author, phaseLbl, approvals)
	sb.WriteString(d.styles.DetailMeta.Render(meta))
	sb.WriteByte('\n')

	// Reviewers
	if len(d.mr.Reviewers) > 0 {
		sb.WriteString(d.styles.DetailMeta.Render(buildReviewerLine(d.mr.Reviewers)))
		sb.WriteByte('\n')
	}

	sb.WriteByte('\n')

	// Description section
	descToggle := "▼"
	if !d.descExpanded {
		descToggle = "▶"
	}
	sb.WriteString(d.styles.DetailSectionHeader.Render(descToggle + " Description"))
	sb.WriteByte('\n')
	if d.descExpanded {
		body := strings.TrimSpace(d.mr.Description)
		if body == "" {
			sb.WriteString(d.styles.DetailMeta.Render("(no description)"))
		} else {
			sb.WriteString(renderMarkdown(body, innerWidth))
		}
		sb.WriteByte('\n')
	}

	sb.WriteByte('\n')

	// Discussions
	if d.loading {
		sb.WriteString(d.styles.DetailMeta.Render("Loading comments…"))
	} else if len(d.threads) == 0 {
		sb.WriteString(d.styles.DetailMeta.Render("No comment threads"))
	} else {
		threadWord := "thread"
		if len(d.threads) > 1 {
			threadWord = "threads"
		}
		sb.WriteString(d.styles.DetailSectionHeader.Render(
			fmt.Sprintf("● %d %s", len(d.threads), threadWord)))
		sb.WriteByte('\n')
		for i, t := range d.threads {
			d.renderThread(&sb, i, t, innerWidth)
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (d detailWidget) renderThread(sb *strings.Builder, idx int, t domain.Thread, innerWidth int) {
	if len(t.Notes) == 0 {
		return
	}
	first := t.Notes[0]
	toggle := "▶"
	expanded := idx < len(d.threadExpanded) && d.threadExpanded[idx]
	if expanded {
		toggle = "▼"
	}

	status := ""
	if t.Resolved {
		status = " ✓"
	}
	header := fmt.Sprintf("%s %s%s", toggle, first.Author, status)
	sb.WriteString(d.styles.DetailSectionHeader.Render(header))
	sb.WriteByte('\n')

	if expanded {
		for _, n := range t.Notes {
			if n.System {
				continue
			}
			noteHeader := d.styles.DetailMeta.Render(n.Author + ":")
			body := wordWrap(strings.TrimSpace(n.Body), innerWidth-threadBodyIndent)
			body = truncateLines(body, maxThreadBodyLines)
			sb.WriteString(noteHeader + "\n")
			sb.WriteString(d.styles.DetailBody.Render(body))
			sb.WriteByte('\n')
		}
	}
}

func mrPhaseLabel(p domain.MRPhase) string {
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

func buildReviewerLine(reviewers []domain.ReviewerInfo) string {
	parts := make([]string, 0, len(reviewers))
	for _, r := range reviewers {
		parts = append(parts, detailReviewerIcon(r.State)+" "+r.Name)
	}
	return strings.Join(parts, "  ")
}

func detailReviewerIcon(s domain.ReviewerState) string {
	switch s {
	case domain.ReviewerApproved:
		return "✓"
	case domain.ReviewerCommented:
		return "💬"
	case domain.ReviewerReReviewRequested:
		return "↩"
	default:
		return "○"
	}
}

// wordWrap wraps text at width characters, preserving existing newlines.
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	var out strings.Builder
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			out.WriteByte('\n')
			continue
		}
		line := ""
		for _, w := range words {
			if line == "" {
				line = w
			} else if len(line)+1+len(w) <= width {
				line += " " + w
			} else {
				out.WriteString(line)
				out.WriteByte('\n')
				line = w
			}
		}
		if line != "" {
			out.WriteString(line)
		}
		out.WriteByte('\n')
	}
	return strings.TrimRight(out.String(), "\n")
}

// truncateLines limits rendered output to maxLines lines.
func truncateLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return s
	}
	lines := strings.SplitN(s, "\n", maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

// joinHorizontalTop joins two strings side-by-side aligned at the top.
func joinHorizontalTop(left, right string) string {
	return lip.JoinHorizontal(lip.Top, left, right)
}

// renderMarkdown renders a markdown string to ANSI terminal output via glamour.
// WithEnvironmentConfig() falls back to "notty" when stdout is not a raw TTY,
// which is always the case inside Bubble Tea's alt-screen. We check GLAMOUR_STYLE
// ourselves and default to "dark" so inline code is always styled correctly.
// Falls back to plain word-wrapped text on error.
func renderMarkdown(md string, width int) string {
	// GitLab sometimes stores descriptions with backslash-escaped backticks (\`)
	// which are literal backticks in CommonMark, not code spans. Unescape them
	// so inline code renders correctly.
	md = strings.ReplaceAll(md, "\\`", "`")

	style := os.Getenv("GLAMOUR_STYLE")
	if style == "" {
		style = "dark"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return wordWrap(md, width)
	}
	out, err := r.Render(md)
	if err != nil {
		return wordWrap(md, width)
	}
	return strings.TrimRight(out, "\n")
}
