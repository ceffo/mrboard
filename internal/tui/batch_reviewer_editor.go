package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// BatchReviewerEditorClosedMsg is sent when the batch reviewer editor is dismissed.
type BatchReviewerEditorClosedMsg struct{}

const batchEditorMaxVisible = 8

// batchReviewerEditorPanel identifies which panel owns keyboard focus.
type batchReviewerEditorPanel int

const (
	batchEditorPanelReviewers batchReviewerEditorPanel = iota
	batchEditorPanelSiblings
)

// batchReviewerEditorWidget is the modal overlay for batch-editing reviewer lists
// across all MRs that share the same JIRA issue key as the focused card.
// The reviewer list is pre-filled from the focused card; sibling MRs are listed
// for context. Save dispatch is wired in a later task.
type batchReviewerEditorWidget struct {
	styles    Styles
	keys      BatchReviewerEditorKeyMap
	focusedMR domain.MergeRequest
	siblings  []domain.MergeRequest // all MRs sharing the JIRA key (includes focusedMR)
	jiraKey   string                // extracted from focusedMR.Title; "" if none

	// Reviewer staging — pre-filled from focusedMR.Reviewers, excluding the author.
	staged    []stagedReviewer
	cursor    int
	scrollOff int

	// Panel focus: reviewers list vs sibling MR list.
	panel batchReviewerEditorPanel

	// Sibling MR list navigation.
	sibCursor    int
	sibScrollOff int
}

// newBatchReviewerEditorWidget creates a batch editor pre-filled from the focused MR.
// siblings should be the full list of MRs sharing the same JIRA key (may include focusedMR).
func newBatchReviewerEditorWidget(
	focusedMR domain.MergeRequest,
	siblings []domain.MergeRequest,
	styles Styles,
	keys BatchReviewerEditorKeyMap,
) *batchReviewerEditorWidget {
	staged := make([]stagedReviewer, 0, len(focusedMR.Reviewers))
	for _, r := range focusedMR.Reviewers {
		if r.Username == focusedMR.Author {
			continue
		}
		staged = append(staged, stagedReviewer{
			Username:   r.Username,
			Name:       r.Name,
			State:      r.State,
			IsApprover: r.IsApprover,
		})
	}
	return &batchReviewerEditorWidget{
		styles:    styles,
		keys:      keys,
		focusedMR: focusedMR,
		siblings:  siblings,
		jiraKey:   domain.ExtractJiraID(focusedMR.Title),
		staged:    staged,
	}
}

func (w *batchReviewerEditorWidget) Init() tea.Cmd { return nil }

func (w *batchReviewerEditorWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return w, nil
	}
	switch {
	case key.Matches(kMsg, w.keys.Close):
		return w, func() tea.Msg { return BatchReviewerEditorClosedMsg{} }

	case key.Matches(kMsg, w.keys.Tab):
		if w.panel == batchEditorPanelReviewers {
			w.panel = batchEditorPanelSiblings
		} else {
			w.panel = batchEditorPanelReviewers
		}

	case key.Matches(kMsg, w.keys.Up):
		if w.panel == batchEditorPanelReviewers {
			if w.cursor > 0 {
				w.cursor--
				w.adjustScrollReviewers()
			}
		} else {
			if w.sibCursor > 0 {
				w.sibCursor--
				w.adjustScrollSiblings()
			}
		}

	case key.Matches(kMsg, w.keys.Down):
		if w.panel == batchEditorPanelReviewers {
			if w.cursor < len(w.staged)-1 {
				w.cursor++
				w.adjustScrollReviewers()
			}
		} else {
			if w.sibCursor < len(w.siblings)-1 {
				w.sibCursor++
				w.adjustScrollSiblings()
			}
		}

	case key.Matches(kMsg, w.keys.ToggleApprover):
		if w.panel == batchEditorPanelReviewers && w.cursor < len(w.staged) {
			w.staged[w.cursor].IsApprover = !w.staged[w.cursor].IsApprover
		}

	case key.Matches(kMsg, w.keys.Remove):
		if w.panel == batchEditorPanelReviewers && w.cursor < len(w.staged) {
			w.staged = append(w.staged[:w.cursor], w.staged[w.cursor+1:]...)
			if w.cursor > 0 && w.cursor >= len(w.staged) {
				w.cursor = len(w.staged) - 1
			}
			w.adjustScrollReviewers()
		}
	}
	return w, nil
}

func (w *batchReviewerEditorWidget) adjustScrollReviewers() {
	if w.cursor < w.scrollOff {
		w.scrollOff = w.cursor
	} else if w.cursor >= w.scrollOff+batchEditorMaxVisible {
		w.scrollOff = w.cursor - batchEditorMaxVisible + 1
	}
}

func (w *batchReviewerEditorWidget) adjustScrollSiblings() {
	if w.sibCursor < w.sibScrollOff {
		w.sibScrollOff = w.sibCursor
	} else if w.sibCursor >= w.sibScrollOff+batchEditorMaxVisible {
		w.sibScrollOff = w.sibCursor - batchEditorMaxVisible + 1
	}
}

func (w *batchReviewerEditorWidget) render() string {
	var sb strings.Builder

	const hintLine = "  tab:switch  ↑/↓ nav  space:approver  d:remove  esc:cancel"

	// Title line: MR ref left, widget title right.
	repoName := w.focusedMR.ProjectPath
	if i := strings.LastIndex(repoName, "/"); i >= 0 {
		repoName = repoName[i+1:]
	}
	leftStr := fmt.Sprintf("!%d %s", w.focusedMR.IID, repoName)
	rightStr := "Batch Edit Reviewers"
	contentW := len([]rune(hintLine))
	gap := contentW - len([]rune(leftStr)) - len([]rune(rightStr))
	if gap < 1 {
		gap = 1
	}
	line1 := w.styles.PopupHint.Render(leftStr) + strings.Repeat(" ", gap) + w.styles.PopupTitle.Render(rightStr)
	sb.WriteString(line1 + "\n")
	sb.WriteString(w.styles.PopupItem.Render(w.focusedMR.Title) + "\n")
	if w.jiraKey != "" {
		sb.WriteString(w.styles.PopupHint.Render("🎫 "+w.jiraKey) + "\n")
	}
	sb.WriteString("\n")

	w.renderReviewersSection(&sb)
	sb.WriteString("\n")
	w.renderSiblingsSection(&sb)
	sb.WriteString("\n")
	sb.WriteString(w.styles.PopupHint.Render(hintLine))

	return w.styles.PopupBorder.Render(sb.String())
}

func (w *batchReviewerEditorWidget) renderReviewersSection(sb *strings.Builder) {
	if w.panel == batchEditorPanelReviewers {
		sb.WriteString(w.styles.PopupSectionFocused.Render("▶ Reviewers") + "\n")
	} else {
		sb.WriteString(w.styles.PopupSection.Render("  Reviewers") + "\n")
	}

	if len(w.staged) == 0 {
		sb.WriteString(w.styles.PopupHint.Render("  (no reviewers)") + "\n")
		return
	}

	end := min(w.scrollOff+batchEditorMaxVisible, len(w.staged))
	for i := w.scrollOff; i < end; i++ {
		s := w.staged[i]
		var markerStyled string
		if s.IsApprover {
			markerStyled = w.styles.PopupItemMarkerOn.Render(markerChecked)
		} else {
			markerStyled = w.styles.PopupItemMarkerOff.Render(markerUnchecked)
		}
		name := s.Name
		if name == "" {
			name = s.Username
		}
		label := name + " " + reviewerIcon(s.State)
		focused := w.panel == batchEditorPanelReviewers && i == w.cursor
		if focused {
			sb.WriteString("  " + markerStyled + " " + w.styles.PopupItemFocused.Render(label) + "\n")
		} else {
			sb.WriteString("  " + markerStyled + " " + w.styles.PopupItem.Render(label) + "\n")
		}
	}
	if len(w.staged) > batchEditorMaxVisible {
		shown := min(w.scrollOff+batchEditorMaxVisible, len(w.staged))
		sb.WriteString(w.styles.PopupHint.Render(
			fmt.Sprintf("  %d–%d / %d", w.scrollOff+1, shown, len(w.staged))) + "\n")
	}
}

func (w *batchReviewerEditorWidget) renderSiblingsSection(sb *strings.Builder) {
	sibCount := len(w.siblings)
	plural := "s"
	if sibCount == 1 {
		plural = ""
	}
	header := fmt.Sprintf("Apply to (%d MR%s)", sibCount, plural)
	if w.panel == batchEditorPanelSiblings {
		sb.WriteString(w.styles.PopupSectionFocused.Render("▶ "+header) + "\n")
	} else {
		sb.WriteString(w.styles.PopupSection.Render("  "+header) + "\n")
	}

	if sibCount == 0 {
		sb.WriteString(w.styles.PopupHint.Render("  (no sibling MRs)") + "\n")
		return
	}

	end := min(w.sibScrollOff+batchEditorMaxVisible, sibCount)
	for i := w.sibScrollOff; i < end; i++ {
		sib := w.siblings[i]
		repo := sib.ProjectPath
		if idx := strings.LastIndex(repo, "/"); idx >= 0 {
			repo = repo[idx+1:]
		}
		suffix := ""
		if sib.ProjectID == w.focusedMR.ProjectID && sib.IID == w.focusedMR.IID {
			suffix = " (this)"
		}
		label := fmt.Sprintf("!%d %s — %s%s", sib.IID, repo, sib.Title, suffix)
		focused := w.panel == batchEditorPanelSiblings && i == w.sibCursor
		if focused {
			sb.WriteString("  " + w.styles.PopupItemFocused.Render(label) + "\n")
		} else {
			sb.WriteString("  " + w.styles.PopupItem.Render(label) + "\n")
		}
	}
	if sibCount > batchEditorMaxVisible {
		shown := min(w.sibScrollOff+batchEditorMaxVisible, sibCount)
		sb.WriteString(w.styles.PopupHint.Render(
			fmt.Sprintf("  %d–%d / %d", w.sibScrollOff+1, shown, sibCount)) + "\n")
	}
}

func (w *batchReviewerEditorWidget) View() tea.View {
	return tea.NewView(w.render())
}
