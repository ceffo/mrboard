package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// FilterAppliedMsg is sent on every toggle to immediately update the board filter.
type FilterAppliedMsg struct {
	Criteria FilterCriteria
}

// FilterClosedMsg is sent when the user closes the filter popup (f or Esc).
type FilterClosedMsg struct{}

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

// lookupName returns the full name for key from m, or key itself if not found.
func lookupName(m map[string]string, key string) string {
	if n, ok := m[key]; ok {
		return n
	}
	return key
}

// --- filterStatusWidget ---

// filterStatusWidget manages the Status (phase) section checkboxes.
type filterStatusWidget struct {
	phases [4]bool
	cursor int
}

func (s *filterStatusWidget) moveCursor(delta int) {
	next := s.cursor + delta
	if next >= 0 && next < len(s.phases) {
		s.cursor = next
	}
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

// --- filterSelectWidget ---

// filterSelectItem is a single entry in a multi-select list (Author or Reviewer).
type filterSelectItem struct {
	value string // "" means "All"
	label string
}

// filterSelectWidget manages a scrollable multi-select list.
type filterSelectWidget struct {
	items       []filterSelectItem
	checked     map[string]bool // nil/empty = all shown (no filter)
	cursor      int
	scrollOff   int
	currentUser string // when set, auto-pins this user when selecting others
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

// --- filterPopupWidget ---

type filterFocus int

const (
	filterFocusStatus filterFocus = iota
	filterFocusAuthor
	filterFocusReviewer
	filterNumSections
)

// filterPopupWidget is the modal filter popup with three independently navigable sections.
// Tab/Shift+Tab switches focus between sections; Up/Down and Space operate within the focused section.
type filterPopupWidget struct {
	styles   Styles
	keys     FilterPopupKeyMap
	focus    filterFocus
	status   filterStatusWidget
	author   filterSelectWidget
	reviewer filterSelectWidget
}

// newFilterPopupWidget builds a popup pre-populated with the current filter state.
// authors and reviewers are sorted unique usernames.
// userMap maps username → display name for both authors and reviewers.
// currentUser is pinned at the top of the author list.
func newFilterPopupWidget(
	styles Styles,
	keys FilterPopupKeyMap,
	authors []string,
	reviewers []string,
	userMap map[string]string,
	current FilterCriteria,
	currentUser string,
) filterPopupWidget {
	// Status section.
	var arr [4]bool
	if len(current.Phases) == 0 {
		arr = [4]bool{true, true, true, true}
	} else {
		for i := range arr {
			arr[i] = current.Phases[domain.MRPhase(i)]
		}
	}

	// Author section: current user pinned first.
	orderedAuthors := make([]string, 0, len(authors))
	if currentUser != "" {
		orderedAuthors = append(orderedAuthors, currentUser)
		for _, a := range authors {
			if a != currentUser {
				orderedAuthors = append(orderedAuthors, a)
			}
		}
	} else {
		orderedAuthors = append(orderedAuthors, authors...)
	}

	authorItems := make([]filterSelectItem, 0, 1+len(orderedAuthors))
	authorItems = append(authorItems, filterSelectItem{value: "", label: "All authors"})
	for _, a := range orderedAuthors {
		lbl := lookupName(userMap, a)
		if a == currentUser && currentUser != "" {
			lbl += " (you)"
		}
		authorItems = append(authorItems, filterSelectItem{value: a, label: lbl})
	}

	var authorChecked map[string]bool
	if len(current.Authors) > 0 {
		authorChecked = make(map[string]bool, len(current.Authors))
		for _, a := range current.Authors {
			authorChecked[a] = true
		}
	}

	// Reviewer section.
	reviewerItems := make([]filterSelectItem, 0, 1+len(reviewers))
	reviewerItems = append(reviewerItems, filterSelectItem{value: "", label: "All reviewers"})
	for _, r := range reviewers {
		reviewerItems = append(reviewerItems, filterSelectItem{value: r, label: lookupName(userMap, r)})
	}

	var reviewerChecked map[string]bool
	if len(current.Reviewers) > 0 {
		reviewerChecked = make(map[string]bool, len(current.Reviewers))
		for _, r := range current.Reviewers {
			reviewerChecked[r] = true
		}
	}

	return filterPopupWidget{
		styles: styles,
		keys:   keys,
		focus:  filterFocusStatus,
		status: filterStatusWidget{phases: arr},
		author: filterSelectWidget{
			items:       authorItems,
			checked:     authorChecked,
			currentUser: currentUser,
		},
		reviewer: filterSelectWidget{
			items:   reviewerItems,
			checked: reviewerChecked,
		},
	}
}

func (p filterPopupWidget) Init() tea.Cmd { return nil }

func (p filterPopupWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	switch {
	case key.Matches(kMsg, p.keys.FocusNext):
		p.focus = (p.focus + 1) % filterNumSections
	case key.Matches(kMsg, p.keys.FocusPrev):
		p.focus = (p.focus + filterNumSections - 1) % filterNumSections
	case key.Matches(kMsg, p.keys.Up):
		p.moveCursor(-1)
	case key.Matches(kMsg, p.keys.Down):
		p.moveCursor(1)
	case key.Matches(kMsg, p.keys.Toggle):
		p.toggleFocused()
		return p, func() tea.Msg { return p.buildApplied() }
	case key.Matches(kMsg, p.keys.Close):
		return p, func() tea.Msg { return FilterClosedMsg{} }
	}
	return p, nil
}

func (p *filterPopupWidget) moveCursor(delta int) {
	switch p.focus {
	case filterFocusStatus:
		p.status.moveCursor(delta)
	case filterFocusAuthor:
		p.author.moveCursor(delta)
	case filterFocusReviewer:
		p.reviewer.moveCursor(delta)
	}
}

func (p *filterPopupWidget) toggleFocused() {
	switch p.focus {
	case filterFocusStatus:
		p.status.toggle()
	case filterFocusAuthor:
		p.author.toggle()
	case filterFocusReviewer:
		p.reviewer.toggle()
	}
}

func (p filterPopupWidget) buildApplied() FilterAppliedMsg {
	allTrue := p.status.phases[0] && p.status.phases[1] && p.status.phases[2] && p.status.phases[3]
	var phaseMap map[domain.MRPhase]bool
	if !allTrue {
		phaseMap = make(map[domain.MRPhase]bool, len(p.status.phases))
		for i, shown := range p.status.phases {
			phaseMap[domain.MRPhase(i)] = shown
		}
	}
	return FilterAppliedMsg{
		Criteria: FilterCriteria{
			Phases:    phaseMap,
			Authors:   p.author.selectedSlice(),
			Reviewers: p.reviewer.selectedSlice(),
		},
	}
}

func (p filterPopupWidget) render() string {
	var sb strings.Builder
	sb.WriteString(p.styles.PopupTitle.Render("Filter") + "\n\n")

	sb.WriteString(renderSectionHeader("Status", p.focus == filterFocusStatus, p.styles) + "\n")
	sb.WriteString(p.status.render(p.focus == filterFocusStatus, p.styles))

	sb.WriteString("\n")
	sb.WriteString(renderSectionHeader("Author", p.focus == filterFocusAuthor, p.styles) + "\n")
	sb.WriteString(p.author.render(p.focus == filterFocusAuthor, p.styles))

	sb.WriteString("\n")
	sb.WriteString(renderSectionHeader("Reviewer", p.focus == filterFocusReviewer, p.styles) + "\n")
	sb.WriteString(p.reviewer.render(p.focus == filterFocusReviewer, p.styles))

	sb.WriteString("\n" + p.styles.PopupHint.Render("  ↑/↓ move  tab section  space toggle  f/esc close"))
	return p.styles.PopupBorder.Render(sb.String())
}

func renderSectionHeader(title string, focused bool, styles Styles) string {
	if focused {
		return styles.PopupSectionFocused.Render("▶ " + title)
	}
	return styles.PopupSection.Render("  " + title)
}

func (p filterPopupWidget) View() tea.View {
	return tea.NewView(p.render())
}

// uniqueAuthorsReviewers extracts sorted unique author usernames and reviewer usernames from mrs.
func uniqueAuthorsReviewers(mrs []domain.MergeRequest) (authors, reviewers []string) {
	authorSet := make(map[string]struct{})
	reviewerSet := make(map[string]struct{})
	for _, mr := range mrs {
		if mr.Author != "" {
			authorSet[mr.Author] = struct{}{}
		}
		for _, r := range mr.Reviewers {
			if r.Username != "" {
				reviewerSet[r.Username] = struct{}{}
			}
		}
	}
	for a := range authorSet {
		authors = append(authors, a)
	}
	for r := range reviewerSet {
		reviewers = append(reviewers, r)
	}
	sort.Strings(authors)
	sort.Strings(reviewers)
	return
}
