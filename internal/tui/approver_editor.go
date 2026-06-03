package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// MembersLoadedMsg carries the result of a lazy project-member fetch.
type MembersLoadedMsg struct {
	Members []domain.ProjectMember
	Err     error
}

// ApproversSavedMsg carries the refreshed MR after saving approvers (or an error).
type ApproversSavedMsg struct {
	MR  domain.MergeRequest
	Err error
}

// ApproverEditorClosedMsg is sent when the editor is dismissed without saving.
type ApproverEditorClosedMsg struct{}

const (
	approverEditorSectionReviewers = 0
	approverEditorSectionMembers   = 1
	approverEditorNumSections      = 2

	approverEditorMaxVisible = 8
)

// approverEditorWidget is the modal overlay for editing the "Approvers" approval rule on an MR.
type approverEditorWidget struct {
	styles         Styles
	keys           ApproverEditorKeyMap
	mr             domain.MergeRequest
	src            mrsvc.MergeRequestSource
	baseCtx        context.Context
	selected       map[string]bool  // username → selected as approver
	userIDByName   map[string]int64 // username → GitLab user ID (from members list)
	members        []domain.ProjectMember
	loading        bool
	saving         bool // true while SaveApprovers is in flight — blocks all key input
	membersErr     error
	section        int // 0=Reviewers, 1=All Members
	reviewerCursor int
	memberCursor   int
	scrollOff      int // scroll offset for the active section
}

// newApproverEditorWidget creates an approver editor for the given MR.
func newApproverEditorWidget(
	baseCtx context.Context,
	mr domain.MergeRequest,
	styles Styles,
	keys ApproverEditorKeyMap,
	src mrsvc.MergeRequestSource,
) *approverEditorWidget {
	selected := make(map[string]bool)
	userIDByName := make(map[string]int64)
	for _, r := range mr.Reviewers {
		if r.IsApprover {
			selected[r.Username] = true
		}
	}
	return &approverEditorWidget{
		styles:       styles,
		keys:         keys,
		mr:           mr,
		src:          src,
		baseCtx:      baseCtx,
		selected:     selected,
		userIDByName: userIDByName,
		section:      approverEditorSectionReviewers,
	}
}

// SetMembers is called when the async member fetch completes.
func (w *approverEditorWidget) SetMembers(members []domain.ProjectMember, err error) {
	w.loading = false
	w.membersErr = err
	if err == nil {
		w.members = members
		for _, m := range members {
			w.userIDByName[m.Username] = m.UserID
		}
	}
}

func (w *approverEditorWidget) Init() tea.Cmd { return nil }

func (w *approverEditorWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return w, nil
	}

	if w.saving {
		return w, nil // block all input while save is in flight
	}

	switch {
	case key.Matches(kMsg, w.keys.Close):
		return w, func() tea.Msg { return ApproverEditorClosedMsg{} }

	case key.Matches(kMsg, w.keys.FocusNext):
		w.section = (w.section + 1) % approverEditorNumSections
		if w.section == approverEditorSectionMembers && !w.loading && w.members == nil && w.membersErr == nil {
			w.loading = true
			return w, w.fetchMembersCmd()
		}

	case key.Matches(kMsg, w.keys.FocusPrev):
		w.section = (w.section + approverEditorNumSections - 1) % approverEditorNumSections

	case key.Matches(kMsg, w.keys.Up):
		w.moveCursor(-1)

	case key.Matches(kMsg, w.keys.Down):
		w.moveCursor(1)

	case key.Matches(kMsg, w.keys.Toggle):
		w.toggleCurrent()

	case key.Matches(kMsg, w.keys.Confirm):
		w.saving = true
		return w, w.saveCmd()
	}

	return w, nil
}

func (w *approverEditorWidget) moveCursor(delta int) {
	switch w.section {
	case approverEditorSectionReviewers:
		next := w.reviewerCursor + delta
		if next >= 0 && next < len(w.mr.Reviewers) {
			w.reviewerCursor = next
			w.adjustScroll()
		}
	case approverEditorSectionMembers:
		next := w.memberCursor + delta
		if next >= 0 && next < len(w.members) {
			w.memberCursor = next
			w.adjustScroll()
		}
	}
}

func (w *approverEditorWidget) adjustScroll() {
	var cursor int
	switch w.section {
	case approverEditorSectionReviewers:
		cursor = w.reviewerCursor
	case approverEditorSectionMembers:
		cursor = w.memberCursor
	}
	if cursor < w.scrollOff {
		w.scrollOff = cursor
	} else if cursor >= w.scrollOff+approverEditorMaxVisible {
		w.scrollOff = cursor - approverEditorMaxVisible + 1
	}
}

func (w *approverEditorWidget) toggleCurrent() {
	var username string
	switch w.section {
	case approverEditorSectionReviewers:
		if w.reviewerCursor < len(w.mr.Reviewers) {
			username = w.mr.Reviewers[w.reviewerCursor].Username
		}
	case approverEditorSectionMembers:
		if w.memberCursor < len(w.members) {
			username = w.members[w.memberCursor].Username
		}
	}
	if username == "" {
		return
	}
	w.selected[username] = !w.selected[username]
}

func (w *approverEditorWidget) fetchMembersCmd() tea.Cmd {
	src := w.src
	projectID := int64(w.mr.ProjectID)
	ctx, cancel := context.WithTimeout(w.baseCtx, fetchTimeout)
	return func() tea.Msg {
		defer cancel()
		members, err := src.GetProjectMembers(ctx, projectID)
		return MembersLoadedMsg{Members: members, Err: err}
	}
}

func (w *approverEditorWidget) saveCmd() tea.Cmd {
	src := w.src
	projectID := int64(w.mr.ProjectID)
	mrIID := int64(w.mr.IID)

	// Snapshot selection and the current ID map at call time.
	selectedUsernames := make([]string, 0, len(w.selected))
	for u, ok := range w.selected {
		if ok {
			selectedUsernames = append(selectedUsernames, u)
		}
	}
	knownIDs := make(map[string]int64, len(w.userIDByName))
	for k, v := range w.userIDByName {
		knownIDs[k] = v
	}

	ctx, cancel := context.WithTimeout(w.baseCtx, fetchTimeout)
	return func() tea.Msg {
		defer cancel()

		// If any selected username has no known GitLab user ID (because the user
		// never opened the "All Members" tab), fetch members now to resolve them.
		needsFetch := false
		for _, u := range selectedUsernames {
			if _, ok := knownIDs[u]; !ok {
				needsFetch = true
				break
			}
		}
		if needsFetch {
			members, err := src.GetProjectMembers(ctx, projectID)
			if err != nil {
				return ApproversSavedMsg{Err: fmt.Errorf("resolve user IDs: %w", err)}
			}
			for _, m := range members {
				knownIDs[m.Username] = m.UserID
			}
		}

		// Build deduplicated user ID slice.
		seen := make(map[int64]bool)
		var userIDs []int64
		for _, u := range selectedUsernames {
			if id, ok := knownIDs[u]; ok && !seen[id] {
				userIDs = append(userIDs, id)
				seen[id] = true
			}
		}

		if err := src.SaveApprovers(ctx, projectID, mrIID, userIDs); err != nil {
			return ApproversSavedMsg{Err: err}
		}
		mr, err := src.FetchMR(ctx, projectID, mrIID)
		return ApproversSavedMsg{MR: mr, Err: err}
	}
}

// reviewerStateLabel returns a short annotation for a reviewer's state.
func reviewerStateLabel(state domain.ReviewerState) string {
	switch state {
	case domain.ReviewerApproved:
		return " (approved)"
	case domain.ReviewerCommented:
		return " (commented)"
	case domain.ReviewerReReviewRequested:
		return " (re-review)"
	default:
		return ""
	}
}

func (w *approverEditorWidget) render() string {
	var sb strings.Builder

	title := fmt.Sprintf("Edit Approvers — !%d %s", w.mr.IID, w.mr.Title)
	sb.WriteString(w.styles.PopupTitle.Render(title) + "\n\n")

	// Section headers
	reviewerHeader := renderSectionHeader("Reviewers", w.section == approverEditorSectionReviewers, w.styles)
	membersHeader := renderSectionHeader("All Members", w.section == approverEditorSectionMembers, w.styles)
	sb.WriteString(reviewerHeader + "   " + membersHeader + "\n\n")

	switch w.section {
	case approverEditorSectionReviewers:
		w.renderReviewers(&sb)
	case approverEditorSectionMembers:
		w.renderMembers(&sb)
	}

	if w.saving {
		sb.WriteString("\n" + w.styles.PopupHint.Render("  Saving…"))
	} else {
		sb.WriteString("\n" + w.styles.PopupHint.Render("  ↑/↓ move  tab:all members  space:toggle  ↵:save  a/esc:close"))
	}
	return w.styles.PopupBorder.Render(sb.String())
}

func (w *approverEditorWidget) renderReviewers(sb *strings.Builder) {
	reviewers := w.mr.Reviewers
	if len(reviewers) == 0 {
		sb.WriteString(w.styles.PopupHint.Render("  (no reviewers assigned)") + "\n")
		return
	}
	end := min(w.scrollOff+approverEditorMaxVisible, len(reviewers))
	for i := w.scrollOff; i < end; i++ {
		r := reviewers[i]
		checked := w.selected[r.Username]
		var markerStyled string
		if checked {
			markerStyled = w.styles.PopupItemMarkerOn.Render(markerChecked)
		} else {
			markerStyled = w.styles.PopupItemMarkerOff.Render(markerUnchecked)
		}
		label := r.Username + reviewerStateLabel(r.State)
		if w.section == approverEditorSectionReviewers && i == w.reviewerCursor {
			sb.WriteString("  " + markerStyled + " " + w.styles.PopupItemFocused.Render(label) + "\n")
		} else {
			sb.WriteString("  " + markerStyled + " " + w.styles.PopupItem.Render(label) + "\n")
		}
	}
	if len(reviewers) > approverEditorMaxVisible {
		shown := min(w.scrollOff+approverEditorMaxVisible, len(reviewers))
		sb.WriteString(w.styles.PopupHint.Render(fmt.Sprintf("  %d–%d / %d", w.scrollOff+1, shown, len(reviewers))) + "\n")
	}
}

func (w *approverEditorWidget) renderMembers(sb *strings.Builder) {
	if w.loading {
		sb.WriteString(w.styles.PopupHint.Render("  Loading members…") + "\n")
		return
	}
	if w.membersErr != nil {
		sb.WriteString(w.styles.PopupHint.Render("  Error: "+w.membersErr.Error()) + "\n")
		return
	}
	if len(w.members) == 0 {
		sb.WriteString(w.styles.PopupHint.Render("  (no members found)") + "\n")
		return
	}
	end := min(w.scrollOff+approverEditorMaxVisible, len(w.members))
	for i := w.scrollOff; i < end; i++ {
		m := w.members[i]
		checked := w.selected[m.Username]
		var markerStyled string
		if checked {
			markerStyled = w.styles.PopupItemMarkerOn.Render(markerChecked)
		} else {
			markerStyled = w.styles.PopupItemMarkerOff.Render(markerUnchecked)
		}
		label := m.Username
		if m.Name != "" && m.Name != m.Username {
			label += " (" + m.Name + ")"
		}
		if w.section == approverEditorSectionMembers && i == w.memberCursor {
			sb.WriteString("  " + markerStyled + " " + w.styles.PopupItemFocused.Render(label) + "\n")
		} else {
			sb.WriteString("  " + markerStyled + " " + w.styles.PopupItem.Render(label) + "\n")
		}
	}
	if len(w.members) > approverEditorMaxVisible {
		shown := min(w.scrollOff+approverEditorMaxVisible, len(w.members))
		sb.WriteString(w.styles.PopupHint.Render(fmt.Sprintf("  %d–%d / %d", w.scrollOff+1, shown, len(w.members))) + "\n")
	}
}

func (w *approverEditorWidget) View() tea.View {
	return tea.NewView(w.render())
}
