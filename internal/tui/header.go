package tui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// ageJustNow is the threshold below which the snapshot age reads "just now"
// rather than domain.FormatDuration's "< 1m".
const ageJustNow = 10 * time.Second

const (
	// headerMargin is the blank run kept at each end of the header line.
	headerMargin = 1
	// headerZoneGap is the minimum blank run between two header zones.
	headerZoneGap = 2
	// headerSegSep separates the filter bar's segments.
	headerSegSep = " · "
)

// headerFilterState summarizes every active filter dimension for the header's
// left-hand filter bar. The zero value means the board is showing everything
// it fetched.
//
// The two bool fields are scopes the user stands in and toggles from the
// board; the int fields are how many values each exclusion list hides. No
// field carries a name: the bar's job is to report that a dimension is
// narrowing the board and by how much, and the Filters tab answers which.
type headerFilterState struct {
	Mine      bool
	Sprint    bool
	Columns   int // phase columns hidden, of the four the board draws
	Assignees int
	Reviewers int
	Tickets   int // excluded issue IDs, counting "no ID" as one
}

func (s headerFilterState) active() bool {
	return s.Mine || s.Sprint ||
		s.Columns > 0 || s.Assignees > 0 || s.Reviewers > 0 || s.Tickets > 0
}

// headerPiece identifies a header element that may be dropped whole when the
// terminal is too narrow, in the order pieces are sacrificed. The MR count is
// not a piece: it is never dropped.
//
// Reductions outrank the title and the sort mode because a filter the user has
// forgotten about silently misrepresents the board, while a missing app name
// costs nothing. Among the reductions, hidden people survive longest — they are
// the exclusions most likely to hide someone's work from a standup. Scopes
// survive longest of all: they are toggled constantly and change what every
// other number on the line means.
type headerPiece int

const (
	pieceTitle headerPiece = iota
	pieceSort
	pieceTickets
	pieceReviewers
	pieceColumns
	pieceAssignees
	pieceAge
	pieceSprint
	pieceMine
	headerPieceCount
)

// headerPieces tracks which pieces still fit. An entry is false either because
// the piece has no content or because it was dropped for width.
type headerPieces [headerPieceCount]bool

// dropNext sacrifices the lowest-ranked surviving piece, reporting whether
// anything was left to drop.
func (p *headerPieces) dropNext() bool {
	for i := headerPiece(0); i < headerPieceCount; i++ {
		if p[i] {
			p[i] = false
			return true
		}
	}
	return false
}

type headerWidget struct {
	styles            Styles
	mrs               []domain.MergeRequest
	total             int // MRs available to filter; the filter bar's denominator
	width             int
	title             string
	filter            headerFilterState
	statsOverride     string
	sortIndicator     string // current sort mode, e.g. "repo·id↑"; state moved out of key labels
	snapshotWrittenAt time.Time
	refreshing        bool
	spinnerFrame      string
}

func newHeaderWidget(styles Styles) headerWidget {
	return headerWidget{styles: styles, title: "mrboard"}
}

func (h *headerWidget) SetStyles(s Styles)               { h.styles = s }
func (h *headerWidget) SetWidth(w int)                   { h.width = w }
func (h *headerWidget) SetMRs(mrs []domain.MergeRequest) { h.mrs = mrs }
func (h *headerWidget) SetTitle(t string)                { h.title = t }
func (h *headerWidget) SetStats(s string)                { h.statsOverride = s }
func (h *headerWidget) SetSort(label string)             { h.sortIndicator = label }

// SetFilterState records the filter summary and the pre-filter MR count the
// bar reports against. total is the population the filters were applied to, so
// that every segment on the bar accounts for part of the gap between it and
// the MRs actually on the board.
func (h *headerWidget) SetFilterState(total int, s headerFilterState) {
	h.total = total
	h.filter = s
}

// SetSnapshotAge records when the currently displayed board data was captured
// and whether a fetch is in flight, so render can show "⠿ 14m ago" collapsing
// to "just now" the moment a swap lands (docs/adr/0005, "Non-blocking refresh").
// A zero writtenAt (nothing cached yet) renders no age segment at all.
func (h *headerWidget) SetSnapshotAge(writtenAt time.Time, refreshing bool, spinnerFrame string) {
	h.snapshotWrittenAt = writtenAt
	h.refreshing = refreshing
	h.spinnerFrame = spinnerFrame
}

func (h headerWidget) ageLabel() string {
	if h.snapshotWrittenAt.IsZero() {
		return ""
	}
	age := time.Since(h.snapshotWrittenAt)
	label := "just now"
	if age >= ageJustNow {
		label = domain.FormatDuration(age) + " ago"
	}
	if h.refreshing {
		return h.spinnerFrame + " " + label
	}
	return label
}

func (h headerWidget) Init() tea.Cmd                         { return nil }
func (h headerWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return h, nil }
func (h headerWidget) View() tea.View                        { return tea.NewView(h.render()) }

// borrowed reports whether an overlay has taken the header over for its own
// caption (the diff view sets both a title and a stats override). The filter
// bar is suppressed there: its counts describe a board that is no longer
// on screen.
func (h headerWidget) borrowed() bool { return h.statsOverride != "" }

func (h headerWidget) render() string {
	pieces := h.presentPieces()
	// An unset width is "not measured yet", not "no room": dropping against it
	// would sacrifice every piece before the first resize arrives.
	if h.width <= 0 {
		return h.compose(h.zones(pieces))
	}
	for {
		z := h.zones(pieces)
		if z.width() <= h.width || !pieces.dropNext() {
			return h.compose(z)
		}
	}
}

// presentPieces marks every piece that has something to show, before any
// width-driven dropping.
func (h headerWidget) presentPieces() headerPieces {
	var p headerPieces
	p[pieceTitle] = h.title != ""
	if h.borrowed() {
		return p
	}
	p[pieceSort] = h.sortIndicator != ""
	p[pieceAge] = !h.snapshotWrittenAt.IsZero()
	p[pieceMine] = h.filter.Mine
	p[pieceSprint] = h.filter.Sprint
	p[pieceColumns] = h.filter.Columns > 0
	p[pieceAssignees] = h.filter.Assignees > 0
	p[pieceReviewers] = h.filter.Reviewers > 0
	p[pieceTickets] = h.filter.Tickets > 0
	return p
}

// headerZones is the header's three rendered regions plus their widths:
// filters on the left, identity in the middle, board chrome on the right.
type headerZones struct {
	bar, title, right    string
	barW, titleW, rightW int
}

func (z headerZones) width() int {
	w := headerMargin + z.barW + headerMargin
	if z.titleW > 0 {
		w += headerZoneGap + z.titleW
	}
	if z.rightW > 0 {
		w += headerZoneGap + z.rightW
	}
	return w
}

func (h headerWidget) zones(p headerPieces) headerZones {
	z := headerZones{bar: h.renderBar(p), right: h.renderRight(p)}
	if p[pieceTitle] {
		z.title = h.styles.HeaderTitle.Inherit(h.styles.Header).Render(h.title)
	}
	z.barW, z.titleW, z.rightW = lip.Width(z.bar), lip.Width(z.title), lip.Width(z.right)
	return z
}

// filterSegment is one labelled piece of the filter bar. scope segments are
// bare words for a mode the user is standing in; the rest are "tag×n" counts
// of values being withheld.
type filterSegment struct {
	piece headerPiece
	text  string
	scope bool
}

func (h headerWidget) filterSegments() []filterSegment {
	var segs []filterSegment
	if h.filter.Mine {
		segs = append(segs, filterSegment{piece: pieceMine, text: "me", scope: true})
	}
	if h.filter.Sprint {
		segs = append(segs, filterSegment{piece: pieceSprint, text: "sprint", scope: true})
	}
	for _, r := range []struct {
		piece headerPiece
		tag   string
		n     int
	}{
		{pieceColumns, "col", h.filter.Columns},
		{pieceAssignees, "asg", h.filter.Assignees},
		{pieceReviewers, "rev", h.filter.Reviewers},
		{pieceTickets, "tkt", h.filter.Tickets},
	} {
		if r.n > 0 {
			segs = append(segs, filterSegment{piece: r.piece, text: r.tag + "×" + strconv.Itoa(r.n)})
		}
	}
	return segs
}

// shownTotal is the "n of m" the bar leads with. The denominator falls back to
// the numerator until SetFilterState has run, so a half-wired header never
// claims MRs are hidden.
func (h headerWidget) shownTotal() (shown, total int) {
	shown = len(h.mrs)
	total = h.total
	if total < shown {
		total = shown
	}
	return shown, total
}

// renderBar builds the left-hand filter bar: the MR count, then one segment
// per surviving active filter, then "+n" for any dropped for width. With no
// filter active it collapses to a bare count on the header background — the
// unfiltered board says nothing it does not have to.
func (h headerWidget) renderBar(p headerPieces) string {
	if h.borrowed() {
		return ""
	}
	shown, total := h.shownTotal()
	if !h.filter.active() {
		return h.styles.HeaderStats.Inherit(h.styles.Header).
			Render(strconv.Itoa(shown) + " mrs")
	}

	bar := h.styles.HeaderFilterBar
	sep := h.styles.HeaderFilterSep.Inherit(bar).Render(headerSegSep)
	// The slash is what tells the user filtering is on at all; it carries the
	// whole of the old "[filtered]" tag inside a number they already read.
	content := h.styles.HeaderCountShown.Inherit(bar).Render(strconv.Itoa(shown)) +
		h.styles.HeaderCountTotal.Inherit(bar).Render("/"+strconv.Itoa(total)+" mrs")

	dropped := 0
	for _, s := range h.filterSegments() {
		if !p[s.piece] {
			dropped++
			continue
		}
		style := h.styles.HeaderFilterReduce
		if s.scope {
			style = h.styles.HeaderFilterScope
		}
		content += sep + style.Inherit(bar).Render(s.text)
	}
	if dropped > 0 {
		content += sep + h.styles.HeaderCountTotal.Inherit(bar).Render("+"+strconv.Itoa(dropped))
	}

	pad := bar.Render(" ")
	return pad + content + pad
}

// renderRight builds the board chrome pinned to the right edge: snapshot age
// and sort mode, or an overlay's stats override in place of both.
func (h headerWidget) renderRight(p headerPieces) string {
	style := h.styles.HeaderStats.Inherit(h.styles.Header)
	if h.borrowed() {
		return style.Render(h.statsOverride)
	}
	var parts []string
	if age := h.ageSegment(p[pieceAge]); age != "" {
		parts = append(parts, age)
	}
	if p[pieceSort] {
		parts = append(parts, "sort "+h.sortIndicator)
	}
	if len(parts) == 0 {
		return ""
	}
	return style.Render(strings.Join(parts, "  "))
}

// ageSegment renders the snapshot age, or — once the age itself has been
// dropped for width — the bare spinner frame, so an in-flight fetch stays
// visible on a terminal too narrow for anything else.
func (h headerWidget) ageSegment(full bool) string {
	if full {
		return h.ageLabel()
	}
	if h.refreshing {
		return h.spinnerFrame
	}
	return ""
}

// compose lays the three zones across the full width. The title is centred on
// the terminal and then clamped rightwards off the bar, rather than re-centred
// in the space the bar leaves: clamping holds it still until the bar actually
// reaches it, instead of nudging it on every filter keystroke.
func (h headerWidget) compose(z headerZones) string {
	bg := h.styles.Header
	if h.width <= 0 || z.width() > h.width {
		parts := make([]string, 0, 3) //nolint:mnd // bar, title, right
		for _, s := range []string{z.bar, z.title, z.right} {
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, bg.Render(" "))
	}

	gap := func(n int) string {
		if n < 0 {
			n = 0
		}
		return bg.Render(strings.Repeat(" ", n))
	}

	line := gap(headerMargin) + z.bar
	col := headerMargin + z.barW

	if z.titleW > 0 {
		start := (h.width - z.titleW) / 2 //nolint:mnd // centred on the terminal
		if start < col+headerZoneGap {
			start = col + headerZoneGap
		}
		if limit := h.width - headerMargin - z.rightW - headerZoneGap - z.titleW; z.rightW > 0 && start > limit {
			start = limit
		}
		line += gap(start-col) + z.title
		col = start + z.titleW
	}

	if z.rightW > 0 {
		start := h.width - headerMargin - z.rightW
		line += gap(start-col) + z.right
		col = start + z.rightW
	}
	return line + gap(h.width-col)
}
