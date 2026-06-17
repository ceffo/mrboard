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

// ReviewersSavedMsg carries the refreshed MR after saving reviewers (or an error).
type ReviewersSavedMsg struct {
	MR  domain.MergeRequest
	Err error
}

// ReviewerEditorClosedMsg is sent when the editor is dismissed without saving.
type ReviewerEditorClosedMsg struct{}

const (
	reviewerEditorMaxVisible = 8
	reviewerApproverStar     = "★"
)

// reviewerEditorMode distinguishes the main reviewer list from the search sub-mode.
type reviewerEditorMode int

const (
	reviewerEditorModeList   reviewerEditorMode = iota
	reviewerEditorModeSearch                    // "/" search to add members
)

// stagedReviewer is an entry in the reviewer editor's local staging buffer.
type stagedReviewer struct {
	Username   string
	Name       string
	State      domain.ReviewerState
	IsApprover bool
	UserID     int64 // 0 until resolved via members fetch or team roster
}

// reviewerEditorWidget is the modal overlay for editing the reviewer list on an MR.
type reviewerEditorWidget struct {
	styles  Styles
	keys    ReviewerEditorKeyMap
	mr      domain.MergeRequest
	src     mrsvc.MergeRequestSource
	baseCtx context.Context
	roster  []domain.User // team roster from startup resolution (T action)

	// Staging buffer — local edits committed only on Enter.
	staged        []stagedReviewer
	origApprovers map[string]bool // approver usernames at open time; used to detect changes

	// Project members (lazy fetch for search and ID resolution at save time).
	members        []domain.ProjectMember
	userIDByName   map[string]int64
	loadingMembers bool
	membersErr     error

	// Main list cursor + scroll.
	cursor    int
	scrollOff int

	// Search sub-mode.
	mode          reviewerEditorMode
	searchQuery   string
	searchResults []domain.ProjectMember // filtered by searchQuery
	searchSel     map[int64]bool         // userID → selected in search

	saving bool // true while the save command is in flight
}

// newReviewerEditorWidget creates a staged-buffer editor for the given MR.
// roster is the resolved team from startup (may be nil for group-only configs).
func newReviewerEditorWidget(
	baseCtx context.Context,
	mr domain.MergeRequest,
	styles Styles,
	keys ReviewerEditorKeyMap,
	src mrsvc.MergeRequestSource,
	roster []domain.User,
) *reviewerEditorWidget {
	// Build the initial staged list from the MR's current reviewers, excluding the author.
	staged := make([]stagedReviewer, 0, len(mr.Reviewers))
	origApprovers := make(map[string]bool)
	for _, r := range mr.Reviewers {
		if r.Username == mr.Author {
			continue
		}
		staged = append(staged, stagedReviewer{
			Username:   r.Username,
			Name:       r.Name,
			State:      r.State,
			IsApprover: r.IsApprover,
		})
		if r.IsApprover {
			origApprovers[r.Username] = true
		}
	}
	return &reviewerEditorWidget{
		styles:        styles,
		keys:          keys,
		mr:            mr,
		src:           src,
		baseCtx:       baseCtx,
		roster:        roster,
		staged:        staged,
		origApprovers: origApprovers,
		userIDByName:  make(map[string]int64),
		searchSel:     make(map[int64]bool),
	}
}

// SetMembers is called when the async member fetch completes. Populates userIDByName
// and the search result list.
func (w *reviewerEditorWidget) SetMembers(members []domain.ProjectMember, err error) {
	w.loadingMembers = false
	w.membersErr = err
	if err != nil {
		return
	}
	w.members = members
	for _, m := range members {
		w.userIDByName[m.Username] = m.UserID
	}
	// Also populate IDs for staged reviewers resolved from this fetch.
	for i := range w.staged {
		if w.staged[i].UserID == 0 {
			if id, ok := w.userIDByName[w.staged[i].Username]; ok {
				w.staged[i].UserID = id
			}
		}
	}
	w.refreshSearchResults()
}

func (w *reviewerEditorWidget) Init() tea.Cmd { return nil }

func (w *reviewerEditorWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return w, nil
	}

	if w.saving {
		return w, nil // block all input while save is in flight
	}

	if w.mode == reviewerEditorModeSearch {
		return w.updateSearch(kMsg)
	}
	return w.updateList(kMsg)
}

func (w *reviewerEditorWidget) updateList(kMsg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(kMsg, w.keys.Close):
		return w, func() tea.Msg { return ReviewerEditorClosedMsg{} }

	case key.Matches(kMsg, w.keys.Up):
		if w.cursor > 0 {
			w.cursor--
			w.adjustScroll()
		}

	case key.Matches(kMsg, w.keys.Down):
		if w.cursor < len(w.staged)-1 {
			w.cursor++
			w.adjustScroll()
		}

	case key.Matches(kMsg, w.keys.ToggleApprover):
		if w.cursor < len(w.staged) {
			w.staged[w.cursor].IsApprover = !w.staged[w.cursor].IsApprover
		}

	case key.Matches(kMsg, w.keys.Remove):
		if w.cursor < len(w.staged) {
			w.staged = append(w.staged[:w.cursor], w.staged[w.cursor+1:]...)
			if w.cursor > 0 && w.cursor >= len(w.staged) {
				w.cursor = len(w.staged) - 1
			}
			w.adjustScroll()
		}

	case key.Matches(kMsg, w.keys.Search):
		w.mode = reviewerEditorModeSearch
		w.searchQuery = ""
		w.searchSel = make(map[int64]bool)
		w.refreshSearchResults()
		if !w.loadingMembers && w.members == nil && w.membersErr == nil {
			w.loadingMembers = true
			return w, w.fetchMembersCmd()
		}

	case key.Matches(kMsg, w.keys.SetTeam):
		w.addTeam()

	case key.Matches(kMsg, w.keys.Confirm):
		w.saving = true
		return w, w.saveCmd()
	}

	return w, nil
}

func (w *reviewerEditorWidget) updateSearch(kMsg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(kMsg, w.keys.Close):
		// Esc/r: cancel search, return to list.
		w.mode = reviewerEditorModeList
		w.searchQuery = ""
		w.searchSel = make(map[int64]bool)

	case key.Matches(kMsg, w.keys.Confirm):
		// Enter: add all selected members to staged list.
		for _, m := range w.searchResults {
			if !w.searchSel[m.UserID] {
				continue
			}
			if w.isAlreadyStaged(m.Username) {
				continue
			}
			w.staged = append(w.staged, stagedReviewer{
				Username: m.Username,
				Name:     m.Name,
				UserID:   m.UserID,
			})
			w.userIDByName[m.Username] = m.UserID
		}
		w.mode = reviewerEditorModeList
		w.searchQuery = ""
		w.searchSel = make(map[int64]bool)

	case key.Matches(kMsg, w.keys.ToggleApprover): // space in search = toggle selection
		for i, m := range w.searchResults {
			// We need to track cursor in search results too.
			_ = i
			_ = m
		}
		// Space toggles the focused search result.
		if w.cursor < len(w.searchResults) {
			m := w.searchResults[w.cursor]
			w.searchSel[m.UserID] = !w.searchSel[m.UserID]
		}

	case key.Matches(kMsg, w.keys.Up):
		if w.cursor > 0 {
			w.cursor--
			w.adjustScroll()
		}

	case key.Matches(kMsg, w.keys.Down):
		if w.cursor < len(w.searchResults)-1 {
			w.cursor++
			w.adjustScroll()
		}

	default:
		// Printable character → append to search query.
		if kMsg.Code == tea.KeyBackspace {
			runes := []rune(w.searchQuery)
			if len(runes) > 0 {
				w.searchQuery = string(runes[:len(runes)-1])
			}
		} else if kMsg.Text != "" {
			w.searchQuery += kMsg.Text
		}
		w.cursor = 0
		w.scrollOff = 0
		w.refreshSearchResults()
	}
	return w, nil
}

func (w *reviewerEditorWidget) adjustScroll() {
	if w.cursor < w.scrollOff {
		w.scrollOff = w.cursor
	} else if w.cursor >= w.scrollOff+reviewerEditorMaxVisible {
		w.scrollOff = w.cursor - reviewerEditorMaxVisible + 1
	}
}

// addTeam appends team roster members not already staged, excluding the author.
// Added members are reviewers only (not approvers). Idempotent.
func (w *reviewerEditorWidget) addTeam() {
	if len(w.roster) == 0 {
		return
	}
	for _, u := range w.roster {
		if u.Username == w.mr.Author {
			continue
		}
		if w.isAlreadyStaged(u.Username) {
			continue
		}
		w.staged = append(w.staged, stagedReviewer{
			Username: u.Username,
			Name:     u.Name,
			UserID:   u.ID,
		})
		w.userIDByName[u.Username] = u.ID
	}
}

func (w *reviewerEditorWidget) isAlreadyStaged(username string) bool {
	for _, s := range w.staged {
		if s.Username == username {
			return true
		}
	}
	return false
}

// refreshSearchResults filters w.members by searchQuery, excluding author and already-staged.
func (w *reviewerEditorWidget) refreshSearchResults() {
	q := strings.ToLower(w.searchQuery)
	w.searchResults = w.searchResults[:0]
	for _, m := range w.members {
		if m.Username == w.mr.Author {
			continue
		}
		if w.isAlreadyStaged(m.Username) {
			continue
		}
		label := strings.ToLower(m.Username + " " + m.Name)
		if q == "" || strings.Contains(label, q) {
			w.searchResults = append(w.searchResults, m)
		}
	}
}

func (w *reviewerEditorWidget) fetchMembersCmd() tea.Cmd {
	src := w.src
	projectID := int64(w.mr.ProjectID)
	ctx, cancel := context.WithTimeout(w.baseCtx, fetchTimeout)
	return func() tea.Msg {
		defer cancel()
		members, err := src.GetProjectMembers(ctx, projectID)
		return MembersLoadedMsg{Members: members, Err: err}
	}
}

func (w *reviewerEditorWidget) saveCmd() tea.Cmd {
	src := w.src
	projectID := int64(w.mr.ProjectID)
	mrIID := int64(w.mr.IID)

	// Snapshot staged state at call time.
	type snap struct {
		username   string
		isApprover bool
		userID     int64
	}
	snapped := make([]snap, len(w.staged))
	for i, s := range w.staged {
		snapped[i] = snap{username: s.Username, isApprover: s.IsApprover, userID: s.UserID}
	}
	knownIDs := make(map[string]int64, len(w.userIDByName))
	for k, v := range w.userIDByName {
		knownIDs[k] = v
	}
	origApprovers := make(map[string]bool, len(w.origApprovers))
	for k, v := range w.origApprovers {
		origApprovers[k] = v
	}

	ctx, cancel := context.WithTimeout(w.baseCtx, fetchTimeout)
	return func() tea.Msg {
		defer cancel()

		// Resolve any staged reviewers whose user IDs are still unknown.
		needFetch := false
		for _, s := range snapped {
			if s.userID == 0 {
				if _, ok := knownIDs[s.username]; !ok {
					needFetch = true
					break
				}
			}
		}
		if needFetch {
			members, err := src.GetProjectMembers(ctx, projectID)
			if err != nil {
				return ReviewersSavedMsg{Err: fmt.Errorf("resolve reviewer IDs: %w", err)}
			}
			for _, m := range members {
				knownIDs[m.Username] = m.UserID
			}
		}

		// Build reviewer_ids (replace semantics — always sent).
		seen := make(map[int64]bool)
		var reviewerIDs []int64
		for _, s := range snapped {
			id := s.userID
			if id == 0 {
				id = knownIDs[s.username]
			}
			if id == 0 || seen[id] {
				continue
			}
			reviewerIDs = append(reviewerIDs, id)
			seen[id] = true
		}

		if err := src.SetReviewers(ctx, projectID, mrIID, reviewerIDs); err != nil {
			return ReviewersSavedMsg{Err: err}
		}

		// Write Approvers rule only if the approver flag set changed.
		nowApprovers := make(map[string]bool)
		var approverIDs []int64
		for _, s := range snapped {
			if s.isApprover {
				nowApprovers[s.username] = true
				id := s.userID
				if id == 0 {
					id = knownIDs[s.username]
				}
				if id != 0 {
					approverIDs = append(approverIDs, id)
				}
			}
		}
		approversChanged := len(nowApprovers) != len(origApprovers)
		if !approversChanged {
			for u := range nowApprovers {
				if !origApprovers[u] {
					approversChanged = true
					break
				}
			}
		}
		if approversChanged {
			if err := src.SaveApprovers(ctx, projectID, mrIID, approverIDs); err != nil {
				return ReviewersSavedMsg{Err: err}
			}
		}

		mr, err := src.FetchMR(ctx, projectID, mrIID)
		return ReviewersSavedMsg{MR: mr, Err: err}
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

func (w *reviewerEditorWidget) render() string {
	var sb strings.Builder

	title := fmt.Sprintf("Edit Reviewers — !%d %s", w.mr.IID, w.mr.Title)
	sb.WriteString(w.styles.PopupTitle.Render(title) + "\n\n")

	if w.mode == reviewerEditorModeSearch {
		w.renderSearch(&sb)
	} else {
		w.renderList(&sb)
	}

	return w.styles.PopupBorder.Render(sb.String())
}

func (w *reviewerEditorWidget) renderList(sb *strings.Builder) {
	if len(w.staged) == 0 {
		sb.WriteString(w.styles.PopupHint.Render("  (no reviewers assigned)") + "\n")
	} else {
		end := min(w.scrollOff+reviewerEditorMaxVisible, len(w.staged))
		for i := w.scrollOff; i < end; i++ {
			s := w.staged[i]
			prefix := "  "
			if i == w.cursor {
				prefix = "> "
			}
			label := s.Username
			if s.Name != "" && s.Name != s.Username {
				label += " (" + s.Name + ")"
			}
			if s.IsApprover {
				label += "   " + reviewerApproverStar + " approver"
			}
			label += reviewerStateLabel(s.State)
			if i == w.cursor {
				sb.WriteString(prefix + w.styles.PopupItemFocused.Render(label) + "\n")
			} else {
				sb.WriteString(prefix + w.styles.PopupItem.Render(label) + "\n")
			}
		}
		if len(w.staged) > reviewerEditorMaxVisible {
			shown := min(w.scrollOff+reviewerEditorMaxVisible, len(w.staged))
			sb.WriteString(w.styles.PopupHint.Render(
				fmt.Sprintf("  %d–%d / %d", w.scrollOff+1, shown, len(w.staged))) + "\n")
		}
	}

	if w.saving {
		sb.WriteString("\n" + w.styles.PopupHint.Render("  Saving…"))
	} else {
		sb.WriteString("\n" + w.styles.PopupHint.Render(
			"  ↑/↓ move  space:approver  d:remove  /:search  T:team  ↵:save  r/esc:cancel"))
	}
}

func (w *reviewerEditorWidget) renderSearch(sb *strings.Builder) {
	sb.WriteString(w.styles.PopupHint.Render("  Search: "+w.searchQuery+"_") + "\n\n")

	if w.loadingMembers {
		sb.WriteString(w.styles.PopupHint.Render("  Loading members…") + "\n")
	} else if w.membersErr != nil {
		sb.WriteString(w.styles.PopupHint.Render("  Error: "+w.membersErr.Error()) + "\n")
	} else if len(w.searchResults) == 0 {
		sb.WriteString(w.styles.PopupHint.Render("  (no results)") + "\n")
	} else {
		end := min(w.scrollOff+reviewerEditorMaxVisible, len(w.searchResults))
		for i := w.scrollOff; i < end; i++ {
			m := w.searchResults[i]
			var markerStyled string
			if w.searchSel[m.UserID] {
				markerStyled = w.styles.PopupItemMarkerOn.Render(markerChecked)
			} else {
				markerStyled = w.styles.PopupItemMarkerOff.Render(markerUnchecked)
			}
			label := m.Username
			if m.Name != "" && m.Name != m.Username {
				label += " (" + m.Name + ")"
			}
			if i == w.cursor {
				sb.WriteString("  " + markerStyled + " " + w.styles.PopupItemFocused.Render(label) + "\n")
			} else {
				sb.WriteString("  " + markerStyled + " " + w.styles.PopupItem.Render(label) + "\n")
			}
		}
		if len(w.searchResults) > reviewerEditorMaxVisible {
			shown := min(w.scrollOff+reviewerEditorMaxVisible, len(w.searchResults))
			sb.WriteString(w.styles.PopupHint.Render(
				fmt.Sprintf("  %d–%d / %d", w.scrollOff+1, shown, len(w.searchResults))) + "\n")
		}
	}

	sb.WriteString("\n" + w.styles.PopupHint.Render("  space:select  ↵:add selected  esc:cancel"))
}

func (w *reviewerEditorWidget) View() tea.View {
	return tea.NewView(w.render())
}
