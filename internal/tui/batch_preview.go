package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// BatchPreviewBackMsg is sent when the user presses Esc in the batch preview screen,
// requesting a return to the batch reviewer editor.
type BatchPreviewBackMsg struct{}

// BatchPreviewConfirmedMsg is sent when the user presses Enter in the batch preview screen.
// Targets contains focusedMR (when its own staged edit is a real change) plus whichever
// sibling MRs are both included and have a detected change.
type BatchPreviewConfirmedMsg struct {
	Staged   []stagedReviewer
	Targets  []domain.MergeRequest
	KnownIDs map[string]int64 // see BatchReviewerEditorPreviewMsg.KnownIDs
}

const batchPreviewMaxVisible = 8

// previewMRRow is one row in the batch preview list. The focused MR (see
// batchPreviewWidget) is always rows[0]: shown so the user can see what will
// happen to it too, but not selectable — its write is unconditional, not an
// "also apply to" — see focused and collectTargets.
type previewMRRow struct {
	mr        domain.MergeRequest
	included  bool // will this MR be part of the write? Always true, and un-toggleable, when focused.
	hasChange bool // staged reviewers differ from current MR reviewers
	conflict  bool // this MR's current approvers differ from focusedMR's (warning only; never true when focused)
	focused   bool // this is the MR the reviewer editor was opened on — not a selectable "also apply to"
}

// batchPreviewWidget is the preview overlay shown after the batch reviewer editor.
// It displays the focused MR (always applied, not selectable) followed by each
// sibling MR with a selectable checkbox and a change indicator, letting the user
// confirm or exclude individual siblings before the write is dispatched.
type batchPreviewWidget struct {
	styles    Styles
	keys      BatchPreviewKeyMap
	staged    []stagedReviewer
	knownIDs  map[string]int64
	rows      []previewMRRow
	cursor    int
	scrollOff int
}

// newBatchPreviewWidget builds the preview widget from the staged reviewer list,
// the focused MR (becomes rows[0]), and the sibling MR slice (focusedMR excluded —
// see BatchReviewerEditorPreviewMsg.Siblings). Change detection and conflict
// detection both run at construction time. knownIDs is carried through unchanged
// to BatchPreviewConfirmedMsg — see its doc comment.
func newBatchPreviewWidget(
	staged []stagedReviewer,
	siblings []domain.MergeRequest,
	focusedMR domain.MergeRequest,
	knownIDs map[string]int64,
	styles Styles,
	keys BatchPreviewKeyMap,
) *batchPreviewWidget {
	rows := make([]previewMRRow, 0, len(siblings)+1)
	rows = append(rows, previewMRRow{
		mr:        focusedMR,
		included:  true,
		hasChange: stagedDiffersFromMR(staged, focusedMR),
		focused:   true,
	})
	for _, sib := range siblings {
		rows = append(rows, previewMRRow{
			mr:        sib,
			included:  true,
			hasChange: stagedDiffersFromMR(staged, sib),
			conflict:  domain.ApproversConflict(focusedMR, sib),
		})
	}
	return &batchPreviewWidget{
		styles:   styles,
		keys:     keys,
		staged:   staged,
		knownIDs: knownIDs,
		rows:     rows,
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

// reviewerWriteDiff reports how applying staged to mr would change its reviewer
// and approver membership: reviewersAdded/reviewersRemoved are reviewer username
// additions/removals; approversAdded/approversRemoved are IsApprover-flag
// transitions for usernames that remain reviewers on both sides. Mirrors the two
// writes ApplyReviewerChanges performs (SetReviewers, SaveApprovers).
//
// A username never appears in both a reviewers list and the corresponding
// approvers list: an approver is a reviewer, so listing it as a plain reviewer
// addition/removal alongside its approver one would just be the same person
// twice under two labels — the approver entry is the more specific one and wins.
func reviewerWriteDiff(
	staged []stagedReviewer, mr domain.MergeRequest,
) (reviewersAdded, reviewersRemoved, approversAdded, approversRemoved []string) {
	type reviewerState struct {
		isApprover bool
	}
	current := make(map[string]reviewerState, len(mr.Reviewers))
	for _, r := range mr.Reviewers {
		current[r.Username] = reviewerState{isApprover: r.IsApprover}
	}
	stagedSet := make(map[string]bool, len(staged))
	approversAddedSet := make(map[string]bool)
	approversRemovedSet := make(map[string]bool)
	for _, s := range staged {
		stagedSet[s.Username] = true
		cur, existed := current[s.Username]
		if !existed {
			reviewersAdded = append(reviewersAdded, s.Username)
			if s.IsApprover {
				approversAdded = append(approversAdded, s.Username)
				approversAddedSet[s.Username] = true
			}
			continue
		}
		if s.IsApprover && !cur.isApprover {
			approversAdded = append(approversAdded, s.Username)
			approversAddedSet[s.Username] = true
		} else if !s.IsApprover && cur.isApprover {
			approversRemoved = append(approversRemoved, s.Username)
			approversRemovedSet[s.Username] = true
		}
	}
	for _, r := range mr.Reviewers {
		if stagedSet[r.Username] {
			continue
		}
		reviewersRemoved = append(reviewersRemoved, r.Username)
		if r.IsApprover {
			approversRemoved = append(approversRemoved, r.Username)
			approversRemovedSet[r.Username] = true
		}
	}
	reviewersAdded = dropUsernames(reviewersAdded, approversAddedSet)
	reviewersRemoved = dropUsernames(reviewersRemoved, approversRemovedSet)
	sort.Strings(reviewersAdded)
	sort.Strings(reviewersRemoved)
	sort.Strings(approversAdded)
	sort.Strings(approversRemoved)
	return reviewersAdded, reviewersRemoved, approversAdded, approversRemoved
}

// dropUsernames filters usernames in place, removing any present in drop.
func dropUsernames(usernames []string, drop map[string]bool) []string {
	if len(drop) == 0 {
		return usernames
	}
	out := usernames[:0]
	for _, u := range usernames {
		if !drop[u] {
			out = append(out, u)
		}
	}
	return out
}

func (w *batchPreviewWidget) Init() tea.Cmd { return nil }

func (w *batchPreviewWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return w, nil
	}
	switch {
	case w.keys.Back.Match(kMsg):
		return w, func() tea.Msg { return BatchPreviewBackMsg{} }

	case w.keys.Up.Match(kMsg):
		if w.cursor > 0 {
			w.cursor--
			w.adjustScroll()
		}

	case w.keys.Down.Match(kMsg):
		if w.cursor < len(w.rows)-1 {
			w.cursor++
			w.adjustScroll()
		}

	case w.keys.Toggle.Match(kMsg):
		if w.cursor < len(w.rows) && !w.rows[w.cursor].focused {
			w.rows[w.cursor].included = !w.rows[w.cursor].included
		}

	case w.keys.Confirm.Match(kMsg):
		targets := w.collectTargets()
		staged := make([]stagedReviewer, len(w.staged))
		copy(staged, w.staged)
		knownIDs := make(map[string]int64, len(w.knownIDs))
		for k, v := range w.knownIDs {
			knownIDs[k] = v
		}
		return w, func() tea.Msg {
			return BatchPreviewConfirmedMsg{Staged: staged, Targets: targets, KnownIDs: knownIDs}
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

// collectTargets returns every row that is included and has a detected
// change — the focused row (rows[0]) is always included, so its own edit is
// covered here too whenever it's a real change.
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
			suffix := ""
			switch {
			case row.focused:
				suffix = " (this)"
			case row.conflict:
				suffix = " " + w.styles.DurationWarning.Render("⚠")
			}
			label := fmt.Sprintf("%s !%d %s — %s%s", phaseIcon(row.mr.Phase), row.mr.IID, repo, row.mr.Title, suffix)

			var markerStyled string
			switch {
			case row.focused:
				markerStyled = w.styles.PopupHint.Render(markerFixed) // always applied, not a toggle
			case row.included:
				markerStyled = w.styles.PopupItemMarkerOn.Render(markerChecked)
			default:
				markerStyled = w.styles.PopupItemMarkerOff.Render(markerUnchecked)
			}

			underCursor := i == w.cursor
			var labelStyled string
			if underCursor {
				labelStyled = w.styles.PopupItemFocused.Render(label)
			} else {
				labelStyled = w.styles.PopupItem.Render(label)
			}

			line := "  " + markerStyled + " " + labelStyled
			switch {
			case !row.hasChange:
				line += " " + w.styles.PopupHint.Render("─")
			case row.included:
				if diff := w.renderRowInlineDiff(row.mr); diff != "" {
					line += " " + diff
				}
			}
			sb.WriteString(line + "\n")
		}
		if len(w.rows) > batchPreviewMaxVisible {
			shown := min(w.scrollOff+batchPreviewMaxVisible, len(w.rows))
			sb.WriteString(w.styles.PopupHint.Render(
				fmt.Sprintf("  %d–%d / %d", w.scrollOff+1, shown, len(w.rows))) + "\n")
		}
		w.renderRowDetails(&sb)
	}

	sb.WriteString("\n")

	// Legend line.
	sb.WriteString(w.styles.PopupHint.Render("  ─ no change") + "\n\n")
	sb.WriteString(w.styles.PopupHint.Render(hintLine))

	return w.styles.PopupBorder.Render(sb.String())
}

// renderRowInlineDiff returns the same-line diff of what applying staged would
// actually do to mr — added/removed reviewers and approvers merged into one
// "-@user +@user" form (see renderInlineDiff), or "" when there's nothing to
// show. Deliberately drops reviewerWriteDiff's (reviewer)/(approver)
// qualifier for compactness — a username never appears in both lists anyway.
func (w *batchPreviewWidget) renderRowInlineDiff(mr domain.MergeRequest) string {
	reviewersAdded, reviewersRemoved, approversAdded, approversRemoved := reviewerWriteDiff(w.staged, mr)
	added := append(append([]string{}, reviewersAdded...), approversAdded...)
	removed := append(append([]string{}, reviewersRemoved...), approversRemoved...)
	sort.Strings(added)
	sort.Strings(removed)
	return renderInlineDiff(w.styles, removed, added)
}

// renderRowDetails renders the focused row's fuller, qualified diff below the
// list: reviewer and approver additions/removals labeled and listed
// separately, complementing the compact unqualified renderRowInlineDiff
// already on the row itself. Nothing renders for a deselected or unchanged
// row — same as its row's inline diff.
func (w *batchPreviewWidget) renderRowDetails(sb *strings.Builder) {
	if w.cursor >= len(w.rows) {
		return
	}
	row := w.rows[w.cursor]
	if !row.included {
		return
	}
	reviewersAdded, reviewersRemoved, approversAdded, approversRemoved := reviewerWriteDiff(w.staged, row.mr)
	if len(reviewersAdded)+len(reviewersRemoved)+len(approversAdded)+len(approversRemoved) == 0 {
		return
	}
	sb.WriteString("\n")
	for _, u := range reviewersAdded {
		sb.WriteString("      " + w.styles.DiffAdded.Render("+@"+u+" (reviewer)") + "\n")
	}
	for _, u := range reviewersRemoved {
		sb.WriteString("      " + w.styles.DiffRemoved.Render("-@"+u+" (reviewer)") + "\n")
	}
	for _, u := range approversAdded {
		sb.WriteString("      " + w.styles.DiffAdded.Render("+@"+u+" (approver)") + "\n")
	}
	for _, u := range approversRemoved {
		sb.WriteString("      " + w.styles.DiffRemoved.Render("-@"+u+" (approver)") + "\n")
	}
}

func (w *batchPreviewWidget) View() tea.View {
	return tea.NewView(w.render())
}
