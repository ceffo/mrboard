package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

const (
	detailBorderWidth   = 2
	detailPadWidth      = 2
	detailMinInnerWidth = 20
	maxThreadBodyLines  = 10
	threadBodyIndent    = 2
	detailTitleLines    = 5
)

// detailWidget renders a single MR's description and discussion threads in a
// side panel. Layout:
//
//	╭─ [title truncated]         ↑↓ ─╮   ← fixed header line (scroll indicator here)
//	│  [scrollable body lines]       │   ← scrollViewport manages this area
//	╰────────────────────────────────╯
//
// The panel style has no Width() so lipgloss never word-wraps the content.
// Each visual line is built individually and the header always fills
// innerWidth, keeping the panel at its assigned width.
type detailWidget struct {
	mr             *domain.MergeRequest
	threads        []domain.Thread
	descExpanded   bool
	threadExpanded []bool
	vp             scrollViewport
	styles         Styles
	width          int
	height         int
	loading        bool
}

func newDetailWidget(styles Styles) detailWidget {
	return detailWidget{styles: styles, descExpanded: true}
}

// SetStyles updates the detail panel's style set.
func (d *detailWidget) SetStyles(s Styles) { d.styles = s }

func (d *detailWidget) SetMR(mr *domain.MergeRequest) {
	d.mr = mr
	d.threads = nil
	d.threadExpanded = nil
	d.descExpanded = true
	d.vp.reset()
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
}

func (d *detailWidget) ScrollUp() { d.vp.scrollUp() }
func (d *detailWidget) ScrollDown() {
	innerWidth := d.width - detailBorderWidth - detailPadWidth
	if innerWidth < detailMinInnerWidth {
		innerWidth = detailMinInnerWidth
	}
	d.vp.scrollDown(len(d.buildLines(innerWidth)), d.bodyLines())
}

// bodyLines returns the number of lines available for the scrollable body
// (total height minus border rows and the fixed header line).
func (d detailWidget) bodyLines() int {
	v := d.height - detailBorderWidth - 1
	if v < 0 {
		return 0
	}
	return v
}

func (d detailWidget) Init() tea.Cmd                         { return nil }
func (d detailWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return d, nil }
func (d detailWidget) View() tea.View                        { return tea.NewView(d.render()) }

func (d detailWidget) render() string {
	if d.mr == nil {
		return ""
	}

	innerWidth := d.width - detailBorderWidth - detailPadWidth
	if innerWidth < detailMinInnerWidth {
		innerWidth = detailMinInnerWidth
	}

	lines := d.buildLines(innerWidth)
	visible := d.bodyLines()
	total := len(lines)

	header := d.renderHeader(innerWidth, d.vp.hasAbove(), d.vp.hasBelow(total, visible))
	window := d.vp.window(lines, visible)
	content := header + "\n" + strings.Join(window, "\n")

	// No Width() on the panel — word-wrap is disabled. The header line is
	// always exactly innerWidth wide, which anchors the panel to d.width.
	// Height() fills the panel to the full assigned height (same as columns).
	panelStyle := d.styles.DetailPanel
	if d.height > 0 {
		panelStyle = panelStyle.Height(d.height)
	}
	return panelStyle.Render(content)
}

// renderHeader renders a thin fixed line that anchors the scroll indicator.
// All MR content lives in the scrollable body below.
func (d detailWidget) renderHeader(innerWidth int, hasAbove, hasBelow bool) string {
	ind := d.styles.ScrollIndicator.Render(scrollIndicator(hasAbove, hasBelow))
	indW := lip.Width(ind)
	pad := innerWidth - indW
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + ind
}

// buildLines produces the full scrollable content as a flat []string of
// individual visual lines. Each element is one rendered line (may contain
// ANSI codes, must not contain embedded newlines).
func (d detailWidget) buildLines(innerWidth int) []string {
	var out []string

	add := func(line string) { out = append(out, line) }

	// Flatten a potentially multi-line rendered block into individual lines.
	addBlock := func(rendered string) {
		out = append(out, strings.Split(rendered, "\n")...)
	}

	// author · phase
	phaseLbl := mrPhaseLabel(d.mr.Phase)
	add(d.styles.CardAuthor.Render(d.mr.DisplayAuthor()) +
		d.styles.DetailMeta.Render("  ·  "+phaseLbl))

	// !IID  full project path
	mrRef := d.styles.MRNumberBang.Render("!") +
		d.styles.DetailMeta.Render(fmt.Sprintf("%d", d.mr.IID))
	if d.mr.ProjectPath != "" {
		mrRef += d.styles.DetailMeta.Render("  " + d.mr.ProjectPath)
	}
	add(mrRef)

	add("") // blank

	// full title, wrapped
	for _, tl := range wrapLines(d.mr.Title, innerWidth, detailTitleLines) {
		add(d.styles.DetailTitle.Render(tl))
	}

	add("") // blank

	// reviewers / approvers — one per line, name + icon, approvers first
	for _, r := range d.detailReviewerLines() {
		add(r)
	}
	if d.mr.Phase == domain.PhaseReadyToMerge &&
		d.mr.DetailedMergeStatus != detailedMergeStatusMergeable &&
		d.mr.DetailedMergeStatus != "" {
		add(d.styles.ErrorMsg.Render("⚠ " + mergeBlockReason(d.mr.DetailedMergeStatus)))
	}

	add("") // blank separator

	descToggle := "▼"
	if !d.descExpanded {
		descToggle = "▶"
	}
	add(d.styles.DetailSectionHeader.Render(descToggle + " Description"))
	if d.descExpanded {
		body := strings.TrimSpace(d.mr.Description)
		if body == "" {
			add(d.styles.DetailMeta.Render("(no description)"))
		} else {
			addBlock(renderMarkdown(body, innerWidth))
		}
	}

	add("") // blank separator

	if d.loading {
		add(d.styles.DetailMeta.Render("Loading comments…"))
	} else if len(d.threads) == 0 {
		add(d.styles.DetailMeta.Render("No comment threads"))
	} else {
		threadWord := "thread"
		if len(d.threads) > 1 {
			threadWord = "threads"
		}
		add(d.styles.DetailSectionHeader.Render(
			fmt.Sprintf("● %d %s", len(d.threads), threadWord)))
		for i, t := range d.threads {
			for _, l := range d.threadLines(i, t, innerWidth) {
				add(l)
			}
		}
	}

	return out
}

// threadLines renders a single discussion thread as a flat []string.
func (d detailWidget) threadLines(idx int, t domain.Thread, innerWidth int) []string {
	if len(t.Notes) == 0 {
		return nil
	}
	var out []string
	first := t.Notes[0]
	expanded := idx < len(d.threadExpanded) && d.threadExpanded[idx]
	toggle := "▶"
	if expanded {
		toggle = "▼"
	}
	status := ""
	if t.Resolved {
		status = " ✓"
	}
	out = append(out, d.styles.DetailSectionHeader.Render(
		fmt.Sprintf("%s %s%s", toggle, first.Author, status)))

	if expanded {
		for _, n := range t.Notes {
			if n.System {
				continue
			}
			out = append(out, d.styles.DetailMeta.Render(n.Author+":"))
			body := wordWrap(strings.TrimSpace(n.Body), innerWidth-threadBodyIndent)
			body = truncateLines(body, maxThreadBodyLines)
			for _, l := range strings.Split(body, "\n") {
				out = append(out, d.styles.DetailBody.Render(
					strings.Repeat(" ", threadBodyIndent)+l))
			}
		}
	}
	return out
}

// ── helpers ──────────────────────────────────────────────────────────────────

func mrPhaseLabel(p domain.MRPhase) string {
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

// detailReviewerLines renders one line per reviewer/approver: approvers first,
// then plain reviewers. Each line is "Full Name <icon>" using the same name and
// pill colour styles as card pills.
func (d detailWidget) detailReviewerLines() []string {
	if len(d.mr.Reviewers) == 0 {
		return nil
	}
	sorted := make([]domain.ReviewerInfo, 0, len(d.mr.Reviewers))
	for _, r := range d.mr.Reviewers {
		if r.IsApprover {
			sorted = append(sorted, r)
		}
	}
	for _, r := range d.mr.Reviewers {
		if !r.IsApprover {
			sorted = append(sorted, r)
		}
	}
	lines := make([]string, 0, len(sorted))
	for _, r := range sorted {
		nameStyle := d.styles.ReviewerName
		if r.IsApprover {
			nameStyle = d.styles.ApproverName
		}
		name := r.Name
		if name == "" {
			name = r.Username
		}
		icon := reviewerIcon(r.State)
		iconStyled := pillStyle(r.State, d.styles).Render(icon)
		lines = append(lines, nameStyle.Render(name)+" "+iconStyled)
	}
	return lines
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

// truncateLines limits output to maxLines lines.
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

// mergeBlockReason maps a GitLab detailed_merge_status value to a human-readable string.
func mergeBlockReason(status string) string {
	switch status {
	case "ci_must_pass":
		return "CI pipeline must pass"
	case "discussions_not_resolved":
		return "Unresolved discussions"
	case "draft_status":
		return "MR is a draft"
	case "not_approved":
		return "Awaiting approval"
	case "blocked_status":
		return "Blocked by another MR"
	case "jira_association_missing":
		return "Jira issue required"
	case "merge_request_blocked":
		return "Merge blocked"
	case "need_rebase":
		return "Needs rebase"
	default:
		return status
	}
}

// renderMarkdown renders markdown to ANSI terminal output via glamour.
// Falls back to plain word-wrapped text on error.
func renderMarkdown(md string, width int) string {
	md = strings.ReplaceAll(md, "\\`", "`")

	style := os.Getenv("GLAMOUR_STYLE")
	if style == "" {
		style = themeModeDark
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
