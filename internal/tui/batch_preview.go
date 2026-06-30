package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// BatchPreviewBackMsg is sent when the user presses Esc in the batch preview screen,
// requesting a return to the batch reviewer editor.
type BatchPreviewBackMsg struct{}

// BatchPreviewConfirmedMsg is sent when the user presses Enter in the batch preview screen.
// Targets contains only the sibling MRs that are both included and have a detected change.
// The dispatch logic is wired in mrr-qtk.4.
type BatchPreviewConfirmedMsg struct {
	Staged  []stagedReviewer
	Targets []domain.MergeRequest
}

const batchPreviewMaxVisible = 8

// previewMRRow is one row in the batch preview list.
type previewMRRow struct {
	mr        domain.MergeRequest
	included  bool // user-toggled: will this MR be part of the write?
	hasChange bool // staged reviewers differ from current MR reviewers
}

// batchPreviewWidget is the preview overlay shown after the batch reviewer editor.
// It displays each sibling MR with a selectable checkbox and a change indicator,
// letting the user confirm or exclude individual MRs before the write is dispatched.
type batchPreviewWidget struct {
	styles    Styles
	keys      BatchPreviewKeyMap
	staged    []stagedReviewer
	rows      []previewMRRow
	cursor    int
	scrollOff int
}

// newBatchPreviewWidget builds the preview widget from the staged reviewer list
// and the sibling MR slice. Change detection runs at construction time.
func newBatchPreviewWidget(
	staged []stagedReviewer,
	siblings []domain.MergeRequest,
	styles Styles,
	keys BatchPreviewKeyMap,
) *batchPreviewWidget {
	rows := make([]previewMRRow, len(siblings))
	for i, sib := range siblings {
		rows[i] = previewMRRow{
			mr:        sib,
			included:  true,
			hasChange: stagedDiffersFromMR(staged, sib),
		}
	}
	return &batchPreviewWidget{
		styles: styles,
		keys:   keys,
		staged: staged,
		rows:   rows,
	}
}

// stagedDiffersFromMR returns true when the staged reviewer list differs from
// the MR's current reviewer list in username membership or approver flags.
func stagedDiffersFromMR(staged []stagedReviewer, mr domain.MergeRequest) bool {
	current := make(map[string]bool, len(mr.Reviewers))
	for _, r := range mr.Reviewers {
		current[r.Username] = r.IsApprover
	}
	if len(staged) != len(current) {
		return true
	}
	for _, s := range staged {
		approver, ok := current[s.Username]
		if !ok || approver != s.IsApprover {
			return true
		}
	}
	return false
}

func (w *batchPreviewWidget) Init() tea.Cmd { return nil }

func (w *batchPreviewWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return w, nil
	}
	switch {
	case key.Matches(kMsg, w.keys.Back):
		return w, func() tea.Msg { return BatchPreviewBackMsg{} }

	case key.Matches(kMsg, w.keys.Up):
		if w.cursor > 0 {
			w.cursor--
			w.adjustScroll()
		}

	case key.Matches(kMsg, w.keys.Down):
		if w.cursor < len(w.rows)-1 {
			w.cursor++
			w.adjustScroll()
		}

	case key.Matches(kMsg, w.keys.Toggle):
		if w.cursor < len(w.rows) {
			w.rows[w.cursor].included = !w.rows[w.cursor].included
		}

	case key.Matches(kMsg, w.keys.Confirm):
		targets := w.collectTargets()
		staged := make([]stagedReviewer, len(w.staged))
		copy(staged, w.staged)
		return w, func() tea.Msg {
			return BatchPreviewConfirmedMsg{Staged: staged, Targets: targets}
		}
	}
	return w, nil
}

func (w *batchPreviewWidget) adjustScroll() {
	if w.cursor < w.scrollOff {
		w.scrollOff = w.cursor
	} else if w.cursor >= w.scrollOff+batchPreviewMaxVisible {
		w.scrollOff = w.cursor - batchPreviewMaxVisible + 1
	}
}

// collectTargets returns rows that are included and have a detected change.
func (w *batchPreviewWidget) collectTargets() []domain.MergeRequest {
	out := make([]domain.MergeRequest, 0, len(w.rows))
	for _, row := range w.rows {
		if row.included && row.hasChange {
			out = append(out, row.mr)
		}
	}
	return out
}

func (w *batchPreviewWidget) render() string {
	var sb strings.Builder

	const hintLine = "  ↑/↓ nav  space:include  ↵:apply  esc:back"

	// Count how many rows will actually be written.
	targets := w.collectTargets()
	title := fmt.Sprintf("Preview Changes (%d to write)", len(targets))
	sb.WriteString(w.styles.PopupTitle.Render(title) + "\n\n")

	if len(w.rows) == 0 {
		sb.WriteString(w.styles.PopupHint.Render("  (no sibling MRs)") + "\n")
	} else {
		end := min(w.scrollOff+batchPreviewMaxVisible, len(w.rows))
		for i := w.scrollOff; i < end; i++ {
			row := w.rows[i]
			repo := row.mr.ProjectPath
			if idx := strings.LastIndex(repo, "/"); idx >= 0 {
				repo = repo[idx+1:]
			}
			label := fmt.Sprintf("!%d %s — %s", row.mr.IID, repo, row.mr.Title)

			var markerStyled string
			if row.included {
				markerStyled = w.styles.PopupItemMarkerOn.Render(markerChecked)
			} else {
				markerStyled = w.styles.PopupItemMarkerOff.Render(markerUnchecked)
			}

			var changePart string
			if row.hasChange {
				changePart = w.styles.PopupItemMarkerOn.Render(" ✎")
			} else {
				changePart = w.styles.PopupHint.Render(" ─")
			}

			focused := i == w.cursor
			var labelStyled string
			if focused {
				labelStyled = w.styles.PopupItemFocused.Render(label)
			} else {
				labelStyled = w.styles.PopupItem.Render(label)
			}
			sb.WriteString("  " + markerStyled + " " + labelStyled + changePart + "\n")
		}
		if len(w.rows) > batchPreviewMaxVisible {
			shown := min(w.scrollOff+batchPreviewMaxVisible, len(w.rows))
			sb.WriteString(w.styles.PopupHint.Render(
				fmt.Sprintf("  %d–%d / %d", w.scrollOff+1, shown, len(w.rows))) + "\n")
		}
	}

	sb.WriteString("\n")

	// Legend line.
	sb.WriteString(w.styles.PopupHint.Render("  ✎ will change  ─ no change") + "\n\n")
	sb.WriteString(w.styles.PopupHint.Render(hintLine))

	return w.styles.PopupBorder.Render(sb.String())
}

func (w *batchPreviewWidget) View() tea.View {
	return tea.NewView(w.render())
}
