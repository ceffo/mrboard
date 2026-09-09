package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

// MembersLoadedMsg carries the result of a lazy project-member fetch.
type MembersLoadedMsg struct {
	Members []domain.ProjectMember
	Err     error
}

// ReviewersSavedMsg carries the refreshed MR after saving reviewers (or an error).
// ApproversChanged reports whether the write actually modified the "Approvers"
// rule; the auto-notification only fires when it did (a plain reviewer
// reassignment is not worth pinging the channel about).
type ReviewersSavedMsg struct {
	MR               domain.MergeRequest
	ApproversChanged bool
	Err              error
}

// ReviewerEditorClosedMsg is sent when the editor is dismissed without saving.
type ReviewerEditorClosedMsg struct{}

// BatchReviewerEditorPreviewMsg is sent when the user confirms the staged reviewer
// list while sibling MRs are present, requesting the per-MR preview screen before
// writing to more than the focused MR.
type BatchReviewerEditorPreviewMsg struct {
	Staged []stagedReviewer
	// Siblings is every other MR sharing the JIRA key, excluding FocusedMR —
	// FocusedMR's own write is unconditional and never listed as a "sibling".
	Siblings  []domain.MergeRequest
	FocusedMR domain.MergeRequest
	// KnownIDs is the editor's resolved username→userID map (see userIDByName),
	// carried through so the eventual batch write reuses it instead of
	// resolving IDs from scratch per target — see makeReviewerWriteCmd.
	KnownIDs map[string]int64
}

const (
	reviewerEditorMaxVisible = 8
)

// reviewerEditorMode distinguishes the main reviewer list from the search sub-mode.
type reviewerEditorMode int

const (
	reviewerEditorModeList   reviewerEditorMode = iota
	reviewerEditorModeSearch                    // "/" search to add members
)

// reviewerEditorPanel identifies which panel owns keyboard focus: the staged
// reviewer list being edited, or the read-only list of sibling MRs sharing the
// same JIRA key.
type reviewerEditorPanel int

const (
	reviewerEditorPanelReviewers reviewerEditorPanel = iota
	reviewerEditorPanelSiblings
)

// stagedReviewer is an entry in the reviewer editor's local staging buffer.
type stagedReviewer struct {
	Username   string
	Name       string
	State      domain.ReviewerState
	IsApprover bool
	UserID     int64 // 0 until resolved via members fetch or team roster
}

// reviewerWriter is the narrow slice of mrsvc.MergeRequestSource that
// reviewerEditorWidget actually calls (GetProjectMembers directly, plus
// FetchMR/SaveApprovers/SetReviewers via mrsvc.ApplyReviewerChanges).
// Declared at this widget's own site per docs/clean_architecture.md §7.3 —
// GitLabAdapter satisfies it implicitly, no change needed there.
type reviewerWriter interface {
	FetchMR(ctx context.Context, projectID int64, mrIID int64) (domain.MergeRequest, error)
	GetProjectMembers(ctx context.Context, projectID int64) ([]domain.ProjectMember, error)
	SaveApprovers(ctx context.Context, projectID int64, mrIID int64, userIDs []int64) error
	SetReviewers(ctx context.Context, projectID int64, mrIID int64, userIDs []int64) error
}

// reviewerEditorWidget is the modal overlay for editing the reviewer list on an MR.
// When the MR shares a JIRA key with other open MRs, it also shows a sibling panel
// so the same reviewer/approver edit can be applied to them — see siblings.
type reviewerEditorWidget struct {
	styles     Styles
	keys       ReviewerEditorKeyMap
	mr         domain.MergeRequest
	src        reviewerWriter
	baseCtx    context.Context
	roster     []domain.User // team roster from startup resolution (T action)
	keyMatcher domain.TicketKeyMatcher

	// Staging buffer — local edits committed only on Enter.
	staged []stagedReviewer

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

	// Other MRs sharing the same JIRA key as mr (mr itself excluded), and the
	// read-only panel used to browse them. Empty when mr has no JIRA key or no
	// other open MR shares it.
	siblings     []domain.MergeRequest
	panel        reviewerEditorPanel
	sibCursor    int
	sibScrollOff int

	saving bool // true while the save command is in flight
}

// newReviewerEditorWidget creates a staged-buffer editor for the given MR.
// roster is the resolved team from startup (may be nil for group-only configs).
// siblings is every MR sharing the same JIRA key as mr, including mr itself
// (e.g. Model.SiblingMRs(m.keyMatcher.ExtractFromTitle(mr.Title))); mr itself is
// filtered out below, since it's the primary edit target, not an "also apply to".
// An empty result (after filtering) means mr has no siblings to offer a batch
// apply to.
func newReviewerEditorWidget(
	baseCtx context.Context,
	mr domain.MergeRequest,
	siblings []domain.MergeRequest,
	styles Styles,
	keys ReviewerEditorKeyMap,
	src reviewerWriter,
	roster []domain.User,
	keyMatcher domain.TicketKeyMatcher,
) *reviewerEditorWidget {
	// Build the initial staged list from the MR's current reviewers, excluding the author.
	staged := make([]stagedReviewer, 0, len(mr.Reviewers))
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
	}
	others := make([]domain.MergeRequest, 0, len(siblings))
	for _, s := range siblings {
		if s.ProjectID == mr.ProjectID && s.IID == mr.IID {
			continue
		}
		others = append(others, s)
	}
	return &reviewerEditorWidget{
		styles:       styles,
		keys:         keys,
		mr:           mr,
		src:          src,
		baseCtx:      baseCtx,
		roster:       roster,
		keyMatcher:   keyMatcher,
		staged:       staged,
		userIDByName: make(map[string]int64),
		searchSel:    make(map[int64]bool),
		siblings:     others,
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

// Context returns the keybinding context matching the widget's current mode;
// the search sub-mode captures text so global printable keys reach the query.
func (w *reviewerEditorWidget) Context() *Context {
	if w.mode == reviewerEditorModeSearch {
		return ReviewerSearchCtx
	}
	return ReviewerEditorCtx
}

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
	case w.keys.Close.Match(kMsg):
		return w, func() tea.Msg { return ReviewerEditorClosedMsg{} }

	case w.keys.Tab.Match(kMsg):
		if w.panel == reviewerEditorPanelReviewers {
			w.panel = reviewerEditorPanelSiblings
		} else {
			w.panel = reviewerEditorPanelReviewers
		}

	case w.keys.Up.Match(kMsg):
		if w.panel == reviewerEditorPanelSiblings {
			if w.sibCursor > 0 {
				w.sibCursor--
				w.adjustScrollSiblings()
			}
			break
		}
		if w.cursor > 0 {
			w.cursor--
			w.adjustScroll()
		}

	case w.keys.Down.Match(kMsg):
		if w.panel == reviewerEditorPanelSiblings {
			if w.sibCursor < len(w.siblings)-1 {
				w.sibCursor++
				w.adjustScrollSiblings()
			}
			break
		}
		if w.cursor < len(w.staged)-1 {
			w.cursor++
			w.adjustScroll()
		}

	case w.keys.ToggleApprover.Match(kMsg):
		if w.panel == reviewerEditorPanelReviewers && w.cursor < len(w.staged) {
			w.staged[w.cursor].IsApprover = !w.staged[w.cursor].IsApprover
		}

	case w.keys.Remove.Match(kMsg):
		if w.panel == reviewerEditorPanelReviewers && w.cursor < len(w.staged) {
			w.staged = append(w.staged[:w.cursor], w.staged[w.cursor+1:]...)
			if w.cursor > 0 && w.cursor >= len(w.staged) {
				w.cursor = len(w.staged) - 1
			}
			w.adjustScroll()
		}

	case w.keys.Search.Match(kMsg):
		if w.panel != reviewerEditorPanelReviewers {
			break
		}
		w.mode = reviewerEditorModeSearch
		w.searchQuery = ""
		w.searchSel = make(map[int64]bool)
		w.refreshSearchResults()
		if !w.loadingMembers && w.members == nil && w.membersErr == nil {
			w.loadingMembers = true
			return w, w.fetchMembersCmd()
		}

	case w.keys.SetTeam.Match(kMsg):
		if w.panel == reviewerEditorPanelReviewers {
			w.addTeam()
		}

	case w.keys.Confirm.Match(kMsg):
		return w.confirm()
	}

	return w, nil
}

// confirm commits the staged edit. With no sibling MRs it writes directly to
// mr; otherwise it hands off to the batch preview screen so the user can review
// and exclude individual siblings before anything else is written.
func (w *reviewerEditorWidget) confirm() (tea.Model, tea.Cmd) { //nolint:ireturn
	if len(w.siblings) == 0 {
		w.saving = true
		return w, w.saveCmd()
	}
	staged := make([]stagedReviewer, len(w.staged))
	copy(staged, w.staged)
	siblings := make([]domain.MergeRequest, len(w.siblings))
	copy(siblings, w.siblings)
	knownIDs := make(map[string]int64, len(w.userIDByName))
	for k, v := range w.userIDByName {
		knownIDs[k] = v
	}
	focusedMR := w.mr
	return w, func() tea.Msg {
		return BatchReviewerEditorPreviewMsg{Staged: staged, Siblings: siblings, FocusedMR: focusedMR, KnownIDs: knownIDs}
	}
}

func (w *reviewerEditorWidget) updateSearch(kMsg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Search mode uses its own arrow-only keymap: letters like j/k/v/q must
	// be typed into the query, not trigger list-mode actions.
	searchKeys := DefaultReviewerSearchKeyMap
	switch {
	case searchKeys.Cancel.Match(kMsg):
		// Esc: cancel search, return to list.
		w.mode = reviewerEditorModeList
		w.searchQuery = ""
		w.searchSel = make(map[int64]bool)

	case searchKeys.Confirm.Match(kMsg):
		// Enter: add selected members; no-op if nothing selected (keeps search open).
		if len(w.searchSel) == 0 {
			break
		}
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

	case searchKeys.Select.Match(kMsg):
		// Space toggles the focused search result.
		if w.cursor < len(w.searchResults) {
			m := w.searchResults[w.cursor]
			w.searchSel[m.UserID] = !w.searchSel[m.UserID]
		}

	case searchKeys.Up.Match(kMsg):
		if w.cursor > 0 {
			w.cursor--
			w.adjustScroll()
		}

	case searchKeys.Down.Match(kMsg):
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

func (w *reviewerEditorWidget) adjustScrollSiblings() {
	if w.sibCursor < w.sibScrollOff {
		w.sibScrollOff = w.sibCursor
	} else if w.sibCursor >= w.sibScrollOff+reviewerEditorMaxVisible {
		w.sibScrollOff = w.sibCursor - reviewerEditorMaxVisible + 1
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

// refreshSearchResults filters w.members by searchQuery (substring of full name),
// excluding the MR author and already-staged reviewers.
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
		name := m.Name
		if name == "" {
			name = m.Username
		}
		if q == "" || strings.Contains(strings.ToLower(name), q) {
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

// saveCmd writes the staged edit to the focused MR via the shared reviewer-write
// use case (see makeReviewerWriteCmd), seeding knownIDs from userIDByName so a
// save that follows a completed member fetch never needs a redundant one.
func (w *reviewerEditorWidget) saveCmd() tea.Cmd {
	return makeReviewerWriteCmd(w.baseCtx, w.src, w.mr, w.staged, w.userIDByName)
}

// Hint lines for the reviewer-list and sibling panels. Kept as consts so the
// header's gap padding (anchored to the widest one) and each panel's own
// bottom hint always agree.
const (
	reviewerListHint = "  ↑/↓ move  space:approver  d:remove  /:search  T:team  tab:siblings  ↵:save  a/esc:cancel"
	reviewerSibHint  = "  ↑/↓ move  tab:reviewers  ↵:save  a/esc:cancel"
)

func (w *reviewerEditorWidget) render() string {
	var sb strings.Builder

	// Line 1: "!IID repoName" (left) — "Edit Reviewers & Approvers" (right)
	// Width is anchored to the widest hint line so the header lines up
	// regardless of which panel is showing.
	contentW := max(lip.Width(reviewerListHint), lip.Width(reviewerSibHint))
	repoName := w.mr.ProjectPath
	if i := strings.LastIndex(repoName, "/"); i >= 0 {
		repoName = repoName[i+1:]
	}
	leftStr := fmt.Sprintf("!%d %s", w.mr.IID, repoName)
	rightStr := "Edit Reviewers & Approvers"
	gap := contentW - lip.Width(leftStr) - lip.Width(rightStr)
	if gap < 1 {
		gap = 1
	}
	line1 := w.styles.PopupHint.Render(leftStr) + strings.Repeat(" ", gap) + w.styles.PopupTitle.Render(rightStr)
	// Line 2: MR title
	line2 := w.styles.PopupItem.Render(w.mr.Title)
	sb.WriteString(line1 + "\n" + line2 + "\n")
	if key := w.keyMatcher.ExtractFromTitle(w.mr.Title); key != "" && len(w.siblings) > 0 {
		sb.WriteString(w.styles.PopupHint.Render(fmt.Sprintf("🎫 %s · %d linked MRs", key, len(w.siblings)+1)) + "\n")
	}
	sb.WriteString("\n")

	switch {
	case w.mode == reviewerEditorModeSearch:
		w.renderSearch(&sb)
	case w.panel == reviewerEditorPanelSiblings:
		w.renderSiblings(&sb)
	default:
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
			label := name
			if icon := reviewerIcon(s.State, s.IsApprover); icon != "" {
				label += " " + icon
			}
			if i == w.cursor {
				sb.WriteString("  " + markerStyled + " " + w.styles.PopupItemFocused.Render(label) + "\n")
			} else {
				sb.WriteString("  " + markerStyled + " " + w.styles.PopupItem.Render(label) + "\n")
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
		sb.WriteString("\n" + w.styles.PopupHint.Render(reviewerListHint))
	}
}

// renderSiblings shows the read-only list of other MRs sharing mr's JIRA key
// (mr itself is never listed — its own write is unconditional, not an "also
// apply to"). Rows are kept short (icon, IID, repo) so the modal's width
// doesn't swing with MR title length; when a row's "Approvers" rule conflicts
// with mr's, the added/removed usernames render inline at the end of that same
// row — a warning, not a block: the write still applies to it on confirm (via
// the batch preview screen) unless the user excludes it there. The focused
// row's title (also left off the row itself) renders below the list.
func (w *reviewerEditorWidget) renderSiblings(sb *strings.Builder) {
	plural := "s"
	if len(w.siblings) == 1 {
		plural = ""
	}
	header := fmt.Sprintf("Also apply to (%d MR%s)", len(w.siblings), plural)
	sb.WriteString(w.styles.PopupSectionFocused.Render("▶ "+header) + "\n")

	if len(w.siblings) == 0 {
		sb.WriteString(w.styles.PopupHint.Render("  (no sibling MRs)") + "\n")
		return
	}

	end := min(w.sibScrollOff+reviewerEditorMaxVisible, len(w.siblings))
	for i := w.sibScrollOff; i < end; i++ {
		sib := w.siblings[i]
		repo := sib.ProjectPath
		if idx := strings.LastIndex(repo, "/"); idx >= 0 {
			repo = repo[idx+1:]
		}
		label := fmt.Sprintf("%s !%d %s", phaseIcon(sib.Phase), sib.IID, repo)
		if added, removed := domain.ApproversDiff(w.mr, sib); len(added)+len(removed) > 0 {
			label += " " + renderInlineDiff(w.styles, removed, added)
		}
		if i == w.sibCursor {
			sb.WriteString("  " + w.styles.PopupItemFocused.Render(label) + "\n")
		} else {
			sb.WriteString("  " + w.styles.PopupItem.Render(label) + "\n")
		}
	}
	if len(w.siblings) > reviewerEditorMaxVisible {
		shown := min(w.sibScrollOff+reviewerEditorMaxVisible, len(w.siblings))
		sb.WriteString(w.styles.PopupHint.Render(
			fmt.Sprintf("  %d–%d / %d", w.sibScrollOff+1, shown, len(w.siblings))) + "\n")
	}

	w.renderSiblingDetails(sb)

	sb.WriteString("\n" + w.styles.PopupHint.Render(reviewerSibHint))
}

// renderSiblingDetails renders the focused sibling row's title below the
// list — kept out of the row itself so the list stays a stable width. The
// row's inline diff (see renderSiblings) is already the complete diagnostic
// for a conflict, so there's nothing to repeat here.
func (w *reviewerEditorWidget) renderSiblingDetails(sb *strings.Builder) {
	if w.sibCursor >= len(w.siblings) {
		return
	}
	sib := w.siblings[w.sibCursor]
	contentW := max(lip.Width(reviewerListHint), lip.Width(reviewerSibHint))
	sb.WriteString("\n" + w.styles.PopupItem.Render(truncateWidth(sib.Title, contentW)) + "\n")
}

// renderInlineDiff formats removed/added usernames as a single same-line diff
// — "-@user1 +@user2", removed before added, each token colored red/green
// with no space between the sign and the "@". Shared by the reviewer editor's
// sibling panel and the batch preview screen.
func renderInlineDiff(styles Styles, removed, added []string) string {
	parts := make([]string, 0, len(removed)+len(added))
	for _, u := range removed {
		parts = append(parts, styles.DiffRemoved.Render("-@"+u))
	}
	for _, u := range added {
		parts = append(parts, styles.DiffAdded.Render("+@"+u))
	}
	return strings.Join(parts, " ")
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
			label := m.Name
			if label == "" {
				label = m.Username
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
