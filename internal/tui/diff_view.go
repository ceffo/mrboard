package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

const (
	fileListWidth   = 28
	fileListSepChar = "│"
	halfPageDivisor = 2
)

// diffViewWidget renders a full-screen diff view for a single MR.
// Layout: file list (left) | separator | diff content (right, scrollable).
type diffViewWidget struct {
	mr      *domain.MergeRequest
	files   []domain.FileDiff
	fileIdx int
	vp      scrollViewport
	styles  Styles
	width   int
	height  int
	loading bool
}

func newDiffViewWidget(styles Styles) diffViewWidget {
	return diffViewWidget{styles: styles}
}

func (d *diffViewWidget) SetStyles(s Styles) { d.styles = s }

func (d *diffViewWidget) SetMR(mr *domain.MergeRequest) {
	d.mr = mr
	d.files = nil
	d.fileIdx = 0
	d.vp.reset()
	d.loading = true
}

func (d *diffViewWidget) SetDiff(files []domain.FileDiff) {
	d.files = files
	d.fileIdx = 0
	d.vp.reset()
	d.loading = false
}

func (d *diffViewWidget) SetSize(width, height int) {
	d.width = width
	d.height = height
}

func (d *diffViewWidget) PrevFile() {
	if d.fileIdx > 0 {
		d.fileIdx--
		d.vp.reset()
	}
}

func (d *diffViewWidget) NextFile() {
	if d.fileIdx < len(d.files)-1 {
		d.fileIdx++
		d.vp.reset()
	}
}

func (d *diffViewWidget) diffPaneWidth() int {
	w := d.width - fileListWidth - 1
	if w < 1 {
		return 1
	}
	return w
}

func (d *diffViewWidget) diffBodyLines() int { return d.height }

func (d *diffViewWidget) currentLines() []string {
	if len(d.files) == 0 || d.fileIdx >= len(d.files) {
		return nil
	}
	return d.colorizedLines(d.files[d.fileIdx].Diff, d.diffPaneWidth())
}

func (d *diffViewWidget) ScrollUp()   { d.vp.scrollUp() }
func (d *diffViewWidget) ScrollDown() { d.vp.scrollDown(len(d.currentLines()), d.diffBodyLines()) }
func (d *diffViewWidget) HalfPageUp() { d.vp.scrollHalfPageUp(d.diffBodyLines() / halfPageDivisor) }
func (d *diffViewWidget) HalfPageDown() {
	d.vp.scrollHalfPageDown(len(d.currentLines()), d.diffBodyLines(), d.diffBodyLines()/halfPageDivisor)
}
func (d *diffViewWidget) ScrollToTop() { d.vp.scrollToTop() }
func (d *diffViewWidget) ScrollToBottom() {
	d.vp.scrollToBottom(len(d.currentLines()), d.diffBodyLines())
}

func (d diffViewWidget) Init() tea.Cmd                         { return nil }
func (d diffViewWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return d, nil }
func (d diffViewWidget) View() tea.View                        { return tea.NewView(d.render()) }

func (d diffViewWidget) render() string {
	if d.mr == nil || d.height <= 0 || d.width <= 0 {
		return ""
	}

	filePane := d.renderFileList(d.height)
	sep := d.renderSeparator(d.height)
	diffPane := d.renderDiffContent(d.diffPaneWidth(), d.height)

	return lip.JoinHorizontal(lip.Top, filePane, sep, diffPane)
}

func (d diffViewWidget) renderFileList(height int) string {
	lines := make([]string, 0, height)

	// Sliding window: keep selected file visible
	total := len(d.files)
	start := d.fileIdx - height/halfPageDivisor
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > total {
		end = total
		start = end - height
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		f := d.files[i]
		marker := diffFileMarker(f)
		name := filepath.Base(f.NewPath)
		if f.DeletedFile {
			name = filepath.Base(f.OldPath)
		}
		maxNameW := fileListWidth - len(marker) - 1
		if maxNameW < 1 {
			maxNameW = 1
		}
		name = truncateWidth(name, maxNameW)
		line := fmt.Sprintf("%s %s", marker, name)
		// Pad to full width so background fills
		padded := lip.NewStyle().Width(fileListWidth).Render(line)
		if i == d.fileIdx {
			lines = append(lines, d.styles.DiffFileSelected.Render(padded))
		} else {
			lines = append(lines, d.styles.DiffFileItem.Render(padded))
		}
	}

	// Pad with blank lines to fill height
	blank := lip.NewStyle().Width(fileListWidth).Render("")
	for len(lines) < height {
		lines = append(lines, blank)
	}

	return strings.Join(lines, "\n")
}

func (d diffViewWidget) renderSeparator(height int) string {
	sep := d.styles.DiffSeparator.Render(fileListSepChar)
	parts := make([]string, height)
	for i := range parts {
		parts[i] = sep
	}
	return strings.Join(parts, "\n")
}

func (d diffViewWidget) renderDiffContent(width, height int) string {
	if d.loading {
		blank := strings.Repeat(" ", width)
		lines := make([]string, height)
		lines[0] = d.styles.DetailMeta.Render(truncateWidth("Loading diff…", width))
		for i := 1; i < height; i++ {
			lines[i] = blank
		}
		return strings.Join(lines, "\n")
	}
	if len(d.files) == 0 {
		blank := strings.Repeat(" ", width)
		lines := make([]string, height)
		lines[0] = d.styles.DetailMeta.Render(truncateWidth("No files changed", width))
		for i := 1; i < height; i++ {
			lines[i] = blank
		}
		return strings.Join(lines, "\n")
	}
	f := d.files[d.fileIdx]
	if f.TooLarge {
		blank := strings.Repeat(" ", width)
		lines := make([]string, height)
		lines[0] = d.styles.ErrorMsg.Render(truncateWidth("File too large to display", width))
		for i := 1; i < height; i++ {
			lines[i] = blank
		}
		return strings.Join(lines, "\n")
	}

	all := d.colorizedLines(f.Diff, width)
	window := d.vp.window(all, height)
	// Pad to fill height
	blank := strings.Repeat(" ", width)
	for len(window) < height {
		window = append(window, blank)
	}
	return strings.Join(window, "\n")
}

func (d diffViewWidget) colorizedLines(diff string, width int) []string {
	rawLines := strings.Split(diff, "\n")
	out := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		var rendered string
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			rendered = d.styles.DiffFileHeader.Render(truncateWidth(line, width))
		case strings.HasPrefix(line, "@@"):
			rendered = d.styles.DiffHunkHeader.Render(truncateWidth(line, width))
		case strings.HasPrefix(line, "+"):
			rendered = d.styles.DiffAdded.Render(truncateWidth(line, width))
		case strings.HasPrefix(line, "-"):
			rendered = d.styles.DiffRemoved.Render(truncateWidth(line, width))
		default:
			rendered = d.styles.DetailBody.Render(truncateWidth(line, width))
		}
		out = append(out, rendered)
	}
	return out
}

// diffStats computes total added/removed lines across all files.
func diffStats(files []domain.FileDiff) (added, removed int) {
	for _, f := range files {
		added += f.LinesAdded
		removed += f.LinesRemoved
	}
	return added, removed
}

// diffFileMarker returns a 3-char status marker for a file in the diff list.
func diffFileMarker(f domain.FileDiff) string {
	switch {
	case f.NewFile:
		return "[+]"
	case f.DeletedFile:
		return "[-]"
	case f.RenamedFile:
		return "[>]"
	default:
		return "[~]"
	}
}
