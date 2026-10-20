package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// Values of the load view settings in state.yaml; the same words as the
// `mrboard load` flags.
const (
	loadKeyPerson = "person"
	loadKeyJira   = "jira"
	loadKeySize   = "size"
)

// loadSelectedMarker replaces the indent of the selected MR line.
const loadSelectedMarker = "▸ "

// loadSettings are the load view's persisted settings (docs/adr/0015, "TUI
// display mode").
type loadSettings struct {
	group     mrsvc.LoadGrouping
	sort      mrsvc.LoadSort
	remaining bool
}

// defaultLoadSettings match the `mrboard load` defaults: by person, heaviest first.
func defaultLoadSettings() loadSettings {
	return loadSettings{group: mrsvc.LoadByPerson, sort: mrsvc.LoadSortSize}
}

// loadSettingsFromState reads the settings saved in state.yaml; an empty or
// unknown value keeps the default.
func loadSettingsFromState(st domain.AppState) loadSettings {
	s := defaultLoadSettings()
	if st.LoadGroup == loadKeyJira {
		s.group = mrsvc.LoadByTicket
	}
	switch st.LoadSort {
	case loadKeyPerson:
		s.sort = mrsvc.LoadSortPerson
	case loadKeyJira:
		s.sort = mrsvc.LoadSortTicket
	}
	s.remaining = st.LoadRemaining
	return s
}

func (s loadSettings) groupKey() string {
	if s.group == mrsvc.LoadByTicket {
		return loadKeyJira
	}
	return loadKeyPerson
}

func (s loadSettings) sortKey() string {
	switch s.sort {
	case mrsvc.LoadSortPerson:
		return loadKeyPerson
	case mrsvc.LoadSortTicket:
		return loadKeyJira
	}
	return loadKeySize
}

// nextSort cycles person → jira → size.
func (s loadSettings) nextSort() mrsvc.LoadSort {
	switch s.sort {
	case mrsvc.LoadSortPerson:
		return mrsvc.LoadSortTicket
	case mrsvc.LoadSortTicket:
		return mrsvc.LoadSortSize
	}
	return mrsvc.LoadSortPerson
}

// indicator is the header's state label for the view, e.g. "size · by person",
// shown after the header's "sort".
func (s loadSettings) indicator() string {
	return s.sortKey() + " · by " + s.groupKey()
}

// loadSettingsChangedMsg tells the root the view's settings changed, so it
// persists them and refreshes the header.
type loadSettingsChangedMsg struct{}

// loadSelection identifies the selected MR line across recomputations: the
// same MR under the same group heading, else the same MR anywhere.
type loadSelection struct {
	group loadGroupID
	mr    domain.MRKey
}

// loadViewWidget is the full-screen review load view: the load tree of the
// CLI with a selection that moves over MR lines only. It owns its settings and
// computes the load from the inputs the root hands it.
type loadViewWidget struct {
	styles   Styles
	keys     LoadKeyMap
	width    int
	height   int
	focused  bool
	settings loadSettings

	// Inputs of the computation; ready is false until a sprint is known.
	mrs    []domain.MergeRequest
	opts   mrsvc.LoadOptions
	sprint LoadSprint
	ready  bool
	// status replaces the tree when the load cannot be computed.
	status string

	load       mrsvc.Load
	lines      []loadTreeLine
	selectable []int // indexes into lines of the MR lines
	sel        int   // index into selectable; -1 when there is none
	selected   loadSelection
	offset     int // first body line shown; the tree's header line is pinned above
}

func newLoadViewWidget(styles Styles, keys LoadKeyMap, settings loadSettings) loadViewWidget {
	return loadViewWidget{styles: styles, keys: keys, settings: settings, focused: true, sel: -1}
}

func (w *loadViewWidget) SetStyles(s Styles) {
	w.styles = s
	w.recompute()
}

func (w *loadViewWidget) SetSize(width, height int) {
	w.width = width
	w.height = height
	w.scrollToSelection()
}

func (w *loadViewWidget) SetFocused(v bool) { w.focused = v }

// Settings returns the view's settings, for the root to persist.
func (w loadViewWidget) Settings() loadSettings { return w.settings }

// SetStatus shows msg instead of the tree, for a load that cannot be computed
// (no active sprint, a sprint read still in flight, ...).
func (w *loadViewWidget) SetStatus(msg string) {
	w.ready = false
	w.status = msg
	w.load = mrsvc.Load{}
	w.lines = nil
	w.selectable = nil
	w.sel = -1
}

// SetData recomputes the load from mrs. opts carries every input but the
// grouping, sort and remaining-only toggle, which the view owns.
func (w *loadViewWidget) SetData(mrs []domain.MergeRequest, opts mrsvc.LoadOptions, sprint LoadSprint) {
	w.mrs = mrs
	w.opts = opts
	w.sprint = sprint
	w.ready = true
	w.status = ""
	w.recompute()
}

func (w *loadViewWidget) recompute() {
	if !w.ready {
		return
	}
	opts := w.opts
	opts.Group = w.settings.group
	opts.Sort = w.settings.sort
	opts.RemainingOnly = w.settings.remaining
	w.load = mrsvc.ComputeLoad(w.mrs, opts)
	w.lines = loadTree(w.load, w.sprint, w.styles)
	w.selectable = w.selectable[:0]
	for i, l := range w.lines {
		if l.entry != nil {
			w.selectable = append(w.selectable, i)
		}
	}
	w.sel = w.findSelection()
	w.scrollToSelection()
}

// findSelection keeps the selected MR under the same heading, else the same MR
// under any heading, else the first line.
func (w *loadViewWidget) findSelection() int {
	if len(w.selectable) == 0 {
		return -1
	}
	sameMR := -1
	for i, li := range w.selectable {
		e := w.lines[li].entry
		if e.MR.Key() != w.selected.mr {
			continue
		}
		if w.lines[li].group == w.selected.group {
			return i
		}
		if sameMR < 0 {
			sameMR = i
		}
	}
	if sameMR >= 0 {
		return sameMR
	}
	w.offset = 0
	w.remember(0)
	return 0
}

func (w *loadViewWidget) remember(sel int) {
	if sel < 0 || sel >= len(w.selectable) {
		return
	}
	li := w.selectable[sel]
	w.selected = loadSelection{group: w.lines[li].group, mr: w.lines[li].entry.MR.Key()}
}

// SelectedMR returns the MR of the selected line, nil when none is selected.
func (w loadViewWidget) SelectedMR() *domain.MergeRequest {
	if w.sel < 0 || w.sel >= len(w.selectable) {
		return nil
	}
	mr := w.lines[w.selectable[w.sel]].entry.MR
	return &mr
}

// Load returns the computed load, for the header's counts.
func (w loadViewWidget) Load() (mrsvc.Load, bool) { return w.load, w.ready }

func (w loadViewWidget) Init() tea.Cmd { return nil }

// Update moves the selection and changes the view's settings; every other key
// of LoadCtx needs the root's resources and is served there.
func (w loadViewWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return w, nil
	}
	switch {
	case w.keys.Up.Match(key):
		w.move(-1)
	case w.keys.Down.Match(key):
		w.move(1)
	case w.keys.Group.Match(key):
		if w.settings.group == mrsvc.LoadByPerson {
			w.settings.group = mrsvc.LoadByTicket
		} else {
			w.settings.group = mrsvc.LoadByPerson
		}
		return w.changed()
	case w.keys.Sort.Match(key):
		w.settings.sort = w.settings.nextSort()
		return w.changed()
	case w.keys.Remaining.Match(key):
		w.settings.remaining = !w.settings.remaining
		return w.changed()
	}
	return w, nil
}

func (w loadViewWidget) changed() (tea.Model, tea.Cmd) {
	w.recompute()
	return w, func() tea.Msg { return loadSettingsChangedMsg{} }
}

// move steps the selection; it stops at the first and last MR line.
func (w *loadViewWidget) move(delta int) {
	if len(w.selectable) == 0 {
		return
	}
	next := w.sel + delta
	if next < 0 || next >= len(w.selectable) {
		return
	}
	w.sel = next
	w.remember(next)
	w.scrollToSelection()
}

// bodyHeight is the number of tree lines shown under the pinned header line.
func (w loadViewWidget) bodyHeight() int {
	return w.height - 1
}

// scrollToSelection keeps the selected line visible and, when it fits, the
// heading of its group too.
func (w *loadViewWidget) scrollToSelection() {
	h := w.bodyHeight()
	body := len(w.lines) - 1
	if h <= 0 || body <= 0 {
		w.offset = 0
		return
	}
	if w.sel >= 0 {
		line := w.selectable[w.sel] - 1 // body index
		top := line
		for i := line; i >= 0; i-- {
			if w.lines[i+1].entry == nil {
				if line-i < h {
					top = i
				}
				break
			}
		}
		if top < w.offset {
			w.offset = top
		}
		if line >= w.offset+h {
			w.offset = line - h + 1
		}
		// The first and last MR lines bring the tree's ends into view: the
		// lines around them (empty groups, footnotes) hold no selection.
		switch w.sel {
		case 0:
			w.offset = 0
		case len(w.selectable) - 1:
			w.offset = max(w.offset, body-h)
		}
	}
	if maxOff := body - h; w.offset > maxOff {
		w.offset = max(maxOff, 0)
	}
}

func (w loadViewWidget) View() tea.View { return tea.NewView(w.render()) }

func (w loadViewWidget) render() string {
	if !w.ready {
		return "\n" + loadIndent + w.styles.LoadNote.Render(w.status)
	}
	out := []string{w.lines[0].text}
	h := w.bodyHeight()
	selLine := -1
	if w.sel >= 0 {
		selLine = w.selectable[w.sel]
	}
	for i := 1 + w.offset; i < len(w.lines) && (h <= 0 || i <= w.offset+h); i++ {
		l := w.lines[i]
		text := l.text
		if i == selLine {
			style := w.styles.LoadSelected
			if !w.focused {
				style = w.styles.LoadSelectedInactive
			}
			text = loadSelectedMarker + style.Render(l.plain)
		}
		out = append(out, text)
	}
	if w.width > 0 {
		for i, s := range out {
			out[i] = ansi.Truncate(s, w.width, "…")
		}
	}
	return strings.Join(out, "\n")
}
