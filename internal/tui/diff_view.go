package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

const (
	fileListWidth      = 28
	fileListSepChar    = "│"
	halfPageDivisor    = 2
	sideBySideMinWidth = 180
	tempFilePerm       = 0o600
	tempDirPerm        = 0o700
)

// difftBin is the path to the difft binary, discovered once at startup.
// Empty string means difft is unavailable; use the colorized fallback.
var difftBin string

func init() {
	difftBin, _ = exec.LookPath("difft") //nolint:errcheck // not found → empty string, which is the intended fallback
}

// diffViewWidget renders a full-screen diff view for a single MR.
// Layout: file list (left, 28 cols) | separator | diff content (right, scrollable).
//
// Rendering is lazy and cached per file: the model dispatches an async cmd
// when a file has no cached render yet. Until the result arrives, a spinner
// placeholder is shown. rendered/rendering are maps (reference types) so
// mutations survive Bubble Tea's value-copy semantics.
type diffViewWidget struct {
	mr      *domain.MergeRequest
	baseSHA string
	headSHA string
	files   []domain.FileDiff
	fileIdx int
	vp      scrollViewport
	styles  Styles
	width   int
	height  int
	loading bool // true until SetDiff is called (waiting for initial MRDiff)

	// rendered caches pre-rendered lines per file index.
	// Being a map (reference type), it is shared across all Model value copies.
	rendered  map[int][]string
	rendering map[int]bool // true while an async render cmd is in-flight
}

func newDiffViewWidget(styles Styles) diffViewWidget {
	return diffViewWidget{
		styles:    styles,
		rendered:  make(map[int][]string),
		rendering: make(map[int]bool),
	}
}

func (d *diffViewWidget) SetStyles(s Styles) { d.styles = s }

func (d *diffViewWidget) SetMR(mr *domain.MergeRequest) {
	d.mr = mr
	d.files = nil
	d.baseSHA = ""
	d.headSHA = ""
	d.fileIdx = 0
	d.vp.reset()
	d.loading = true
	d.rendered = make(map[int][]string)
	d.rendering = make(map[int]bool)
}

func (d *diffViewWidget) SetDiff(diff domain.MRDiff) {
	d.files = diff.Files
	d.baseSHA = diff.BaseSHA
	d.headSHA = diff.HeadSHA
	d.fileIdx = 0
	d.vp.reset()
	d.loading = false
}

// HasRendered reports whether file idx has cached rendered lines.
func (d *diffViewWidget) HasRendered(idx int) bool {
	_, ok := d.rendered[idx]
	return ok
}

// IsRendering reports whether an async render is in-flight for file idx.
func (d *diffViewWidget) IsRendering(idx int) bool { return d.rendering[idx] }

// SetRendering marks file idx as having an in-flight render cmd.
func (d *diffViewWidget) SetRendering(idx int) { d.rendering[idx] = true }

// SetRendered stores pre-rendered lines for file idx and clears its in-flight flag.
func (d *diffViewWidget) SetRendered(idx int, lines []string) {
	delete(d.rendering, idx)
	d.rendered[idx] = lines
}

// RenderFallback synchronously computes colorized lines for file idx using the
// lipgloss-based fallback and stores the result in the cache. Used when difft
// is unavailable so no async cmd is needed.
func (d *diffViewWidget) RenderFallback(idx int) {
	if idx >= len(d.files) {
		return
	}
	f := d.files[idx]
	d.rendered[idx] = d.colorizedLines(f.Diff, d.diffPaneWidth())
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

func (d *diffViewWidget) currentLines() []string { return d.rendered[d.fileIdx] }

func (d *diffViewWidget) ScrollUp() { d.vp.scrollUp() }
func (d *diffViewWidget) ScrollDown() {
	d.vp.scrollDown(len(d.currentLines()), d.diffBodyLines())
}

func (d *diffViewWidget) HalfPageUp() {
	d.vp.scrollHalfPageUp(d.diffBodyLines() / halfPageDivisor)
}

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
		padded := lip.NewStyle().Width(fileListWidth).Render(fmt.Sprintf("%s %s", marker, name))
		if i == d.fileIdx {
			lines = append(lines, d.styles.DiffFileSelected.Render(padded))
		} else {
			lines = append(lines, d.styles.DiffFileItem.Render(padded))
		}
	}

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
	blank := strings.Repeat(" ", width)
	padLines := func(lines []string) string {
		for len(lines) < height {
			lines = append(lines, blank)
		}
		return strings.Join(lines, "\n")
	}
	placeholder := func(msg string) string {
		out := make([]string, height)
		out[0] = d.styles.DetailMeta.Render(truncateWidth(msg, width))
		for i := 1; i < height; i++ {
			out[i] = blank
		}
		return strings.Join(out, "\n")
	}

	if d.loading {
		return placeholder("⠋ Loading diff…")
	}
	if len(d.files) == 0 {
		return placeholder("No files changed")
	}

	f := d.files[d.fileIdx]
	if f.TooLarge {
		return d.styles.ErrorMsg.Render(truncateWidth("File too large to display", width))
	}

	all := d.currentLines()
	if all == nil {
		// Per-file async render in-flight: show spinner.
		name := filepath.Base(f.NewPath)
		if f.DeletedFile {
			name = filepath.Base(f.OldPath)
		}
		return placeholder(fmt.Sprintf("⠋ Rendering %s…", name))
	}

	window := d.vp.window(all, height)
	return padLines(window)
}

// colorizedLines renders a unified diff string with lipgloss syntax highlighting.
// Used as the fallback when difft is not available.
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

// diffFileMarker returns a 3-char status prefix for the file list.
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

// runDifft runs the difft binary on old/new content and returns the output lines.
// Pass nil for oldContent (new file) or newContent (deleted file) to use /dev/null.
func runDifft(oldContent, newContent []byte, oldName, newName string, width int) ([]string, error) {
	dir, err := os.MkdirTemp("", "mrboard-diff-*")
	if err != nil {
		return nil, fmt.Errorf("difft: mktemp: %w", err)
	}
	defer os.RemoveAll(dir)

	// Use a/b subdirs so difft displays "a/filename" and "b/filename" instead of the full temp path.
	for _, sub := range []string{"a", "b"} {
		if err := os.Mkdir(filepath.Join(dir, sub), tempDirPerm); err != nil {
			return nil, fmt.Errorf("difft: mkdir %s: %w", sub, err)
		}
	}

	oldPath := "/dev/null"
	if oldContent != nil {
		oldPath = filepath.Join(dir, "a", filepath.Base(oldName))
		if err := os.WriteFile(oldPath, oldContent, tempFilePerm); err != nil {
			return nil, fmt.Errorf("difft: write old: %w", err)
		}
	}

	newPath := "/dev/null"
	if newContent != nil {
		newPath = filepath.Join(dir, "b", filepath.Base(newName))
		if err := os.WriteFile(newPath, newContent, tempFilePerm); err != nil {
			return nil, fmt.Errorf("difft: write new: %w", err)
		}
	}

	display := "inline"
	if width >= sideBySideMinWidth {
		display = "side-by-side"
	}

	// difftBin is set by exec.LookPath in init() — path is safe.
	cmd := exec.Command(difftBin, //nolint:gosec
		"--display", display,
		"--width", strconv.Itoa(width),
		"--color", "always",
		oldPath, newPath)
	out, err := cmd.Output()
	// difft exits 1 when files differ (expected); only fail when there is no output.
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("difft: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("difft produced no output")
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n"), nil
}
