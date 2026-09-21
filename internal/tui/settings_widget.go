package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// --- Phase/filter sub-widget helpers ---

const (
	phaseLabelDraft    = "Draft"
	phaseLabelReview   = "Needs Review"
	phaseLabelAuthorAc = "Needs Author Action"
	phaseLabelReady    = "Approved"

	markerChecked   = "[x]"
	markerUnchecked = "[ ]"
	markerFixed     = "[•]" // always-applied row with no toggle, e.g. the focused MR in the batch preview

	filterSelectMaxVisible    = 8 // fallback used before SetSize sizes the panel to the terminal
	filterSelectMaxVisibleCap = 14
	// settingsMinVisible is the floor SetSize clamps any tab's visible-row
	// count to, however short the terminal.
	settingsMinVisible = 3
	// filterFixedChromeLines is every Filters-tab line but the list rows
	// themselves: border+tabbar+blank+status+view+rule+header+indicator+
	// blank+hint. See SetSize.
	filterFixedChromeLines = 11
	// filterColumnMinContentWidth/filterColumnMaxContentWidth bound the
	// label+count width inside a filter list column; SetSize derives the
	// actual width from the terminal width between these two.
	// filterColumnMaxContentWidth comfortably covers a long full name (e.g.
	// "Jean-Philippe Dubois" is 21 chars) — past it, extra width is just
	// padding, not legibility.
	filterColumnMinContentWidth = 22
	filterColumnMaxContentWidth = 34
	// filterMarkerPrefixWidth is "  " + "[x]" + " " preceding a list row's content.
	filterMarkerPrefixWidth = 6
)

// filterColumnTotalWidth is a column's full on-screen width: the row marker
// prefix plus the label+count content width.
func filterColumnTotalWidth(contentWidth int) int {
	return filterMarkerPrefixWidth + contentWidth
}

var phaseLabels = [4]string{phaseLabelDraft, phaseLabelReview, phaseLabelAuthorAc, phaseLabelReady}

// --- Theme mode helpers ---

var modeOptions = []string{themeModeAuto, themeModeDark, themeModeLight}

const pickerRadioPrefixLen = 2 // "● " or "○ "

// filterFocus identifies which section of the Filters tab owns keyboard focus.
type filterFocus int

const (
	filterFocusStatus filterFocus = iota
	filterFocusView
	filterFocusAssignee
	filterFocusReviewer
	filterFocusTicket
	filterNumSections
)

// filterStatusWidget manages the Status (phase) section checkboxes, rendered
// as a single horizontal strip — Left/Right moves between them, matching how
// the strip actually reads on screen.
type filterStatusWidget struct {
	phases [4]bool
	cursor int
}

func (s *filterStatusWidget) toggle() {
	if s.cursor < len(s.phases) {
		s.phases[s.cursor] = !s.phases[s.cursor]
	}
}

func (s filterStatusWidget) render(focused bool, styles Styles) string {
	parts := make([]string, len(phaseLabels))
	for i, lbl := range phaseLabels {
		marker := markerUnchecked
		markerStyle := styles.PopupItemMarkerOff
		if s.phases[i] {
			marker = markerChecked
			markerStyle = styles.PopupItemMarkerOn
		}
		markerStyled := markerStyle.Render(marker)
		var labelStyled string
		if focused && i == s.cursor {
			labelStyled = styles.PopupItemFocused.Render(lbl)
		} else {
			labelStyled = styles.PopupItem.Render(lbl)
		}
		parts[i] = markerStyled + " " + labelStyled
	}
	return strings.Join(parts, "   ")
}

// filterViewItem is one checkbox in filterViewWidget's strip.
type filterViewItem struct {
	label   string
	checked bool
}

// filterViewWidget manages the "My MRs only" / "Current sprint" checkboxes
// — a strip surfacing the same view-scoping state the tab and S keybindings
// already control (Model.viewMode, Model.sprintFilterActive), so it's
// visible and toggleable from the Filters tab too instead of being
// invisible outside a header badge. Each checkbox is omitted entirely when
// its underlying mechanism isn't available (no current user configured, no
// JIRA board configured), matching how those keybindings are themselves
// disabled in that case.
type filterViewWidget struct {
	myMRsOnly   bool
	myMRsAvail  bool
	sprint      bool
	sprintAvail bool
	cursor      int
}

// items returns the currently available checkboxes, in display order.
func (v filterViewWidget) items() []filterViewItem {
	items := make([]filterViewItem, 0, 2) //nolint:mnd
	if v.myMRsAvail {
		items = append(items, filterViewItem{label: "My MRs only", checked: v.myMRsOnly})
	}
	if v.sprintAvail {
		items = append(items, filterViewItem{label: "Current sprint", checked: v.sprint})
	}
	return items
}

func (v *filterViewWidget) toggle() {
	items := v.items()
	if v.cursor >= len(items) {
		return
	}
	switch items[v.cursor].label {
	case "My MRs only":
		v.myMRsOnly = !v.myMRsOnly
	case "Current sprint":
		v.sprint = !v.sprint
	}
}

func (v filterViewWidget) render(focused bool, styles Styles) string {
	items := v.items()
	parts := make([]string, len(items))
	for i, it := range items {
		marker := markerUnchecked
		markerStyle := styles.PopupItemMarkerOff
		if it.checked {
			marker = markerChecked
			markerStyle = styles.PopupItemMarkerOn
		}
		markerStyled := markerStyle.Render(marker)
		var labelStyled string
		if focused && i == v.cursor {
			labelStyled = styles.PopupItemFocused.Render(it.label)
		} else {
			labelStyled = styles.PopupItem.Render(it.label)
		}
		parts[i] = markerStyled + " " + labelStyled
	}
	return strings.Join(parts, "   ")
}

// filterItemKind discriminates the "No ID" pseudo-item from a real
// selectable value in a filterSelectWidget list.
type filterItemKind int

const (
	filterItemValue filterItemKind = iota
	filterItemNone
)

// filterSelectItem is a single entry in a multi-select list (Assignee,
// Reviewer, or Issue ID). count is an MR-count badge shown right-aligned;
// zero means no badge unless absent is set. absent marks a value that is
// excluded in persisted state but no longer present in the current MR set
// (e.g. a ticket ID from a closed sprint) — without this, such a selection
// filters invisibly: still applied, but with no row to show or re-include it.
// short is the compact-mode label (filterSelectWidget.compact); for
// Assignee/Reviewer it's the bare "@username", for Issue ID it equals label.
type filterSelectItem struct {
	kind   filterItemKind
	value  string // "" for kind != filterItemValue
	label  string
	short  string
	count  int
	absent bool
}

// filterSelectWidget manages a scrollable multi-select list: every entry is
// shown by default (checkbox on), and unchecking one hides it from the
// board. For the Issue ID list only, one entry is the "No ID" pseudo-item.
type filterSelectWidget struct {
	items        []filterSelectItem
	excluded     map[string]bool // true = hidden from the board; nil/empty = show everything
	noneExcluded bool            // "No ID" unchecked — meaningful for the Issue ID list only
	cursor       int
	scrollOff    int
	maxVisible   int  // 0 falls back to filterSelectMaxVisible; set by settingsWidget.SetSize
	colWidth     int  // 0 falls back to filterColumnMinContentWidth; set by settingsWidget.SetSize
	compact      bool // show filterSelectItem.short instead of .label; set by settingsWidget
}

func (s *filterSelectWidget) moveCursor(delta int) {
	next := s.cursor + delta
	if next >= 0 && next < len(s.items) {
		s.cursor = next
		s.adjustScroll()
	}
}

// displayLabel is the label to render for item, honoring compact mode.
func (s filterSelectWidget) displayLabel(item filterSelectItem) string {
	if s.compact {
		return item.short
	}
	return item.label
}

func (s filterSelectWidget) effectiveColWidth() int {
	if s.colWidth > 0 {
		return s.colWidth
	}
	return filterColumnMinContentWidth
}

func (s filterSelectWidget) effectiveMaxVisible() int {
	if s.maxVisible > 0 {
		return s.maxVisible
	}
	return filterSelectMaxVisible
}

func (s *filterSelectWidget) adjustScroll() {
	mv := s.effectiveMaxVisible()
	if s.cursor < s.scrollOff {
		s.scrollOff = s.cursor
	} else if s.cursor >= s.scrollOff+mv {
		s.scrollOff = s.cursor - mv + 1
	}
}

func (s *filterSelectWidget) toggle() {
	if s.cursor >= len(s.items) {
		return
	}
	item := s.items[s.cursor]
	switch item.kind {
	case filterItemNone:
		s.noneExcluded = !s.noneExcluded
	case filterItemValue:
		if s.excluded == nil {
			s.excluded = make(map[string]bool)
		}
		if s.excluded[item.value] {
			delete(s.excluded, item.value)
			if len(s.excluded) == 0 {
				s.excluded = nil
			}
		} else {
			s.excluded[item.value] = true
		}
	}
}

// toggleSelectAll implements the select-all/clear-all binding: when
// everything is currently shown, it excludes every real value (and "No ID"
// where present); otherwise it clears every exclusion back to showing
// everything. This is a two-state toggle, not a restore of whatever was
// excluded before the last toggle.
func (s *filterSelectWidget) toggleSelectAll() {
	if len(s.excluded) == 0 && !s.noneExcluded {
		s.excluded = make(map[string]bool, len(s.items))
		hasNone := false
		for _, it := range s.items {
			switch it.kind {
			case filterItemValue:
				s.excluded[it.value] = true
			case filterItemNone:
				hasNone = true
			}
		}
		s.noneExcluded = hasNone
		return
	}
	s.excluded = nil
	s.noneExcluded = false
}

func (s filterSelectWidget) excludedSlice() []string {
	result := make([]string, 0, len(s.excluded))
	for v := range s.excluded {
		result = append(result, v)
	}
	sort.Strings(result)
	return result
}

// activeCount is how many values are currently excluded from this list —
// shown as a badge on the column header. Not to be confused with an item's
// own per-value MR count.
func (s filterSelectWidget) activeCount() int {
	n := len(s.excluded)
	if s.noneExcluded {
		n++
	}
	return n
}

func (s filterSelectWidget) isChecked(item filterSelectItem) bool {
	switch item.kind {
	case filterItemNone:
		return !s.noneExcluded
	default:
		return !s.excluded[item.value]
	}
}

// render renders this list as a fixed-width, fixed-height block (padded with
// blank rows to effectiveMaxVisible) so several lists can sit side by side
// via lip.JoinHorizontal without ragged edges.
func (s filterSelectWidget) render(focused bool, styles Styles) string {
	var sb strings.Builder
	mv := s.effectiveMaxVisible()
	cw := s.effectiveColWidth()
	end := min(s.scrollOff+mv, len(s.items))
	rows := 0
	for i := s.scrollOff; i < end; i++ {
		item := s.items[i]
		var markerStyled string
		if s.isChecked(item) {
			markerStyled = styles.PopupItemMarkerOn.Render(markerChecked)
		} else {
			markerStyled = styles.PopupItemMarkerOff.Render(markerUnchecked)
		}
		content := renderFilterRowContent(s.displayLabel(item), item.count, item.absent, cw)
		var contentStyled string
		switch {
		case item.absent:
			contentStyled = styles.PopupHint.Render(content)
		case focused && i == s.cursor:
			contentStyled = styles.PopupItemFocused.Render(content)
		default:
			contentStyled = styles.PopupItem.Render(content)
		}
		sb.WriteString("  " + markerStyled + " " + contentStyled + "\n")
		rows++
	}
	blank := strings.Repeat(" ", filterColumnTotalWidth(cw))
	for rows < mv {
		sb.WriteString(blank + "\n")
		rows++
	}
	if len(s.items) > mv {
		sb.WriteString(styles.PopupHint.Render(fmt.Sprintf("  %d–%d / %d", s.scrollOff+1, end, len(s.items))) + "\n")
	} else {
		sb.WriteString(blank + "\n")
	}
	return sb.String()
}

// renderFilterRowContent lays out a label with its optional right-aligned
// count badge inside width columns, truncating the label if it doesn't fit
// (see fitLabel). showZero forces the "(0)" badge for an absent-but-checked
// item even though count itself is 0.
func renderFilterRowContent(label string, count int, showZero bool, width int) string {
	countStr := ""
	if count > 0 || showZero {
		countStr = fmt.Sprintf("(%d)", count)
	}
	avail := width
	if countStr != "" {
		avail -= lip.Width(countStr) + 1
	}
	if avail < 1 {
		avail = 1
	}
	label = truncateWidth(label, avail)
	pad := avail - lip.Width(label)
	if pad < 0 {
		pad = 0
	}
	content := label + strings.Repeat(" ", pad)
	if countStr != "" {
		content += " " + countStr
	}
	return content
}

func renderSectionHeader(title string, focused bool, styles Styles) string {
	if focused {
		return styles.PopupSectionFocused.Render("▶ " + title)
	}
	return styles.PopupSection.Render("  " + title)
}

// padDisplay right-pads an already-styled string to width, measuring visible
// (ANSI-stripped) width so styled and plain strings can share a column.
func padDisplay(s string, width int) string {
	w := lip.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// --- SettingsAppliedMsg / SettingsClosedMsg ---

// SettingsAppliedMsg is emitted on every live change in the settings panel.
type SettingsAppliedMsg struct {
	Filter             domain.FilterCriteria
	ViewMine           bool // "My MRs only" — mirrors Model.viewMode == domain.ViewMine
	SprintFilter       bool // "Current sprint" — mirrors Model.sprintFilterActive
	IncludeReviewerMRs bool
	SortField          string // "repo_iid" | "author" | "age"
	SortDesc           bool
	ThemeName          string
	ThemeMode          string
}

// SettingsClosedMsg is sent when the settings panel is closed.
type SettingsClosedMsg struct{}

type settingsTab int

const (
	tabGeneral settingsTab = iota
	tabFilters
	tabSorting
	tabTheme
	numSettingsTabs
)

var settingsTabLabels = [numSettingsTabs]string{"General", "Filters", "Sorting", "Theme"}

// sorting tab cursor positions
const (
	sortCursorRepoIID    = 0
	sortCursorAssignee   = 1
	sortCursorAge        = 2
	sortCursorAscending  = 3
	sortCursorDescending = 4
	sortNumCursors       = 5
)

const (
	settingsPickerMaxVisible = 10 // cap; SetSize may lower it to fit a short terminal
	settingsPickerListWidth  = 22
	settingsModeWidth        = 10
	sortColumnWidth          = 22
	// settingsFrameChromeWidth/Height are the modal's border+padding+tab-bar+hint
	// overhead outside a tab's body — see canvasSize.
	settingsFrameChromeWidth  = 4 // border (2) + PopupBorder padding (2)
	settingsFrameChromeHeight = 6 // border (2) + tab bar (1) + blank (1) + blank (1) + hint (1)
	// settingsHintText is the footer hint shown under every tab; hoisted to a
	// const so canvasSize can measure it alongside each tab's body.
	settingsHintText = "  tab/shift+tab tabs  ↑↓←→ move  space toggle  ^a select/clear all  n names/@ids  ,/esc close"
)

// settingsWidget is a 4-tab settings panel: General / Filters / Sorting / Theme.
type settingsWidget struct {
	styles        Styles
	keys          SettingsKeyMap
	tab           settingsTab
	width, height int

	// General tab
	includeReviewerMRs bool

	// Filters tab
	filterStatus   filterStatusWidget
	filterView     filterViewWidget
	filterAssignee filterSelectWidget
	filterReviewer filterSelectWidget
	filterTicket   filterSelectWidget
	filterFocused  filterFocus
	filterLastList filterFocus // column to return to when leaving the Status strip
	filterColWidth int         // 0 falls back to filterColumnMinContentWidth; set by SetSize
	filterCompact  bool        // show @username instead of "Full Name (@username)"

	// Sorting tab
	sortCursor  int // 0–4
	sortSection int // 0 = field, 1 = direction
	sortField   sortField
	sortDesc    bool

	// Theme tab
	themes          []string
	themeCursor     int
	themeScrollOff  int
	themeModeCursor int
	themeSection    int // 0 = list, 1 = mode
	themeMaxVisible int // 0 falls back to settingsPickerMaxVisible; set by SetSize

	// current persisted theme values (updated on each live change)
	themeName string
	themeMode string
}

// TicketKeyCount pairs an extracted issue ID with how many MRs in the current
// set carry it.
type TicketKeyCount struct {
	Key   string
	Count int
}

// newSettingsWidget constructs a settingsWidget populated from current app state.
// authors and reviewers are sorted username slices; tickets and ticketNoneCount
// are the issue-ID breakdown (see BuildTicketKeys). All three populate the
// Filters tab. viewState carries the current on/off state of the "My MRs
// only" / "Current sprint" toggles and which of them apply to this
// deployment — see filterViewWidget.
func newSettingsWidget(
	themes []string,
	authors, reviewers []string,
	tickets []TicketKeyCount, ticketNoneCount int,
	userMap map[string]string,
	filter domain.FilterCriteria,
	viewState filterViewWidget,
	includeReviewerMRs bool,
	currentSortField sortField,
	currentSortDesc bool,
	currentThemeName, currentThemeMode string,
	styles Styles,
	keys SettingsKeyMap,
	initialTab settingsTab,
) settingsWidget {
	// --- Filters tab init ---
	phaseState := [4]bool{true, true, true, true}
	if len(filter.Phases) > 0 {
		phaseState = [4]bool{}
		for i := range phaseState {
			phaseState[i] = filter.Phases[domain.MRPhase(i)]
		}
	}
	assigneeExcluded := excludedSet(filter.ExcludedAssignees)
	reviewerExcluded := excludedSet(filter.ExcludedReviewers)
	ticketExcluded := excludedSet(filter.ExcludedTicketKeys)
	authorItems := buildSelectItems(authors, userMap, assigneeExcluded)
	reviewerItems := buildSelectItems(reviewers, userMap, reviewerExcluded)
	ticketItems := buildTicketItems(tickets, ticketNoneCount, ticketExcluded)

	// --- Sorting tab init ---
	var sc int
	switch currentSortField {
	case sortByAssignee:
		sc = sortCursorAssignee
	case sortByAge:
		sc = sortCursorAge
	default:
		sc = sortCursorRepoIID
	}

	// --- Theme tab init ---
	themeCursor := 0
	for i, name := range themes {
		if name == currentThemeName {
			themeCursor = i
			break
		}
	}
	themeScrollOff := 0
	if themeCursor >= settingsPickerMaxVisible {
		themeScrollOff = themeCursor - settingsPickerMaxVisible + 1
	}
	modeCursor := 0
	for i, m := range modeOptions {
		if m == currentThemeMode {
			modeCursor = i
			break
		}
	}

	return settingsWidget{
		styles:             styles,
		keys:               keys,
		tab:                initialTab,
		includeReviewerMRs: includeReviewerMRs,
		filterStatus:       filterStatusWidget{phases: phaseState},
		filterView:         viewState,
		filterAssignee: filterSelectWidget{
			items: authorItems, excluded: assigneeExcluded, maxVisible: filterSelectMaxVisible,
		},
		filterReviewer: filterSelectWidget{
			items: reviewerItems, excluded: reviewerExcluded, maxVisible: filterSelectMaxVisible,
		},
		filterTicket: filterSelectWidget{
			items: ticketItems, excluded: ticketExcluded, noneExcluded: filter.ExcludeTicketless,
			maxVisible: filterSelectMaxVisible,
		},
		filterFocused:   filterFocusStatus,
		filterLastList:  filterFocusAssignee,
		sortCursor:      sc,
		sortField:       currentSortField,
		sortDesc:        currentSortDesc,
		themes:          themes,
		themeCursor:     themeCursor,
		themeScrollOff:  themeScrollOff,
		themeModeCursor: modeCursor,
		themeName:       currentThemeName,
		themeMode:       currentThemeMode,
	}
}

func excludedSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

// SetSize records the terminal size and derives how many rows each filter
// list column shows and how wide it is, so the panel scales with the
// terminal instead of hard-coding dimensions that can overflow a short one
// or truncate labels a wider terminal has room for.
func (w *settingsWidget) SetSize(width, height int) {
	w.width, w.height = width, height
	mv := clampVisible(height-filterFixedChromeLines, settingsMinVisible, filterSelectMaxVisibleCap)
	w.filterAssignee.maxVisible = mv
	w.filterReviewer.maxVisible = mv
	w.filterTicket.maxVisible = mv
	w.themeMaxVisible = clampVisible(height-settingsFrameChromeHeight, settingsMinVisible, settingsPickerMaxVisible)
	w.adjustThemeScroll()

	budget := width - settingsFrameChromeWidth - (filterNumColumns-1)*filterColumnDividerWidth
	colWidth := clampVisible(
		budget/filterNumColumns-filterMarkerPrefixWidth, filterColumnMinContentWidth, filterColumnMaxContentWidth,
	)
	w.filterColWidth = colWidth
	w.filterAssignee.colWidth = colWidth
	w.filterReviewer.colWidth = colWidth
	w.filterTicket.colWidth = colWidth
}

// clampVisible bounds n to [lo, hi].
func clampVisible(n, lo, hi int) int {
	switch {
	case n < lo:
		return lo
	case n > hi:
		return hi
	default:
		return n
	}
}

// buildSelectItems builds the item list for a filterSelectWidget (sorted
// entries), appending any value in excluded that isn't in usernames — see
// filterSelectItem.absent.
func buildSelectItems(usernames []string, userMap map[string]string, excluded map[string]bool) []filterSelectItem {
	items := make([]filterSelectItem, 0, len(usernames))
	seen := make(map[string]bool, len(usernames))
	for _, u := range usernames {
		seen[u] = true
		items = append(items, userSelectItem(u, userMap, false))
	}
	for _, v := range absentExcludedValues(excluded, seen) {
		items = append(items, userSelectItem(v, userMap, true))
	}
	return items
}

// userSelectItem builds one Assignee/Reviewer row. label is the display
// name alone when one is known, else the bare username; short is always
// the bare "@username". The n toggle (filterSelectWidget.compact) switches
// between the two — kept as two single-purpose strings, rather than one
// "Full Name (@username)" label, so each mode is a name a reader actually
// knows end to end instead of a compound string a narrow column truncates
// into something unrecognizable either way.
func userSelectItem(username string, userMap map[string]string, absent bool) filterSelectItem {
	label := username
	if name, ok := userMap[username]; ok && name != "" {
		label = name
	}
	return filterSelectItem{kind: filterItemValue, value: username, label: label, short: "@" + username, absent: absent}
}

// filterPseudoItemCount is the "No ID" entry every Issue ID list starts with.
const filterPseudoItemCount = 1

// buildTicketItems builds the Issue ID list: "No ID", then real keys in the
// order BuildTicketKeys already sorted them, then any excluded key no longer
// present in tickets — see filterSelectItem.absent.
func buildTicketItems(tickets []TicketKeyCount, noneCount int, excluded map[string]bool) []filterSelectItem {
	items := make([]filterSelectItem, 0, len(tickets)+filterPseudoItemCount)
	items = append(items, filterSelectItem{kind: filterItemNone, label: "No ID", short: "No ID", count: noneCount})
	seen := make(map[string]bool, len(tickets))
	for _, t := range tickets {
		seen[t.Key] = true
		items = append(items,
			filterSelectItem{kind: filterItemValue, value: t.Key, label: t.Key, short: t.Key, count: t.Count})
	}
	for _, v := range absentExcludedValues(excluded, seen) {
		items = append(items, filterSelectItem{kind: filterItemValue, value: v, label: v, short: v, absent: true})
	}
	return items
}

// absentExcludedValues returns, sorted, every key of excluded not present in
// seen — a persisted exclusion whose value no longer exists in the current
// MR set.
func absentExcludedValues(excluded, seen map[string]bool) []string {
	if len(excluded) == 0 {
		return nil
	}
	missing := make([]string, 0, len(excluded))
	for v := range excluded {
		if !seen[v] {
			missing = append(missing, v)
		}
	}
	sort.Strings(missing)
	return missing
}

// BuildAuthorsReviewers extracts sorted unique assignee and reviewer username slices from the MR list.
// Assignee falls back to author when unset so unassigned MRs remain filterable.
func BuildAuthorsReviewers(mrs []domain.MergeRequest) (assignees, reviewers []string) {
	assigneeSet := make(map[string]bool)
	reviewerSet := make(map[string]bool)
	for _, mr := range mrs {
		u := mr.Assignee
		if u == "" {
			u = mr.Author
		}
		if u != "" {
			assigneeSet[u] = true
		}
		for _, r := range mr.Reviewers {
			if r.Username != "" {
				reviewerSet[r.Username] = true
			}
		}
	}
	assignees = make([]string, 0, len(assigneeSet))
	for u := range assigneeSet {
		assignees = append(assignees, u)
	}
	sort.Strings(assignees)
	reviewers = make([]string, 0, len(reviewerSet))
	for u := range reviewerSet {
		reviewers = append(reviewers, u)
	}
	sort.Strings(reviewers)
	return assignees, reviewers
}

// BuildTicketKeys extracts the distinct issue IDs found in mrs' titles via
// matcher, each with how many MRs carry it, plus how many MRs have none.
// Keys are ordered by prefix ascending, then numeric suffix descending, so
// the newest ticket in a project sorts first without reshuffling as MRs
// churn (sorting by count instead would move rows out from under the cursor
// on every refresh).
func BuildTicketKeys(
	mrs []domain.MergeRequest, matcher domain.TicketKeyMatcher,
) (keys []TicketKeyCount, noneCount int) {
	counts := make(map[string]int)
	for _, mr := range mrs {
		key := matcher.ExtractFromTitle(mr.Title)
		if key == "" {
			noneCount++
			continue
		}
		counts[key]++
	}
	keys = make([]TicketKeyCount, 0, len(counts))
	for k, c := range counts {
		keys = append(keys, TicketKeyCount{Key: k, Count: c})
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, ni := splitTicketKey(keys[i].Key)
		pj, nj := splitTicketKey(keys[j].Key)
		if pi != pj {
			return pi < pj
		}
		return ni > nj
	})
	return keys, noneCount
}

// splitTicketKey splits a "PREFIX-NUMBER" issue ID into its prefix and
// numeric suffix for sorting. A key without that shape sorts by its whole
// string value, numeric part 0.
func splitTicketKey(key string) (prefix string, n int) {
	i := strings.LastIndex(key, "-")
	if i < 0 {
		return key, 0
	}
	num, err := strconv.Atoi(key[i+1:])
	if err != nil {
		return key, 0
	}
	return key[:i], num
}

// Init implements tea.Model.
func (w settingsWidget) Init() tea.Cmd { return nil }

// View implements tea.Model.
func (w settingsWidget) View() tea.View { return tea.NewView(w.render()) }

// Update implements tea.Model.
func (w settingsWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return w, nil
	}
	switch {
	case w.keys.Close.Match(kMsg):
		return w, func() tea.Msg { return SettingsClosedMsg{} }
	case w.keys.NextTab.Match(kMsg):
		w.tab = (w.tab + 1) % numSettingsTabs
	case w.keys.PrevTab.Match(kMsg):
		w.tab = (w.tab + numSettingsTabs - 1) % numSettingsTabs
	case w.keys.Up.Match(kMsg):
		w.moveVertical(-1)
		return w, w.emitApplied()
	case w.keys.Down.Match(kMsg):
		w.moveVertical(1)
		return w, w.emitApplied()
	case w.keys.Left.Match(kMsg):
		w.moveHorizontal(-1)
		return w, w.emitApplied()
	case w.keys.Right.Match(kMsg):
		w.moveHorizontal(1)
		return w, w.emitApplied()
	case w.keys.Toggle.Match(kMsg), w.keys.Confirm.Match(kMsg):
		w.activate()
		return w, w.emitApplied()
	case w.tab == tabFilters && w.isFilterListFocused() && w.keys.SelectAll.Match(kMsg):
		w.filterList(w.filterFocused).toggleSelectAll()
		return w, w.emitApplied()
	case w.tab == tabFilters && w.keys.Compact.Match(kMsg):
		w.toggleCompact()
	}
	return w, nil
}

// toggleCompact flips whether the Assignee/Reviewer columns show
// "Full Name (@username)" or the bare "@username" — a display preference,
// not part of the filter criteria, so it doesn't emit SettingsAppliedMsg.
func (w *settingsWidget) toggleCompact() {
	w.filterCompact = !w.filterCompact
	w.filterAssignee.compact = w.filterCompact
	w.filterReviewer.compact = w.filterCompact
	w.filterTicket.compact = w.filterCompact
}

// moveVertical handles Up/Down. On every tab but Filters it moves the cursor
// within the focused sub-section, unchanged from before. On Filters it also
// crosses between the Status strip and whichever list column was last
// focused — see moveVerticalFilters.
func (w *settingsWidget) moveVertical(delta int) {
	switch w.tab {
	case tabGeneral:
		// single item, nothing to move
	case tabFilters:
		w.moveVerticalFilters(delta)
	case tabSorting:
		var lo, hi int
		if w.sortSection == 0 {
			lo, hi = sortCursorRepoIID, sortCursorAge
		} else {
			lo, hi = sortCursorAscending, sortCursorDescending
		}
		next := w.sortCursor + delta
		if next >= lo && next <= hi {
			w.sortCursor = next
		}
	case tabTheme:
		w.moveCursorTheme(delta)
	}
}

// moveHorizontal handles Left/Right. On Sorting/Theme this moves between the
// tab's two side-by-side sub-sections, unchanged from before. On Filters, in
// the Status strip it moves the phase cursor; across the list columns it
// changes which column has focus — see moveHorizontalFilters.
func (w *settingsWidget) moveHorizontal(delta int) {
	switch w.tab {
	case tabFilters:
		w.moveHorizontalFilters(delta)
	case tabSorting:
		next := w.sortSection + delta
		if next >= 0 && next <= 1 {
			w.sortSection = next
			if w.sortSection == 0 && w.sortCursor > sortCursorAge {
				w.sortCursor = sortCursorAge
			} else if w.sortSection == 1 && w.sortCursor < sortCursorAscending {
				w.sortCursor = sortCursorAscending
			}
		}
	case tabTheme:
		next := w.themeSection + delta
		if next >= 0 && next <= 1 {
			w.themeSection = next
		}
	}
}

// moveVerticalFilters: within a list column, moves the row cursor; from a
// column's top row, steps up into the Status strip; from Status, steps back
// down into filterLastList at its remembered row.
func (w *settingsWidget) moveVerticalFilters(delta int) {
	switch w.filterFocused {
	case filterFocusStatus:
		if delta > 0 {
			w.filterFocused = w.nextFocusDown()
		}
		return
	case filterFocusView:
		if delta < 0 {
			w.filterFocused = filterFocusStatus
		} else {
			w.filterFocused = w.filterLastList
		}
		return
	}
	list := w.filterList(w.filterFocused)
	if delta < 0 && list.cursor == 0 {
		w.filterFocused = w.nextFocusUp()
		return
	}
	list.moveCursor(delta)
}

// nextFocusDown/nextFocusUp cross the View strip when it has a checkbox to
// land on, or skip straight past it (to the last list column / Status)
// when both its toggles are unavailable for this deployment.
func (w *settingsWidget) nextFocusDown() filterFocus {
	if len(w.filterView.items()) > 0 {
		return filterFocusView
	}
	return w.filterLastList
}

func (w *settingsWidget) nextFocusUp() filterFocus {
	if len(w.filterView.items()) > 0 {
		return filterFocusView
	}
	return filterFocusStatus
}

// moveHorizontalFilters: within the Status or View strip, moves that
// strip's own cursor; across the three list columns, changes which column
// has focus, carrying the row index across and clamping it to the target
// column's length.
func (w *settingsWidget) moveHorizontalFilters(delta int) {
	switch w.filterFocused {
	case filterFocusStatus:
		next := w.filterStatus.cursor + delta
		if next >= 0 && next < len(w.filterStatus.phases) {
			w.filterStatus.cursor = next
		}
		return
	case filterFocusView:
		next := w.filterView.cursor + delta
		if next >= 0 && next < len(w.filterView.items()) {
			w.filterView.cursor = next
		}
		return
	}
	next := w.filterFocused + filterFocus(delta)
	if next < filterFocusAssignee || next >= filterNumSections {
		return
	}
	row := w.filterList(w.filterFocused).cursor
	w.filterFocused = next
	w.filterLastList = next
	target := w.filterList(next)
	if row >= len(target.items) {
		row = len(target.items) - 1
	}
	if row < 0 {
		row = 0
	}
	target.cursor = row
	target.adjustScroll()
}

// isFilterListFocused reports whether one of the three Assignee/Reviewer/
// Issue ID list columns — as opposed to the Status or View strip — owns
// focus, i.e. whether filterList(w.filterFocused) is valid to call.
func (w settingsWidget) isFilterListFocused() bool {
	return w.filterFocused == filterFocusAssignee ||
		w.filterFocused == filterFocusReviewer ||
		w.filterFocused == filterFocusTicket
}

// filterList returns the list widget for a column focus value. Must not be
// called with filterFocusStatus or filterFocusView.
func (w *settingsWidget) filterList(f filterFocus) *filterSelectWidget {
	switch f {
	case filterFocusReviewer:
		return &w.filterReviewer
	case filterFocusTicket:
		return &w.filterTicket
	default:
		return &w.filterAssignee
	}
}

func (w *settingsWidget) moveCursorTheme(delta int) {
	if w.themeSection == 0 {
		next := w.themeCursor + delta
		if next >= 0 && next < len(w.themes) {
			w.themeCursor = next
			w.adjustThemeScroll()
			w.themeName = w.themes[w.themeCursor]
		}
	} else {
		next := w.themeModeCursor + delta
		if next >= 0 && next < len(modeOptions) {
			w.themeModeCursor = next
			w.themeMode = modeOptions[w.themeModeCursor]
		}
	}
}

func (w *settingsWidget) adjustThemeScroll() {
	mv := w.effectiveThemeMaxVisible()
	if w.themeCursor < w.themeScrollOff {
		w.themeScrollOff = w.themeCursor
	} else if w.themeCursor >= w.themeScrollOff+mv {
		w.themeScrollOff = w.themeCursor - mv + 1
	}
}

func (w settingsWidget) effectiveThemeMaxVisible() int {
	if w.themeMaxVisible > 0 {
		return w.themeMaxVisible
	}
	return settingsPickerMaxVisible
}

func (w *settingsWidget) activate() {
	switch w.tab {
	case tabGeneral:
		w.includeReviewerMRs = !w.includeReviewerMRs
	case tabFilters:
		switch w.filterFocused {
		case filterFocusStatus:
			w.filterStatus.toggle()
		case filterFocusView:
			w.filterView.toggle()
		default:
			w.filterList(w.filterFocused).toggle()
		}
	case tabSorting:
		switch w.sortCursor {
		case sortCursorRepoIID:
			w.sortField = sortByRepoIID
		case sortCursorAssignee:
			w.sortField = sortByAssignee
		case sortCursorAge:
			w.sortField = sortByAge
		case sortCursorAscending:
			w.sortDesc = false
		case sortCursorDescending:
			w.sortDesc = true
		}
	case tabTheme:
		if w.themeSection == 0 && w.themeCursor < len(w.themes) {
			w.themeName = w.themes[w.themeCursor]
		} else if w.themeSection == 1 && w.themeModeCursor < len(modeOptions) {
			w.themeMode = modeOptions[w.themeModeCursor]
		}
	}
}

func (w settingsWidget) emitApplied() tea.Cmd {
	return func() tea.Msg {
		return w.buildApplied()
	}
}

func (w settingsWidget) buildApplied() SettingsAppliedMsg {
	allTrue := w.filterStatus.phases[0] && w.filterStatus.phases[1] &&
		w.filterStatus.phases[2] && w.filterStatus.phases[3]
	var phaseMap map[domain.MRPhase]bool
	if !allTrue {
		phaseMap = make(map[domain.MRPhase]bool, len(w.filterStatus.phases))
		for i, shown := range w.filterStatus.phases {
			phaseMap[domain.MRPhase(i)] = shown
		}
	}
	return SettingsAppliedMsg{
		Filter: domain.FilterCriteria{
			Phases:             phaseMap,
			ExcludedAssignees:  w.filterAssignee.excludedSlice(),
			ExcludedReviewers:  w.filterReviewer.excludedSlice(),
			ExcludedTicketKeys: w.filterTicket.excludedSlice(),
			ExcludeTicketless:  w.filterTicket.noneExcluded,
		},
		ViewMine:           w.filterView.myMRsOnly,
		SprintFilter:       w.filterView.sprint,
		IncludeReviewerMRs: w.includeReviewerMRs,
		SortField:          w.sortField.stateKey(),
		SortDesc:           w.sortDesc,
		ThemeName:          w.themeName,
		ThemeMode:          w.themeMode,
	}
}

// --- rendering ---

func (w settingsWidget) render() string {
	var body string
	switch w.tab {
	case tabGeneral:
		body = w.renderGeneral()
	case tabFilters:
		body = w.renderFilters()
	case tabSorting:
		body = w.renderSorting()
	case tabTheme:
		body = w.renderTheme()
	}
	// Every tab's body is placed into the same canvas so the border — and the
	// header above it — lands on the same screen cell on every tab, instead
	// of the popup resizing (and its position jumping) on each tab switch.
	cw, ch := w.canvasSize()
	body = lip.Place(cw, ch, lip.Left, lip.Top, body)

	var sb strings.Builder
	sb.WriteString(w.renderTabBar() + "\n\n")
	sb.WriteString(body)
	sb.WriteString("\n" + w.styles.PopupHint.Render(settingsHintText))
	return w.styles.PopupBorder.Render(sb.String())
}

// canvasSize is the fixed content width/height every tab body is placed
// into — the max natural size across all four tabs' bodies (plus the tab
// bar and hint line, which share the same border). It is not capped to the
// terminal: lip.Place pads content narrower than the target size but never
// crops content wider than it, so a cap here couldn't actually shrink the
// widest tab (typically Filters, whose column width already has its own
// floor from SetSize) — it would only make the other tabs inconsistent
// with it again, reintroducing the very jump this method exists to remove.
// Each tab already keeps itself within the terminal via SetSize (Filters'
// and Theme's row counts, and Filters' column width, are all height/width
// budget of the terminal), so this rarely needs to exceed it in practice.
func (w settingsWidget) canvasSize() (width, height int) {
	for _, body := range []string{w.renderGeneral(), w.renderFilters(), w.renderSorting(), w.renderTheme()} {
		if bw := lip.Width(body); bw > width {
			width = bw
		}
		if bh := lip.Height(body); bh > height {
			height = bh
		}
	}
	for _, line := range []string{w.renderTabBar(), settingsHintText} {
		if lw := lip.Width(line); lw > width {
			width = lw
		}
	}
	return width, height
}

func (w settingsWidget) renderTabBar() string {
	parts := make([]string, numSettingsTabs)
	for i, label := range settingsTabLabels {
		if settingsTab(i) == w.tab {
			parts[i] = w.styles.PopupSectionFocused.Render("▶ " + label)
		} else {
			parts[i] = w.styles.PopupItem.Render("  " + label)
		}
	}
	return lip.JoinHorizontal(lip.Top, parts...)
}

func (w settingsWidget) renderGeneral() string {
	var sb strings.Builder
	sb.WriteString(w.styles.PopupSection.Render("  General") + "\n\n")
	marker := markerUnchecked
	if w.includeReviewerMRs {
		marker = markerChecked
	}
	markerStyled := w.styles.PopupItemMarkerOn.Render(marker)
	if !w.includeReviewerMRs {
		markerStyled = w.styles.PopupItemMarkerOff.Render(marker)
	}
	sb.WriteString("  " + markerStyled + " " + w.styles.PopupItemFocused.Render("Include reviewer MRs") + "\n")
	return sb.String()
}

// filterNumColumns is the three side-by-side list columns on the Filters tab
// (Assignee/Reviewer/Issue ID); filterColumnDividerWidth is " │ ".
const (
	filterNumColumns         = 3
	filterColumnDividerWidth = 3
	// filterColumnChromeLines is a rendered column's non-row lines: the
	// header and the trailing scroll-indicator/blank line.
	filterColumnChromeLines = 2
)

// renderFilters lays out Status as a one-line horizontal strip, then
// Assignee/Reviewer/Issue ID as three side-by-side columns — the same
// direction the columns are navigated in, see moveHorizontalFilters.
func (w settingsWidget) renderFilters() string {
	var sb strings.Builder
	sb.WriteString(renderSectionHeader("Status", w.filterFocused == filterFocusStatus, w.styles) + "  " +
		w.filterStatus.render(w.filterFocused == filterFocusStatus, w.styles) + "\n")
	sb.WriteString(w.renderFilterView() + "\n")
	colWidth := w.filterAssignee.effectiveColWidth()
	ruleWidth := filterNumColumns*filterColumnTotalWidth(colWidth) + (filterNumColumns-1)*filterColumnDividerWidth
	sb.WriteString(w.styles.PopupDivider.Render(strings.Repeat("─", ruleWidth)) + "\n")

	assigneeCol := w.renderFilterColumn("Assignee", filterFocusAssignee, w.filterAssignee)
	reviewerCol := w.renderFilterColumn("Reviewer", filterFocusReviewer, w.filterReviewer)
	ticketCol := w.renderFilterColumn("Issue ID", filterFocusTicket, w.filterTicket)

	rows := filterColumnChromeLines + w.filterAssignee.effectiveMaxVisible()
	divLines := make([]string, rows)
	for i := range divLines {
		divLines[i] = w.styles.PopupDivider.Render(" │ ")
	}
	divider := strings.Join(divLines, "\n")

	sb.WriteString(lip.JoinHorizontal(lip.Top, assigneeCol, divider, reviewerCol, divider, ticketCol))
	return sb.String()
}

// renderFilterView renders the "My MRs only" / "Current sprint" row —
// blank when neither toggle applies to this deployment (see
// filterViewWidget), so the Filters tab's line count stays the same either
// way for a given config, rather than depending on which checkboxes exist.
func (w settingsWidget) renderFilterView() string {
	if len(w.filterView.items()) == 0 {
		return ""
	}
	return renderSectionHeader("View", w.filterFocused == filterFocusView, w.styles) + "    " +
		w.filterView.render(w.filterFocused == filterFocusView, w.styles)
}

// renderFilterColumn renders one Filters-tab column: a header (with an
// active-selection-count badge when the dimension is restricting anything)
// over the list body.
func (w settingsWidget) renderFilterColumn(title string, focus filterFocus, list filterSelectWidget) string {
	focused := w.filterFocused == focus
	headerLabel := title
	if n := list.activeCount(); n > 0 {
		headerLabel = fmt.Sprintf("%s (%d)", title, n)
	}
	colWidth := filterColumnTotalWidth(list.effectiveColWidth())
	header := padDisplay(renderSectionHeader(headerLabel, focused, w.styles), colWidth)
	return header + "\n" + list.render(focused, w.styles)
}

func (w settingsWidget) renderSorting() string {
	fieldFocused := w.sortSection == 0
	dirFocused := w.sortSection == 1

	fieldLines := []string{padDisplay(renderSectionHeader("Sort field", fieldFocused, w.styles), sortColumnWidth)}
	sortFieldItems := []struct {
		label string
		field sortField
	}{
		{"repo·id", sortByRepoIID},
		{"assignee", sortByAssignee},
		{"age", sortByAge},
	}
	for i, item := range sortFieldItems {
		selected := w.sortField == item.field
		focused := fieldFocused && w.sortCursor == i
		fieldLines = append(fieldLines, padDisplay(renderRadioItem(item.label, selected, focused, w.styles), sortColumnWidth))
	}

	dirLines := []string{padDisplay(renderSectionHeader("Direction", dirFocused, w.styles), sortColumnWidth)}
	dirItems := []struct {
		label string
		desc  bool
		idx   int
	}{
		{"↑ ascending", false, sortCursorAscending},
		{"↓ descending", true, sortCursorDescending},
	}
	for _, item := range dirItems {
		selected := w.sortDesc == item.desc
		focused := dirFocused && w.sortCursor == item.idx
		dirLines = append(dirLines, padDisplay(renderRadioItem(item.label, selected, focused, w.styles), sortColumnWidth))
	}

	rows := max(len(fieldLines), len(dirLines))
	blank := strings.Repeat(" ", sortColumnWidth)
	for len(fieldLines) < rows {
		fieldLines = append(fieldLines, blank)
	}
	for len(dirLines) < rows {
		dirLines = append(dirLines, blank)
	}
	divLines := make([]string, rows)
	for i := range divLines {
		divLines[i] = w.styles.PopupDivider.Render(" │ ")
	}
	divider := strings.Join(divLines, "\n")

	return lip.JoinHorizontal(lip.Top, strings.Join(fieldLines, "\n"), divider, strings.Join(dirLines, "\n"))
}

func renderRadioItem(label string, selected, focused bool, styles Styles) string {
	radio := "○"
	if selected {
		radio = "●"
	}
	raw := fmt.Sprintf("  %s %s", radio, label)
	if focused {
		return styles.PopupItemFocused.Render(raw)
	}
	return styles.PopupItem.Render(raw)
}

func (w settingsWidget) renderTheme() string {
	mv := w.effectiveThemeMaxVisible()

	// Left pane: theme list
	end := w.themeScrollOff + mv
	if end > len(w.themes) {
		end = len(w.themes)
	}
	visible := w.themes[w.themeScrollOff:end]

	var listLines []string
	for i, name := range visible {
		idx := w.themeScrollOff + i
		selected := idx == w.themeCursor
		focused := w.themeSection == 0 && selected
		prefix := "  "
		if selected {
			prefix = "▶ "
		}
		padWidth := settingsPickerListWidth - len([]rune(prefix))
		raw := fmt.Sprintf("%s%-*s", prefix, padWidth, name)
		var line string
		if focused {
			line = w.styles.PopupItemFocused.Render(raw)
		} else {
			line = w.styles.PopupItem.Render(raw)
		}
		listLines = append(listLines, line)
	}
	emptyRaw := fmt.Sprintf("%-*s", settingsPickerListWidth, "")
	for len(listLines) < mv {
		listLines = append(listLines, w.styles.PopupItem.Render(emptyRaw))
	}
	listPane := strings.Join(listLines, "\n")

	// Divider
	var divLines []string
	for range mv {
		divLines = append(divLines, w.styles.PopupDivider.Render("│"))
	}
	divider := strings.Join(divLines, "\n")

	// Right pane: mode radio
	var modeLines []string
	for i, m := range modeOptions {
		selected := i == w.themeModeCursor
		focused := w.themeSection == 1 && selected
		radio := "○"
		if selected {
			radio = "●"
		}
		padWidth := settingsModeWidth - pickerRadioPrefixLen
		raw := fmt.Sprintf("%s %-*s", radio, padWidth, m)
		var line string
		if focused {
			line = w.styles.PopupItemFocused.Render(raw)
		} else {
			line = w.styles.PopupItem.Render(raw)
		}
		modeLines = append(modeLines, line)
	}
	emptyModeRaw := fmt.Sprintf("%-*s", settingsModeWidth, "")
	for len(modeLines) < mv {
		modeLines = append(modeLines, w.styles.PopupItem.Render(emptyModeRaw))
	}
	modePane := strings.Join(modeLines, "\n")

	return lip.JoinHorizontal(lip.Top, listPane, divider, modePane)
}
