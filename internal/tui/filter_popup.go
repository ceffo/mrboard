package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// FilterAppliedMsg is sent when the user confirms the filter popup.
type FilterAppliedMsg struct {
	Phases   map[domain.MRPhase]bool
	Author   string
	Reviewer string
}

// FilterCancelledMsg is sent when the user closes the popup without applying.
type FilterCancelledMsg struct{}

type filterItemKind int

const (
	filterItemPhase filterItemKind = iota
	filterItemAuthor
	filterItemReviewer
)

type filterItem struct {
	kind  filterItemKind
	phase domain.MRPhase
	label string
	value string // author or reviewer username; "" = "all"
}

// filterPopupWidget is the modal filter popup. It is self-contained: pressing
// Enter emits FilterAppliedMsg and pressing Esc emits FilterCancelledMsg.
type filterPopupWidget struct {
	styles   Styles
	keys     FilterPopupKeyMap
	items    []filterItem
	cursor   int
	phases   [4]bool // indexed by domain.MRPhase; true = phase is shown
	author   string  // "" = all authors
	reviewer string  // "" = all reviewers
}

const (
	phaseLabelDraft    = "Draft"
	phaseLabelReview   = "Needs Review"
	phaseLabelAuthorAc = "Needs Author Action"
	phaseLabelReady    = "Ready to Merge"
)

var phaseLabels = [4]string{phaseLabelDraft, phaseLabelReview, phaseLabelAuthorAc, phaseLabelReady}

// newFilterPopupWidget builds a popup pre-populated with the current filter state.
// authors and reviewers are the distinct values available in the current MR list.
func newFilterPopupWidget(
	styles Styles,
	keys FilterPopupKeyMap,
	authors []string,
	reviewers []string,
	phases map[domain.MRPhase]bool,
	author string,
	reviewer string,
) filterPopupWidget {
	items := make([]filterItem, 0, 4+1+len(authors)+1+len(reviewers))

	for i := range phaseLabels {
		items = append(items, filterItem{
			kind:  filterItemPhase,
			phase: domain.MRPhase(i),
			label: phaseLabels[i],
		})
	}
	items = append(items, filterItem{kind: filterItemAuthor, label: "All authors", value: ""})
	for _, a := range authors {
		items = append(items, filterItem{kind: filterItemAuthor, label: "@" + a, value: a})
	}
	items = append(items, filterItem{kind: filterItemReviewer, label: "All reviewers", value: ""})
	for _, r := range reviewers {
		items = append(items, filterItem{kind: filterItemReviewer, label: "@" + r, value: r})
	}

	// Convert map → array; nil/empty map means all phases shown.
	var arr [4]bool
	if len(phases) == 0 {
		arr = [4]bool{true, true, true, true}
	} else {
		for i := range arr {
			arr[i] = phases[domain.MRPhase(i)]
		}
	}

	return filterPopupWidget{
		styles:   styles,
		keys:     keys,
		items:    items,
		phases:   arr,
		author:   author,
		reviewer: reviewer,
	}
}

func (p filterPopupWidget) Init() tea.Cmd { return nil }

func (p filterPopupWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}

	switch {
	case key.Matches(kMsg, p.keys.Up):
		if p.cursor > 0 {
			p.cursor--
		}
	case key.Matches(kMsg, p.keys.Down):
		if p.cursor < len(p.items)-1 {
			p.cursor++
		}
	case key.Matches(kMsg, p.keys.Toggle):
		p = p.toggle()
	case key.Matches(kMsg, p.keys.Apply):
		applied := p.buildApplied()
		return p, func() tea.Msg { return applied }
	case key.Matches(kMsg, p.keys.Cancel):
		return p, func() tea.Msg { return FilterCancelledMsg{} }
	}
	return p, nil
}

func (p filterPopupWidget) toggle() filterPopupWidget {
	if p.cursor >= len(p.items) {
		return p
	}
	item := p.items[p.cursor]
	switch item.kind {
	case filterItemPhase:
		idx := int(item.phase)
		if idx >= 0 && idx < len(p.phases) {
			p.phases[idx] = !p.phases[idx]
		}
	case filterItemAuthor:
		p.author = item.value
	case filterItemReviewer:
		p.reviewer = item.value
	}
	return p
}

func (p filterPopupWidget) buildApplied() FilterAppliedMsg {
	allTrue := p.phases[0] && p.phases[1] && p.phases[2] && p.phases[3]
	var phaseMap map[domain.MRPhase]bool
	if !allTrue {
		phaseMap = make(map[domain.MRPhase]bool, len(p.phases))
		for i, shown := range p.phases {
			phaseMap[domain.MRPhase(i)] = shown
		}
	}
	return FilterAppliedMsg{Phases: phaseMap, Author: p.author, Reviewer: p.reviewer}
}

func (p filterPopupWidget) render() string {
	var sb strings.Builder

	sb.WriteString(p.styles.PopupTitle.Render("Filter") + "\n\n")

	var lastKind filterItemKind = -1
	for i, item := range p.items {
		if item.kind != lastKind {
			if lastKind != -1 {
				sb.WriteString("\n")
			}
			switch item.kind {
			case filterItemPhase:
				sb.WriteString(p.styles.PopupSection.Render("Phase") + "\n")
			case filterItemAuthor:
				sb.WriteString(p.styles.PopupSection.Render("Author") + "\n")
			case filterItemReviewer:
				sb.WriteString(p.styles.PopupSection.Render("Reviewer") + "\n")
			}
			lastKind = item.kind
		}

		marker := p.markerFor(item)
		line := fmt.Sprintf("  %s %s", marker, item.label)
		if i == p.cursor {
			line = p.styles.PopupItemFocused.Render(line)
		} else {
			line = p.styles.PopupItem.Render(line)
		}
		sb.WriteString(line + "\n")
	}

	sb.WriteString("\n" + p.styles.PopupHint.Render("  ↑/↓ move  space toggle  ↵ apply  esc cancel"))

	return p.styles.PopupBorder.Render(sb.String())
}

func (p filterPopupWidget) markerFor(item filterItem) string {
	switch item.kind {
	case filterItemPhase:
		idx := int(item.phase)
		if idx >= 0 && idx < len(p.phases) && p.phases[idx] {
			return "[x]"
		}
		return "[ ]"
	case filterItemAuthor:
		if p.author == item.value {
			return "(•)"
		}
		return "( )"
	case filterItemReviewer:
		if p.reviewer == item.value {
			return "(•)"
		}
		return "( )"
	}
	return "   "
}

func (p filterPopupWidget) View() tea.View {
	return tea.NewView(p.render())
}

// uniqueAuthorsReviewers extracts sorted unique author and reviewer usernames from mrs.
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
