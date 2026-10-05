package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

const (
	editorTestApprover = "alice"
	editorTestOther    = "bob"
	editorTestRepo     = "org/alpha"
)

func focusedMR() domain.MergeRequest {
	return domain.MergeRequest{
		ID: 1, IID: 10, ProjectID: 100, Author: "carol", ProjectPath: editorTestRepo,
		Title:     "feat(OD-1): change",
		Approvers: []string{editorTestApprover},
		Reviewers: []domain.ReviewerInfo{{Username: editorTestApprover, IsApprover: true}},
	}
}

func siblingMR(iid int, approverUsernames ...string) domain.MergeRequest {
	reviewers := make([]domain.ReviewerInfo, len(approverUsernames))
	for i, u := range approverUsernames {
		reviewers[i] = domain.ReviewerInfo{Username: u, IsApprover: true}
	}
	return domain.MergeRequest{
		ID: iid, IID: iid, ProjectID: 100, ProjectPath: editorTestRepo,
		Title: "feat(OD-1): related change", Approvers: approverUsernames, Reviewers: reviewers,
	}
}

func newTestReviewerEditor(siblings []domain.MergeRequest, src *mocks.MockMergeRequestSource) *reviewerEditorWidget {
	return newReviewerEditorWidget(
		context.Background(), focusedMR(), siblings, Styles{}, DefaultReviewerEditorKeyMap, src, nil,
		domain.NewTicketKeyMatcher(false),
	)
}

// --- Confirm branching ---

func TestReviewerEditorWidget_NoSiblings_ConfirmSavesDirectly(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	// An unedited confirm changes nothing, so the write use case only re-reads the MR.
	src.EXPECT().FetchMR(mock.Anything, int64(100), int64(10)).Return(focusedMR(), nil).Once()

	w := newTestReviewerEditor(nil, src) // no siblings
	updated, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	w2 := updated.(*reviewerEditorWidget)

	assert.True(t, w2.saving, "expected saving=true on the direct-save path")
	require.NotNil(t, cmd, "expected a non-nil save command")
	msg := cmd()
	_, ok := msg.(ReviewersSavedMsg)
	assert.True(t, ok, "expected ReviewersSavedMsg, got %T", msg)
}

func TestReviewerEditorWidget_SingleSelfSibling_ConfirmSavesDirectly(t *testing.T) {
	// SiblingMRs includes the focused MR itself; a length-1 slice means "no
	// other siblings" and must take the same direct-save path as nil.
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchMR(mock.Anything, int64(100), int64(10)).Return(focusedMR(), nil).Once()

	w := newTestReviewerEditor([]domain.MergeRequest{focusedMR()}, src)
	_, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, ok := cmd().(ReviewersSavedMsg)
	assert.True(t, ok, "expected direct save when siblings only contains the focused MR")
}

func TestReviewerEditorWidget_WithSiblings_ConfirmOpensPreview(t *testing.T) {
	siblings := []domain.MergeRequest{focusedMR(), siblingMR(20, editorTestApprover), siblingMR(30, editorTestOther)}
	w := newTestReviewerEditor(siblings, nil) // src unused on this path

	updated, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	w2 := updated.(*reviewerEditorWidget)

	assert.False(t, w2.saving, "saving must stay false when handing off to the preview screen")
	require.NotNil(t, cmd, "expected a non-nil preview command")
	msg, ok := cmd().(BatchReviewerEditorPreviewMsg)
	require.True(t, ok, "expected BatchReviewerEditorPreviewMsg, got %T", cmd())
	assert.Len(t, msg.Siblings, 2, "expected 2 siblings forwarded, self excluded")
	assert.Equal(t, focusedMR().IID, msg.FocusedMR.IID)
	assert.Equal(t, msg.Staged, msg.Baseline, "an unedited staging buffer equals the baseline it started from")
}

func TestReviewerEditorWidget_Remove_KeepsBaselineIntact(t *testing.T) {
	siblings := []domain.MergeRequest{focusedMR(), siblingMR(20)}
	w := newTestReviewerEditor(siblings, nil)

	updated, _ := w.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	w2 := updated.(*reviewerEditorWidget)
	_, cmd := w2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg, ok := cmd().(BatchReviewerEditorPreviewMsg)

	require.True(t, ok, "expected BatchReviewerEditorPreviewMsg, got %T", cmd())
	assert.Empty(t, msg.Staged, "the removed reviewer is gone from the staged list")
	require.Len(t, msg.Baseline, 1, "the baseline still records who was on the MR when the editor opened")
	assert.Equal(t, editorTestApprover, msg.Baseline[0].Username)
}

func TestReviewerEditorWidget_WithSiblings_ConfirmForwardsKnownIDs(t *testing.T) {
	// The editor's own resolved IDs must reach the batch write path so it can
	// reuse them instead of starting from an empty map per target — the
	// divergence the shared write use case (makeReviewerWriteCmd) closes.
	siblings := []domain.MergeRequest{focusedMR(), siblingMR(20, editorTestApprover)}
	w := newTestReviewerEditor(siblings, nil)
	w.SetMembers([]domain.ProjectMember{{UserID: 1, Username: editorTestApprover}}, nil)

	_, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg, ok := cmd().(BatchReviewerEditorPreviewMsg)
	require.True(t, ok, "expected BatchReviewerEditorPreviewMsg, got %T", cmd())
	assert.Equal(t, map[string]int64{editorTestApprover: 1}, msg.KnownIDs)
}

// --- Sibling panel navigation ---

func TestReviewerEditorWidget_Tab_TogglesPanel(t *testing.T) {
	w := newTestReviewerEditor([]domain.MergeRequest{focusedMR(), siblingMR(20)}, nil)
	assert.Equal(t, reviewerEditorPanelReviewers, w.panel, "expected reviewers panel focused initially")

	updated, _ := w.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	w2 := updated.(*reviewerEditorWidget)
	assert.Equal(t, reviewerEditorPanelSiblings, w2.panel, "expected siblings panel after first tab")

	updated, _ = w2.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	w3 := updated.(*reviewerEditorWidget)
	assert.Equal(t, reviewerEditorPanelReviewers, w3.panel, "expected reviewers panel after second tab")
}

func TestReviewerEditorWidget_SiblingsPanel_DownMovesSiblingCursorOnly(t *testing.T) {
	w := newTestReviewerEditor([]domain.MergeRequest{focusedMR(), siblingMR(20), siblingMR(30)}, nil)
	w.panel = reviewerEditorPanelSiblings

	updated, _ := w.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	w2 := updated.(*reviewerEditorWidget)

	assert.Equal(t, 1, w2.sibCursor, "expected sibCursor=1")
	assert.Equal(t, 0, w2.cursor, "expected reviewer cursor unchanged at 0")
}

func TestReviewerEditorWidget_SiblingsPanel_ToggleApproverIsNoop(t *testing.T) {
	w := newTestReviewerEditor([]domain.MergeRequest{focusedMR(), siblingMR(20)}, nil)
	w.panel = reviewerEditorPanelSiblings
	before := w.staged[0].IsApprover

	updated, _ := w.Update(tea.KeyPressMsg{Text: " ", Code: ' '})
	w2 := updated.(*reviewerEditorWidget)

	assert.Equal(t, before, w2.staged[0].IsApprover, "toggling approver from the siblings panel must be a no-op")
}

func TestReviewerEditorWidget_SiblingsPanel_SearchIsNoop(t *testing.T) {
	w := newTestReviewerEditor([]domain.MergeRequest{focusedMR(), siblingMR(20)}, nil)
	w.panel = reviewerEditorPanelSiblings

	updated, _ := w.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	w2 := updated.(*reviewerEditorWidget)

	assert.Equal(t, reviewerEditorModeList, w2.mode, "search must not activate from the siblings panel")
}

// --- Sibling additions rendering ---

func TestReviewerEditorWidget_RenderSiblings_OmitsSelfAndShowsOnlyAdditions(t *testing.T) {
	siblings := []domain.MergeRequest{
		focusedMR(),                       // self — excluded from the list entirely
		siblingMR(20, editorTestApprover), // already has everything staged — no diff
		siblingMR(30, editorTestOther),    // lacks the staged approver — inline addition
	}
	w := newTestReviewerEditor(siblings, nil)
	w.panel = reviewerEditorPanelSiblings

	out := w.render()

	assert.Len(t, w.siblings, 2, "expected self excluded, only the two other siblings kept")
	assert.NotContains(t, out, "(this)", "self must not appear in the sibling list at all")
	assert.Contains(t, out, "+@"+editorTestApprover, "expected the addition applying the edit would make, inline")
	assert.NotContains(t, out, "-@", "applying to a sibling never removes anyone, so no removal is previewed")
}

func TestReviewerEditorWidget_Render_HeightIsStableAcrossPanelsAndRemoval(t *testing.T) {
	siblings := []domain.MergeRequest{focusedMR(), siblingMR(20, editorTestOther), siblingMR(30)}
	w := newTestReviewerEditor(siblings, nil)
	want := layoutShape(w.render())

	w.panel = reviewerEditorPanelSiblings
	assert.Len(t, layoutShape(w.render()), len(want), "switching to the siblings panel must not resize the modal")

	w.panel = reviewerEditorPanelReviewers
	updated, _ := w.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	w = updated.(*reviewerEditorWidget)
	assert.Len(t, layoutShape(w.render()), len(want), "removing a reviewer must not resize the modal")

	updated, _ = w.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	w = updated.(*reviewerEditorWidget)
	assert.Len(t, layoutShape(w.render()), len(want), "entering search must not resize the modal")
}

// layoutShape returns the display width of each line of s.
func layoutShape(s string) []int {
	lines := strings.Split(s, "\n")
	widths := make([]int, len(lines))
	for i, l := range lines {
		widths[i] = lip.Width(l)
	}
	return widths
}

func TestReviewerEditorWidget_RenderSiblings_DetailsPaneShowsFocusedTitle(t *testing.T) {
	siblings := []domain.MergeRequest{focusedMR(), siblingMR(20, editorTestOther)}
	w := newTestReviewerEditor(siblings, nil)
	w.panel = reviewerEditorPanelSiblings

	out := w.render()

	assert.Contains(t, out, siblings[1].Title, "expected the focused sibling's title in the details pane below the list")
}

func TestNewBatchPreviewWidget_PinsFocusedRowAndLeavesSiblingsOptIn(t *testing.T) {
	focused := focusedMR()
	siblings := []domain.MergeRequest{siblingMR(20, editorTestApprover), siblingMR(30, editorTestOther)}
	staged := []stagedReviewer{{Username: editorTestApprover, IsApprover: true}}

	w := newBatchPreviewWidget(staged, staged, siblings, focused, nil, Styles{}, DefaultBatchPreviewKeyMap)

	require.Len(t, w.rows, 3, "expected the focused MR plus its two siblings as rows")
	assert.True(t, w.rows[0].focused, "expected the focused MR pinned as rows[0]")
	assert.True(t, w.rows[0].included, "the focused MR is always written")
	assert.False(t, w.rows[0].hasChange, "staged matches focused's current reviewers, so no change")
	assert.False(t, w.rows[1].included, "siblings start unchecked: applying to them is opt-in")
	assert.False(t, w.rows[2].included, "siblings start unchecked: applying to them is opt-in")
	assert.False(t, w.rows[1].hasChange, "sibling already has the staged approver, so the union adds nothing")
	assert.True(t, w.rows[2].hasChange, "sibling lacks the staged approver, so the union adds it")
}

func TestBatchPreviewWidget_CollectTargets_IncludesFocusedOnlyWhenChanged(t *testing.T) {
	focused := focusedMR() // current reviewer/approver: editorTestApprover

	unchanged := newBatchPreviewWidget(
		[]stagedReviewer{{Username: editorTestApprover, IsApprover: true}}, nil,
		nil, focused, nil, Styles{}, DefaultBatchPreviewKeyMap,
	)
	assert.Empty(t, unchanged.collectTargets(), "no siblings and no change to focused MR means nothing to write")

	changed := newBatchPreviewWidget(
		[]stagedReviewer{{Username: editorTestOther, IsApprover: true}}, nil,
		nil, focused, nil, Styles{}, DefaultBatchPreviewKeyMap,
	)
	targets := changed.collectTargets()
	require.Len(t, targets, 1, "expected focused MR's own changed edit to be included even with no siblings")
	assert.Equal(t, focused.IID, targets[0].IID)
}

func TestBatchPreviewWidget_CollectTargets_SiblingsJoinOnlyWhenTicked(t *testing.T) {
	focused := focusedMR()
	sib := siblingMR(20, editorTestApprover)
	staged := []stagedReviewer{{Username: editorTestOther, IsApprover: true}}
	w := newBatchPreviewWidget(staged, nil, []domain.MergeRequest{sib}, focused, nil, Styles{}, DefaultBatchPreviewKeyMap)

	require.Len(t, w.collectTargets(), 1, "only the focused MR is written until a sibling is ticked")

	updated, _ := w.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	updated, _ = updated.Update(tea.KeyPressMsg{Text: " ", Code: ' '})
	w2 := updated.(*batchPreviewWidget)

	assert.Len(t, w2.collectTargets(), 2, "the ticked sibling joins the focused MR as a write target")
}

func TestBatchPreviewWidget_Confirm_CarriesBaselineAndFocusedMR(t *testing.T) {
	focused := focusedMR()
	baseline := []stagedReviewer{{Username: editorTestApprover, IsApprover: true}}
	staged := []stagedReviewer{{Username: editorTestOther}}
	w := newBatchPreviewWidget(staged, baseline, nil, focused, nil, Styles{}, DefaultBatchPreviewKeyMap)

	_, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg, ok := cmd().(BatchPreviewConfirmedMsg)

	require.True(t, ok, "expected BatchPreviewConfirmedMsg, got %T", cmd())
	assert.Equal(t, baseline, msg.Baseline)
	assert.Equal(t, focused.Key(), msg.FocusedMR.Key())
}

// previewWithSibling builds a preview over the focused MR (reviewer+approver alice) and
// one sibling (reviewer+approver carol), staging bob as a plain reviewer.
func previewWithSibling() *batchPreviewWidget {
	staged := []stagedReviewer{{Username: editorTestOther}}
	baseline := []stagedReviewer{{Username: editorTestApprover, IsApprover: true}}
	return newBatchPreviewWidget(
		staged, baseline, []domain.MergeRequest{siblingMR(20, "carol")}, focusedMR(), nil,
		Styles{}, DefaultBatchPreviewKeyMap,
	)
}

func TestBatchPreviewWidget_Render_ShowsTitlesAndEachRowsOwnDiff(t *testing.T) {
	w := previewWithSibling()
	sib := siblingMR(20, "carol")

	out := w.render()

	assert.Contains(t, out, focusedMR().Title, "expected the focused MR's own title shown, not just its siblings'")
	assert.Contains(t, out, "(this)", "expected the focused row marked as non-selectable")
	assert.Contains(t, out, sib.Title, "expected the sibling's MR title shown in the preview row")
	assert.Contains(t, out, "-@"+editorTestApprover+" +@"+editorTestOther,
		"expected the focused row's diff (alice dropped, bob added) inline, no space after the sign")
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, sib.Title) {
			assert.Contains(t, line, "+@"+editorTestOther,
				"expected the sibling's addition of bob on its own row, even though it is not ticked")
		}
	}
	assert.NotContains(t, out, "-@carol", "applying to a sibling never removes its own reviewers")
}

func TestBatchPreviewWidget_Toggle_FocusedRowIsNotSelectable(t *testing.T) {
	w := previewWithSibling()

	updated, _ := w.Update(tea.KeyPressMsg{Text: " ", Code: ' '})
	w2 := updated.(*batchPreviewWidget)

	assert.True(t, w2.rows[0].included, "expected the focused row to stay included — it is not selectable")
}

func TestBatchPreviewWidget_Layout_IsStableWhenTogglingAndMoving(t *testing.T) {
	w := previewWithSibling()
	want := layoutShape(w.render())

	// Move onto the sibling row, tick it, untick it: nothing may resize or shift.
	steps := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"move to sibling", tea.KeyPressMsg{Text: "j", Code: 'j'}},
		{"tick sibling", tea.KeyPressMsg{Text: " ", Code: ' '}},
		{"untick sibling", tea.KeyPressMsg{Text: " ", Code: ' '}},
		{"move back to focused", tea.KeyPressMsg{Text: "k", Code: 'k'}},
	}
	var model tea.Model = w
	for _, step := range steps {
		model, _ = model.Update(step.key)
		got := layoutShape(model.(*batchPreviewWidget).render())
		assert.Equal(t, want, got, "layout (line count and widths) must not change on %q", step.name)
	}
}

func TestBatchPreviewWidget_Render_DetailsPaneShowsQualifiedDiffForCursorRow(t *testing.T) {
	w := previewWithSibling()

	out := w.render()
	assert.Contains(t, out, "+@"+editorTestOther+" (reviewer)",
		"expected the cursor-focused row's qualified diff in the details pane")
	assert.Contains(t, out, "-@"+editorTestApprover+" (approver)",
		"expected the cursor-focused row's qualified diff in the details pane")

	// Move down to the sibling row: the details pane should now describe it instead.
	updated, _ := w.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	out2 := updated.(*batchPreviewWidget).render()
	assert.Contains(t, out2, "+@"+editorTestOther+" (reviewer)", "expected the sibling row's qualified diff once focused")
	assert.NotContains(t, out2, "(approver)", "applying to a sibling never removes an approver")
}

// --- reviewerWriteDiff dedup ---

func TestReviewerWriteDiff_ApproverAdditionDropsRedundantReviewerEntry(t *testing.T) {
	mr := siblingMR(20) // no reviewers yet
	staged := []stagedReviewer{{Username: editorTestApprover, IsApprover: true}}

	reviewersAdded, reviewersRemoved, approversAdded, approversRemoved := reviewerWriteDiff(staged, mr, false)

	assert.Empty(t, reviewersAdded, "a brand new approver must not also show as a plain reviewer addition")
	assert.Empty(t, reviewersRemoved)
	assert.Equal(t, []string{editorTestApprover}, approversAdded)
	assert.Empty(t, approversRemoved)
}

func TestReviewerWriteDiff_ApproverRemovalDropsRedundantReviewerEntry(t *testing.T) {
	mr := siblingMR(20, editorTestApprover) // currently a reviewer and approver
	var staged []stagedReviewer             // removed entirely

	reviewersAdded, reviewersRemoved, approversAdded, approversRemoved := reviewerWriteDiff(staged, mr, false)

	assert.Empty(t, reviewersAdded)
	assert.Empty(t, approversAdded)
	assert.Equal(t, []string{editorTestApprover}, approversRemoved)
	assert.Empty(t, reviewersRemoved, "a removed approver must not also show as a plain reviewer removal")
}

func TestReviewerWriteDiff_UnionReportsOnlyAdditions(t *testing.T) {
	mr := siblingMR(20, editorTestApprover) // currently a reviewer and approver
	staged := []stagedReviewer{{Username: editorTestOther, IsApprover: true}}

	reviewersAdded, reviewersRemoved, approversAdded, approversRemoved := reviewerWriteDiff(staged, mr, true)

	assert.Empty(t, reviewersAdded, "a new approver is reported as an approver addition only")
	assert.Equal(t, []string{editorTestOther}, approversAdded)
	assert.Empty(t, reviewersRemoved, "a union never removes a reviewer")
	assert.Empty(t, approversRemoved, "a union never clears an approver flag")
}

func TestReviewerWriteDiff_UnionNeverClearsApproverFlag(t *testing.T) {
	mr := siblingMR(20, editorTestApprover)
	staged := []stagedReviewer{{Username: editorTestApprover}} // staged as a plain reviewer

	reviewersAdded, reviewersRemoved, approversAdded, approversRemoved := reviewerWriteDiff(staged, mr, true)

	assert.Empty(t, reviewersAdded)
	assert.Empty(t, reviewersRemoved)
	assert.Empty(t, approversAdded)
	assert.Empty(t, approversRemoved)
}

// --- Shared reviewer-write use case (mrr-arch-improve-2026-08-j83.3) ---

func TestMakeReviewerWriteCmd_ReusesSeededKnownIDs(t *testing.T) {
	// A staged reviewer with an unresolved UserID must still avoid a
	// GetProjectMembers call when the caller already seeded knownIDs for that
	// username — this is the fix for the divergence where the batch-write path
	// used to always start knownIDs empty, unlike the single-edit save path.
	target := siblingMR(20) // no reviewers yet
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchMR(mock.Anything, int64(100), int64(20)).Return(target, nil).Twice() // live read + refetch
	src.EXPECT().SetReviewers(mock.Anything, int64(100), int64(20), []int64{1}).Return(nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, int64(100), int64(20), []int64{1}).Return(nil).Once()

	staged := []stagedReviewer{{Username: editorTestApprover, IsApprover: true}} // UserID unresolved
	knownIDs := map[string]int64{editorTestApprover: 1}

	cmd := makeReviewerWriteCmd(context.Background(), src, target, staged, nil, mrsvc.ReviewerWriteUnion, knownIDs)
	result := cmd()
	msg, ok := result.(ReviewersSavedMsg)
	require.True(t, ok, "expected ReviewersSavedMsg, got %T", result)
	require.NoError(t, msg.Err)
	// No GetProjectMembers expectation was registered above; an unexpected
	// call on a mockery mock panics immediately, so the absence of a panic
	// here is what proves it wasn't called.
}

func TestMakeReviewerWriteCmd_UnionKeepsTargetsOwnReviewersAndApprovers(t *testing.T) {
	// A non-focused target keeps the reviewers and approvers it already has: the
	// staged edit is unioned into what GitLab reports for that MR, not written
	// over it.
	target := siblingMR(20, editorTestApprover) // approver: editorTestApprover
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchMR(mock.Anything, int64(100), int64(20)).Return(target, nil).Twice() // live read + refetch
	src.EXPECT().SetReviewers(mock.Anything, int64(100), int64(20), []int64{1, 2}).Return(nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, int64(100), int64(20), []int64{1, 2}).Return(nil).Once()

	// The staged approver is not on the target yet; the target's own approver is not staged.
	staged := []stagedReviewer{{Username: editorTestOther, IsApprover: true, UserID: 2}}
	knownIDs := map[string]int64{editorTestApprover: 1}

	cmd := makeReviewerWriteCmd(context.Background(), src, target, staged, nil, mrsvc.ReviewerWriteUnion, knownIDs)
	result := cmd()
	msg, ok := result.(ReviewersSavedMsg)
	require.True(t, ok, "expected ReviewersSavedMsg, got %T", result)
	require.NoError(t, msg.Err)
	assert.True(t, msg.ApproversChanged, "expected the new approver to count as an approver change")
}
