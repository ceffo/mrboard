package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
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

	filterSelectMaxVisible = 8
)

var phaseLabels = [4]string{phaseLabelDraft, phaseLabelReview, phaseLabelAuthorAc, phaseLabelReady}

// --- Theme mode helpers ---

var modeOptions = []string{themeModeAuto, themeModeDark, themeModeLight}

const pickerRadioPrefixLen = 2 // "● " or "○ "

// filterFocus identifies which section of the Filters tab owns keyboard focus.
type filterFocus int

const (
	filterFocusStatus filterFocus = iota
	filterFocusAuthor
	filterFocusReviewer
	filterNumSections
)

// filterStatusWidget manages the Status (phase) section checkboxes.
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
	var sb strings.Builder
	for i, lbl := range phaseLabels {
		var markerStyled string
		if s.phases[i] {
			markerStyled = styles.PopupItemMarkerOn.Render(markerChecked)
		} else {
			markerStyled = styles.PopupItemMarkerOff.Render(markerUnchecked)
		}
		if focused && i == s.cursor {
			sb.WriteString("  " + markerStyled + " " + styles.PopupItemFocused.Render(lbl) + "\n")
		} else {
			sb.WriteString("  " + markerStyled + " " + styles.PopupItem.Render(lbl) + "\n")
		}
	}
	return sb.String()
}

// filterSelectItem is a single entry in a multi-select list (Author or Reviewer).
type filterSelectItem struct {
	value string // "" means "All"
	label string
}

// filterSelectWidget manages a scrollable multi-select list.
type filterSelectWidget struct {
	items     []filterSelectItem
	checked   map[string]bool // nil/empty = all shown (no filter)
	cursor    int
	scrollOff int
}

func (s *filterSelectWidget) moveCursor(delta int) {
	next := s.cursor + delta
	if next >= 0 && next < len(s.items) {
		s.cursor = next
		s.adjustScroll()
	}
}

func (s *filterSelectWidget) adjustScroll() {
	if s.cursor < s.scrollOff {
		s.scrollOff = s.cursor
	} else if s.cursor >= s.scrollOff+filterSelectMaxVisible {
		s.scrollOff = s.cursor - filterSelectMaxVisible + 1
	}
}

func (s *filterSelectWidget) toggle() {
	if s.cursor >= len(s.items) {
		return
	}
	item := s.items[s.cursor]
	if item.value == "" {
		s.checked = nil
		return
	}
	if s.checked == nil {
		s.checked = make(map[string]bool)
	}
	if s.checked[item.value] {
		delete(s.checked, item.value)
		if len(s.checked) == 0 {
			s.checked = nil
		}
	} else {
		s.checked[item.value] = true
	}
}

func (s filterSelectWidget) selectedSlice() []string {
	result := make([]string, 0, len(s.checked))
	for v := range s.checked {
		result = append(result, v)
	}
	sort.Strings(result)
	return result
}

func (s filterSelectWidget) render(focused bool, styles Styles) string {
	var sb strings.Builder
	end := min(s.scrollOff+filterSelectMaxVisible, len(s.items))
	for i := s.scrollOff; i < end; i++ {
		item := s.items[i]
		var checked bool
		if item.value == "" {
			checked = len(s.checked) == 0
		} else {
			checked = s.checked[item.value]
		}
		var markerStyled string
		if checked {
			markerStyled = styles.PopupItemMarkerOn.Render(markerChecked)
		} else {
			markerStyled = styles.PopupItemMarkerOff.Render(markerUnchecked)
		}
		if focused && i == s.cursor {
			sb.WriteString("  " + markerStyled + " " + styles.PopupItemFocused.Render(item.label) + "\n")
		} else {
			sb.WriteString("  " + markerStyled + " " + styles.PopupItem.Render(item.label) + "\n")
		}
	}
	if len(s.items) > filterSelectMaxVisible {
		shown := min(s.scrollOff+filterSelectMaxVisible, len(s.items))
		sb.WriteString(styles.PopupHint.Render(fmt.Sprintf("  %d–%d / %d", s.scrollOff+1, shown, len(s.items))) + "\n")
	}
	return sb.String()
}

func renderSectionHeader(title string, focused bool, styles Styles) string {
	if focused {
		return styles.PopupSectionFocused.Render("▶ " + title)
	}
	return styles.PopupSection.Render("  " + title)
}

// --- SettingsAppliedMsg / SettingsClosedMsg ---

// SettingsAppliedMsg is emitted on every live change in the settings panel.
type SettingsAppliedMsg struct {
	Filter             domain.FilterCriteria
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
	sortCursorAuthor     = 1
	sortCursorAge        = 2
	sortCursorAscending  = 3
	sortCursorDescending = 4
	sortNumCursors       = 5
)

const (
	settingsPickerMaxVisible = 10
	settingsPickerListWidth  = 22
	settingsModeWidth        = 10
)

// settingsWidget is a 4-tab settings panel: General / Filters / Sorting / Theme.
type settingsWidget struct {
	styles Styles
	keys   SettingsKeyMap
	tab    settingsTab

	// General tab
	includeReviewerMRs bool

	// Filters tab
	filterStatus   filterStatusWidget
	filterAuthor   filterSelectWidget
	filterReviewer filterSelectWidget
	filterFocused  filterFocus

	// Sorting tab
	sortCursor int // 0–4
	sortField  sortField
	sortDesc   bool

	// Theme tab
	themes          []string
	themeCursor     int
	themeScrollOff  int
	themeModeCursor int
	themeSection    int // 0 = list, 1 = mode

	// current persisted theme values (updated on each live change)
	themeName string
	themeMode string
}

// newSettingsWidget constructs a settingsWidget populated from current app state.
// authors and reviewers are sorted username slices used to populate the Filters tab.
func newSettingsWidget(
	themes []string,
	authors, reviewers []string,
	userMap map[string]string,
	filter domain.FilterCriteria,
	includeReviewerMRs bool,
	currentSortField sortField,
	currentSortDesc bool,
	currentThemeName, currentThemeMode string,
	styles Styles,
	keys SettingsKeyMap,
) settingsWidget {
	// --- Filters tab init ---
	phaseState := [4]bool{true, true, true, true}
	if len(filter.Phases) > 0 {
		phaseState = [4]bool{}
		for i := range phaseState {
			phaseState[i] = filter.Phases[domain.MRPhase(i)]
		}
	}
	authorItems := buildSelectItems(authors, userMap)
	authorChecked := make(map[string]bool, len(filter.Authors))
	for _, a := range filter.Authors {
		authorChecked[a] = true
	}
	if len(authorChecked) == 0 {
		authorChecked = nil
	}
	reviewerItems := buildSelectItems(reviewers, userMap)
	reviewerChecked := make(map[string]bool, len(filter.Reviewers))
	for _, r := range filter.Reviewers {
		reviewerChecked[r] = true
	}
	if len(reviewerChecked) == 0 {
		reviewerChecked = nil
	}

	// --- Sorting tab init ---
	var sc int
	switch currentSortField {
	case sortByAuthor:
		sc = sortCursorAuthor
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
		tab:                tabGeneral,
		includeReviewerMRs: includeReviewerMRs,
		filterStatus:       filterStatusWidget{phases: phaseState},
		filterAuthor:       filterSelectWidget{items: authorItems, checked: authorChecked},
		filterReviewer:     filterSelectWidget{items: reviewerItems, checked: reviewerChecked},
		filterFocused:      filterFocusStatus,
		sortCursor:         sc,
		sortField:          currentSortField,
		sortDesc:           currentSortDesc,
		themes:             themes,
		themeCursor:        themeCursor,
		themeScrollOff:     themeScrollOff,
		themeModeCursor:    modeCursor,
		themeName:          currentThemeName,
		themeMode:          currentThemeMode,
	}
}

// buildSelectItems builds the item list for a filterSelectWidget ("All" + sorted entries).
func buildSelectItems(usernames []string, userMap map[string]string) []filterSelectItem {
	items := make([]filterSelectItem, 0, len(usernames)+1)
	items = append(items, filterSelectItem{value: "", label: "All"})
	for _, u := range usernames {
		label := u
		if name, ok := userMap[u]; ok && name != "" {
			label = name + " (@" + u + ")"
		}
		items = append(items, filterSelectItem{value: u, label: label})
	}
	return items
}

// BuildAuthorsReviewers extracts sorted unique author and reviewer username slices from the MR list.
func BuildAuthorsReviewers(mrs []domain.MergeRequest) (authors, reviewers []string) {
	authorSet := make(map[string]bool)
	reviewerSet := make(map[string]bool)
	for _, mr := range mrs {
		if mr.Author != "" {
			authorSet[mr.Author] = true
		}
		for _, r := range mr.Reviewers {
			if r.Username != "" {
				reviewerSet[r.Username] = true
			}
		}
	}
	authors = make([]string, 0, len(authorSet))
	for u := range authorSet {
		authors = append(authors, u)
	}
	sort.Strings(authors)
	reviewers = make([]string, 0, len(reviewerSet))
	for u := range reviewerSet {
		reviewers = append(reviewers, u)
	}
	sort.Strings(reviewers)
	return authors, reviewers
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
	case key.Matches(kMsg, w.keys.Close):
		return w, func() tea.Msg { return SettingsClosedMsg{} }
	case key.Matches(kMsg, w.keys.NextTab):
		w.tab = (w.tab + 1) % numSettingsTabs
	case key.Matches(kMsg, w.keys.PrevTab):
		w.tab = (w.tab + numSettingsTabs - 1) % numSettingsTabs
	case key.Matches(kMsg, w.keys.Up):
		w.moveCursor(-1)
		return w, w.emitApplied()
	case key.Matches(kMsg, w.keys.Down):
		w.moveCursor(1)
		return w, w.emitApplied()
	case key.Matches(kMsg, w.keys.Toggle), key.Matches(kMsg, w.keys.Confirm):
		w.activate()
		return w, w.emitApplied()
	}
	return w, nil
}

func (w *settingsWidget) moveCursor(delta int) {
	switch w.tab {
	case tabGeneral:
		// single item, nothing to move
	case tabFilters:
		w.moveCursorFilters(delta)
	case tabSorting:
		next := w.sortCursor + delta
		if next >= 0 && next < sortNumCursors {
			w.sortCursor = next
		}
	case tabTheme:
		w.moveCursorTheme(delta)
	}
}

func (w *settingsWidget) moveCursorFilters(delta int) {
	switch w.filterFocused {
	case filterFocusStatus:
		next := w.filterStatus.cursor + delta
		if next < 0 {
			w.filterFocused = filterFocusReviewer
			w.filterReviewer.cursor = len(w.filterReviewer.items) - 1
			w.filterReviewer.adjustScroll()
		} else if next >= len(w.filterStatus.phases) {
			w.filterFocused = filterFocusAuthor
			w.filterAuthor.cursor = 0
			w.filterAuthor.scrollOff = 0
		} else {
			w.filterStatus.cursor = next
		}
	case filterFocusAuthor:
		next := w.filterAuthor.cursor + delta
		if next < 0 {
			w.filterFocused = filterFocusStatus
			w.filterStatus.cursor = len(w.filterStatus.phases) - 1
		} else if next >= len(w.filterAuthor.items) {
			w.filterFocused = filterFocusReviewer
			w.filterReviewer.cursor = 0
			w.filterReviewer.scrollOff = 0
		} else {
			w.filterAuthor.moveCursor(delta)
		}
	case filterFocusReviewer:
		next := w.filterReviewer.cursor + delta
		if next < 0 {
			w.filterFocused = filterFocusAuthor
			w.filterAuthor.cursor = len(w.filterAuthor.items) - 1
			w.filterAuthor.adjustScroll()
		} else if next >= len(w.filterReviewer.items) {
			w.filterFocused = filterFocusStatus
			w.filterStatus.cursor = 0
		} else {
			w.filterReviewer.moveCursor(delta)
		}
	}
}

func (w *settingsWidget) moveCursorTheme(delta int) {
	if w.themeSection == 0 {
		next := w.themeCursor + delta
		if next < 0 {
			w.themeSection = 1
			w.themeModeCursor = len(modeOptions) - 1
		} else if next >= len(w.themes) {
			w.themeSection = 1
			w.themeModeCursor = 0
		} else {
			w.themeCursor = next
			w.adjustThemeScroll()
			w.themeName = w.themes[w.themeCursor]
		}
	} else {
		next := w.themeModeCursor + delta
		if next < 0 {
			w.themeSection = 0
			w.themeCursor = len(w.themes) - 1
			w.adjustThemeScroll()
		} else if next >= len(modeOptions) {
			w.themeSection = 0
			w.themeCursor = 0
			w.themeScrollOff = 0
		} else {
			w.themeModeCursor = next
			w.themeMode = modeOptions[w.themeModeCursor]
		}
	}
}

func (w *settingsWidget) adjustThemeScroll() {
	if w.themeCursor < w.themeScrollOff {
		w.themeScrollOff = w.themeCursor
	} else if w.themeCursor >= w.themeScrollOff+settingsPickerMaxVisible {
		w.themeScrollOff = w.themeCursor - settingsPickerMaxVisible + 1
	}
}

func (w *settingsWidget) activate() {
	switch w.tab {
	case tabGeneral:
		w.includeReviewerMRs = !w.includeReviewerMRs
	case tabFilters:
		switch w.filterFocused {
		case filterFocusStatus:
			w.filterStatus.toggle()
		case filterFocusAuthor:
			w.filterAuthor.toggle()
		case filterFocusReviewer:
			w.filterReviewer.toggle()
		}
	case tabSorting:
		switch w.sortCursor {
		case sortCursorRepoIID:
			w.sortField = sortByRepoIID
		case sortCursorAuthor:
			w.sortField = sortByAuthor
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
			Phases:    phaseMap,
			Authors:   w.filterAuthor.selectedSlice(),
			Reviewers: w.filterReviewer.selectedSlice(),
		},
		IncludeReviewerMRs: w.includeReviewerMRs,
		SortField:          w.sortField.stateKey(),
		SortDesc:           w.sortDesc,
		ThemeName:          w.themeName,
		ThemeMode:          w.themeMode,
	}
}

// --- rendering ---

func (w settingsWidget) render() string {
	var sb strings.Builder
	sb.WriteString(w.renderTabBar() + "\n\n")
	switch w.tab {
	case tabGeneral:
		sb.WriteString(w.renderGeneral())
	case tabFilters:
		sb.WriteString(w.renderFilters())
	case tabSorting:
		sb.WriteString(w.renderSorting())
	case tabTheme:
		sb.WriteString(w.renderTheme())
	}
	sb.WriteString("\n" + w.styles.PopupHint.Render("  tab next-tab  ↑/↓ move  space toggle  ,/esc close"))
	return w.styles.PopupBorder.Render(sb.String())
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

func (w settingsWidget) renderFilters() string {
	var sb strings.Builder
	sb.WriteString(renderSectionHeader("Status", w.filterFocused == filterFocusStatus, w.styles) + "\n")
	sb.WriteString(w.filterStatus.render(w.filterFocused == filterFocusStatus, w.styles))
	sb.WriteString("\n")
	sb.WriteString(renderSectionHeader("Author", w.filterFocused == filterFocusAuthor, w.styles) + "\n")
	sb.WriteString(w.filterAuthor.render(w.filterFocused == filterFocusAuthor, w.styles))
	sb.WriteString("\n")
	sb.WriteString(renderSectionHeader("Reviewer", w.filterFocused == filterFocusReviewer, w.styles) + "\n")
	sb.WriteString(w.filterReviewer.render(w.filterFocused == filterFocusReviewer, w.styles))
	return sb.String()
}

func (w settingsWidget) renderSorting() string {
	var sb strings.Builder
	sb.WriteString(w.styles.PopupSection.Render("  Sort field") + "\n")
	sortFieldItems := []struct {
		label string
		field sortField
	}{
		{"repo·id", sortByRepoIID},
		{"author", sortByAuthor},
		{"age", sortByAge},
	}
	for i, item := range sortFieldItems {
		selected := w.sortField == item.field
		focused := w.sortCursor == i
		sb.WriteString(renderRadioItem(item.label, selected, focused, w.styles))
	}
	sb.WriteString("\n")
	sb.WriteString(w.styles.PopupSection.Render("  Direction") + "\n")
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
		focused := w.sortCursor == item.idx
		sb.WriteString(renderRadioItem(item.label, selected, focused, w.styles))
	}
	return sb.String()
}

func renderRadioItem(label string, selected, focused bool, styles Styles) string {
	radio := "○"
	if selected {
		radio = "●"
	}
	raw := fmt.Sprintf("  %s %s", radio, label)
	if focused {
		return styles.PopupItemFocused.Render(raw) + "\n"
	}
	return styles.PopupItem.Render(raw) + "\n"
}

func (w settingsWidget) renderTheme() string {
	// Left pane: theme list
	end := w.themeScrollOff + settingsPickerMaxVisible
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
	for len(listLines) < settingsPickerMaxVisible {
		listLines = append(listLines, w.styles.PopupItem.Render(emptyRaw))
	}
	listPane := strings.Join(listLines, "\n")

	// Divider
	var divLines []string
	for range settingsPickerMaxVisible {
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
	for len(modeLines) < settingsPickerMaxVisible {
		modeLines = append(modeLines, w.styles.PopupItem.Render(emptyModeRaw))
	}
	modePane := strings.Join(modeLines, "\n")

	return lip.JoinHorizontal(lip.Top, listPane, divider, modePane)
}
