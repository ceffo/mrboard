package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// BatchPreviewBackMsg is sent when the user presses Esc in the batch preview screen,
// requesting a return to the batch reviewer editor.
type BatchPreviewBackMsg struct{}

// BatchPreviewConfirmedMsg is sent when the user presses Enter in the batch preview screen.
// Targets contains focusedMR (when its own staged edit is a real change) plus whichever
// sibling MRs are both included and have a detected change. FocusedMR's write applies
// the edit between Baseline and Staged; every other target only receives the union.
type BatchPreviewConfirmedMsg struct {
	Staged    []stagedReviewer
	Baseline  []stagedReviewer
	FocusedMR domain.MergeRequest
	Targets   []domain.MergeRequest
	KnownIDs  map[string]int64 // see BatchReviewerEditorPreviewMsg.KnownIDs
}

const (
	batchPreviewMaxVisible = 8
	// batchPreviewLabelWidth is the fixed width of a row's "icon !IID repo — title"
	// label, so rows line up and the overlay doesn't resize as the cursor scrolls
	// past short or long MR titles.
	batchPreviewLabelWidth = 44
	batchPreviewRowIndent  = "  "
	batchPreviewDetailPad  = "      "
	batchPreviewHint       = "  ↑/↓ nav  space:include  ↵:apply  esc:back"
	batchPreviewLegend     = "  ─ no change"
)

// previewMRRow is one row in the batch preview list. The focused MR (see
// batchPreviewWidget) is always rows[0]: shown so the user can see what will
// happen to it too, but not selectable — its write is unconditional, not an
// "also apply to" — see focused and collectTargets.
type previewMRRow struct {
	mr domain.MergeRequest
	// included: will this MR be part of the write? Always true, and un-toggleable,
	// when focused; siblings start false — applying to them is opt-in.
	included  bool
	hasChange bool // the write would change this MR (see reviewerWriteDiff)
	focused   bool // this is the MR the reviewer editor was opened on — not a selectable "also apply to"
}

// batchPreviewWidget is the preview overlay shown after the batch reviewer editor.
// It displays the focused MR (always applied, not selectable) followed by each
// sibling MR with a selectable checkbox and a change indicator, letting the user
// opt individual siblings in before the write is dispatched.
//
// The layout is fixed for the widget's lifetime: toggling a row changes its
// checkbox and the styling of its diff, never the number of lines or their width.
type batchPreviewWidget struct {
	styles    Styles
	keys      BatchPreviewKeyMap
	staged    []stagedReviewer
	baseline  []stagedReviewer
	focusedMR domain.MergeRequest
	knownIDs  map[string]int64
	rows      []previewMRRow
	cursor    int
	scrollOff int
	// detailLines is the height reserved below the list for the cursor row's
	// qualified diff: the tallest any row's diff gets.
	detailLines int
}

// newBatchPreviewWidget builds the preview widget from the staged reviewer list,
// the baseline it was edited from, the focused MR (becomes rows[0]), and the
// sibling MR slice (focusedMR excluded — see BatchReviewerEditorPreviewMsg.Siblings).
// Change detection runs at construction time. knownIDs is carried through
// unchanged to BatchPreviewConfirmedMsg — see its doc comment.
func newBatchPreviewWidget(
	staged, baseline []stagedReviewer,
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
		hasChange: hasWriteDiff(staged, focusedMR, false),
		focused:   true,
	})
	for _, sib := range siblings {
		rows = append(rows, previewMRRow{
			mr:        sib,
			hasChange: hasWriteDiff(staged, sib, true),
		})
	}
	w := &batchPreviewWidget{
		styles:    styles,
		keys:      keys,
		staged:    staged,
		baseline:  baseline,
		focusedMR: focusedMR,
		knownIDs:  knownIDs,
		rows:      rows,
	}
	for _, row := range rows {
		ra, rr, aa, ar := reviewerWriteDiff(staged, row.mr, !row.focused)
		w.detailLines = max(w.detailLines, len(ra)+len(rr)+len(aa)+len(ar))
	}
	return w
}

// hasWriteDiff reports whether applying staged to mr would change it.
func hasWriteDiff(staged []stagedReviewer, mr domain.MergeRequest, union bool) bool {
	ra, rr, aa, ar := reviewerWriteDiff(staged, mr, union)
	return len(ra)+len(rr)+len(aa)+len(ar) > 0
}

// reviewerWriteDiff reports how applying staged to mr would change its reviewer
// and approver membership: reviewersAdded/reviewersRemoved are reviewer username
// additions/removals; approversAdded/approversRemoved are IsApprover-flag
// transitions for usernames that remain reviewers on both sides. Mirrors the two
// writes ApplyReviewerChanges performs (SetReviewers, SaveApprovers).
//
// With union set, only additions are reported: the write for a sibling MR never
// removes a reviewer or clears an approver flag (mrsvc.ReviewerWriteUnion).
//
// A username never appears in both a reviewers list and the corresponding
// approvers list: an approver is a reviewer, so listing it as a plain reviewer
// addition/removal alongside its approver one would just be the same person
// twice under two labels — the approver entry is the more specific one and wins.
func reviewerWriteDiff(
	staged []stagedReviewer, mr domain.MergeRequest, union bool,
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
		} else if !union && !s.IsApprover && cur.isApprover {
			approversRemoved = append(approversRemoved, s.Username)
			approversRemovedSet[s.Username] = true
		}
	}
	for _, r := range mr.Reviewers {
		if union || stagedSet[r.Username] {
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
		baseline := make([]stagedReviewer, len(w.baseline))
		copy(baseline, w.baseline)
		knownIDs := make(map[string]int64, len(w.knownIDs))
		for k, v := range w.knownIDs {
			knownIDs[k] = v
		}
		focusedMR := w.focusedMR
		return w, func() tea.Msg {
			return BatchPreviewConfirmedMsg{
				Staged: staged, Baseline: baseline, FocusedMR: focusedMR, Targets: targets, KnownIDs: knownIDs,
			}
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

// renderRow returns row i's full line. Its diff is always shown — muted while the
// row is excluded — so ticking a row restyles it in place instead of growing it.
func (w *batchPreviewWidget) renderRow(i int) string {
	row := w.rows[i]
	repo := row.mr.ProjectPath
	if idx := strings.LastIndex(repo, "/"); idx >= 0 {
		repo = repo[idx+1:]
	}
	suffix := ""
	if row.focused {
		suffix = " (this)"
	}
	labelW := batchPreviewLabelWidth - lip.Width(suffix)
	label := fmt.Sprintf("%s !%d %s — %s", phaseIcon(row.mr.Phase), row.mr.IID, repo, row.mr.Title)
	label = padRight(truncateWidth(label, labelW), labelW) + suffix

	var markerStyled string
	switch {
	case row.focused:
		markerStyled = w.styles.PopupHint.Render(markerFixed) // always applied, not a toggle
	case row.included:
		markerStyled = w.styles.PopupItemMarkerOn.Render(markerChecked)
	default:
		markerStyled = w.styles.PopupItemMarkerOff.Render(markerUnchecked)
	}

	var labelStyled string
	if i == w.cursor {
		labelStyled = w.styles.PopupItemFocused.Render(label)
	} else {
		labelStyled = w.styles.PopupItem.Render(label)
	}

	line := batchPreviewRowIndent + markerStyled + " " + labelStyled
	if !row.hasChange {
		return line + " " + w.styles.PopupHint.Render("─")
	}
	return line + " " + w.renderRowInlineDiff(row)
}

func (w *batchPreviewWidget) render() string {
	// Every line is padded to the widest line any row can produce, measured over
	// all rows rather than the visible window, so the overlay keeps one size.
	title := fmt.Sprintf("Preview Changes (%d to write)", len(w.collectTargets()))
	width := max(lip.Width(title), lip.Width(batchPreviewHint), lip.Width(batchPreviewLegend))
	rowLines := make([]string, len(w.rows))
	for i := range w.rows {
		rowLines[i] = w.renderRow(i)
		width = max(width, lip.Width(rowLines[i]))
	}

	var sb strings.Builder
	line := func(s string) { sb.WriteString(padRight(s, width) + "\n") }

	line(w.styles.PopupTitle.Render(title))
	line("")
	end := min(w.scrollOff+batchPreviewMaxVisible, len(w.rows))
	for i := w.scrollOff; i < end; i++ {
		line(rowLines[i])
	}
	if len(w.rows) > batchPreviewMaxVisible {
		line(w.styles.PopupHint.Render(
			fmt.Sprintf("  %d–%d / %d", w.scrollOff+1, end, len(w.rows))))
	}
	if w.detailLines > 0 {
		line("")
		details := w.renderRowDetails()
		for i := range w.detailLines {
			if i < len(details) {
				line(details[i])
			} else {
				line("")
			}
		}
	}
	line("")
	line(w.styles.PopupHint.Render(batchPreviewLegend))
	line("")
	sb.WriteString(padRight(w.styles.PopupHint.Render(batchPreviewHint), width))

	return w.styles.PopupBorder.Render(sb.String())
}

// padRight pads s with spaces to width display columns; s is returned as-is
// when already at least that wide.
func padRight(s string, width int) string {
	if gap := width - lip.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// renderRowInlineDiff returns the same-line diff of what applying staged would
// actually do to the row's MR — added/removed reviewers and approvers merged
// into one "-@user +@user" form (see renderInlineDiff). Deliberately drops
// reviewerWriteDiff's (reviewer)/(approver) qualifier for compactness — a
// username never appears in both lists anyway. An excluded row renders the same
// text in the muted hint style, so it reads as "what you'd get" rather than
// "what will happen".
func (w *batchPreviewWidget) renderRowInlineDiff(row previewMRRow) string {
	reviewersAdded, reviewersRemoved, approversAdded, approversRemoved := reviewerWriteDiff(w.staged, row.mr, !row.focused)
	added := append(append([]string{}, reviewersAdded...), approversAdded...)
	removed := append(append([]string{}, reviewersRemoved...), approversRemoved...)
	sort.Strings(added)
	sort.Strings(removed)
	if row.included {
		return renderInlineDiff(w.styles, removed, added)
	}
	return w.styles.PopupHint.Render(plainInlineDiff(removed, added))
}

// plainInlineDiff is renderInlineDiff's text without the per-token colors.
func plainInlineDiff(removed, added []string) string {
	parts := make([]string, 0, len(removed)+len(added))
	for _, u := range removed {
		parts = append(parts, "-@"+u)
	}
	for _, u := range added {
		parts = append(parts, "+@"+u)
	}
	return strings.Join(parts, " ")
}

// renderRowDetails returns the cursor row's fuller, qualified diff lines:
// reviewer and approver additions/removals labeled and listed separately,
// complementing the compact unqualified renderRowInlineDiff already on the row
// itself. The caller reserves detailLines lines for it whatever the cursor row.
func (w *batchPreviewWidget) renderRowDetails() []string {
	if w.cursor >= len(w.rows) {
		return nil
	}
	row := w.rows[w.cursor]
	reviewersAdded, reviewersRemoved, approversAdded, approversRemoved := reviewerWriteDiff(w.staged, row.mr, !row.focused)
	added, removed := w.styles.DiffAdded, w.styles.DiffRemoved
	if !row.included {
		added, removed = w.styles.PopupHint, w.styles.PopupHint
	}
	var out []string
	for _, u := range reviewersAdded {
		out = append(out, batchPreviewDetailPad+added.Render("+@"+u+" (reviewer)"))
	}
	for _, u := range reviewersRemoved {
		out = append(out, batchPreviewDetailPad+removed.Render("-@"+u+" (reviewer)"))
	}
	for _, u := range approversAdded {
		out = append(out, batchPreviewDetailPad+added.Render("+@"+u+" (approver)"))
	}
	for _, u := range approversRemoved {
		out = append(out, batchPreviewDetailPad+removed.Render("-@"+u+" (approver)"))
	}
	return out
}

func (w *batchPreviewWidget) View() tea.View {
	return tea.NewView(w.render())
}
