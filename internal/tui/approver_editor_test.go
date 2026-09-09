package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
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
	src.EXPECT().GetProjectMembers(mock.Anything, int64(100)).
		Return([]domain.ProjectMember{{UserID: 1, Username: editorTestApprover}}, nil).Once()
	src.EXPECT().SetReviewers(mock.Anything, int64(100), int64(10), mock.Anything).Return(nil).Once()
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
	src.EXPECT().GetProjectMembers(mock.Anything, int64(100)).
		Return([]domain.ProjectMember{{UserID: 1, Username: editorTestApprover}}, nil).Once()
	src.EXPECT().SetReviewers(mock.Anything, int64(100), int64(10), mock.Anything).Return(nil).Once()
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

// --- Conflict rendering ---

func TestReviewerEditorWidget_RenderSiblings_OmitsSelfAndShowsInlineDiffOnlyForConflict(t *testing.T) {
	siblings := []domain.MergeRequest{
		focusedMR(),                       // self — excluded from the list entirely
		siblingMR(20, editorTestApprover), // same approvers — no diff
		siblingMR(30, editorTestOther),    // different approvers — inline diff
	}
	w := newTestReviewerEditor(siblings, nil)
	w.panel = reviewerEditorPanelSiblings

	out := w.render()

	assert.Len(t, w.siblings, 2, "expected self excluded, only the two other siblings kept")
	assert.NotContains(t, out, "(this)", "self must not appear in the sibling list at all")
	assert.Contains(t, out, "-@"+editorTestApprover+" +@"+editorTestOther,
		"expected the conflicting sibling's diff inline, removed before added, no space after the sign")
}

func TestReviewerEditorWidget_RenderSiblings_DetailsPaneShowsFocusedTitle(t *testing.T) {
	siblings := []domain.MergeRequest{focusedMR(), siblingMR(20, editorTestOther)}
	w := newTestReviewerEditor(siblings, nil)
	w.panel = reviewerEditorPanelSiblings

	out := w.render()

	assert.Contains(t, out, siblings[1].Title, "expected the focused sibling's title in the details pane below the list")
}

func TestNewBatchPreviewWidget_PinsFocusedRowAndMarksConflict(t *testing.T) {
	focused := focusedMR()
	siblings := []domain.MergeRequest{siblingMR(20, editorTestApprover), siblingMR(30, editorTestOther)}
	staged := []stagedReviewer{{Username: editorTestApprover, IsApprover: true}}

	w := newBatchPreviewWidget(staged, siblings, focused, nil, Styles{}, DefaultBatchPreviewKeyMap)

	require.Len(t, w.rows, 3, "expected the focused MR plus its two siblings as rows")
	assert.True(t, w.rows[0].focused, "expected the focused MR pinned as rows[0]")
	assert.False(t, w.rows[0].hasChange, "staged matches focused's current reviewers, so no change")
	assert.False(t, w.rows[1].conflict, "sibling with matching approvers must not be flagged as conflicting")
	assert.True(t, w.rows[2].conflict, "sibling with different approvers must be flagged as conflicting")
}

func TestBatchPreviewWidget_CollectTargets_IncludesFocusedOnlyWhenChanged(t *testing.T) {
	focused := focusedMR() // current reviewer/approver: editorTestApprover

	unchanged := newBatchPreviewWidget(
		[]stagedReviewer{{Username: editorTestApprover, IsApprover: true}},
		nil, focused, nil, Styles{}, DefaultBatchPreviewKeyMap,
	)
	assert.Empty(t, unchanged.collectTargets(), "no siblings and no change to focused MR means nothing to write")

	changed := newBatchPreviewWidget(
		[]stagedReviewer{{Username: editorTestOther, IsApprover: true}},
		nil, focused, nil, Styles{}, DefaultBatchPreviewKeyMap,
	)
	targets := changed.collectTargets()
	require.Len(t, targets, 1, "expected focused MR's own changed edit to be included even with no siblings")
	assert.Equal(t, focused.IID, targets[0].IID)
}

func TestBatchPreviewWidget_Render_ShowsFocusedRowTitleAndInlineDiffUnlessDeselected(t *testing.T) {
	focused := focusedMR()                                  // current reviewer+approver: alice
	sib := siblingMR(20, editorTestOther)                   // current reviewer+approver: bob
	staged := []stagedReviewer{{Username: editorTestOther}} // bob, as a plain reviewer (not approver)

	w := newBatchPreviewWidget(staged, []domain.MergeRequest{sib}, focused, nil, Styles{}, DefaultBatchPreviewKeyMap)

	out := w.render()
	assert.Contains(t, out, focused.Title, "expected the focused MR's own title shown, not just its siblings'")
	assert.Contains(t, out, "(this)", "expected the focused row marked as non-selectable")
	assert.Contains(t, out, sib.Title, "expected the sibling's MR title shown in the preview row")
	assert.Contains(t, out, "-@"+editorTestApprover+" +@"+editorTestOther,
		"expected the focused row's diff (alice dropped, bob added) inline, no space after the sign")
	assert.Contains(t, out, "-@"+editorTestOther,
		"expected the sibling row's diff (bob demoted from approver) inline")

	// The focused row is rows[0] and cursor starts there — toggling it must be a no-op.
	updated, _ := w.Update(tea.KeyPressMsg{Text: " ", Code: ' '})
	w2 := updated.(*batchPreviewWidget)
	assert.True(t, w2.rows[0].included, "expected the focused row to stay included — it is not selectable")

	// Move down to the sibling row and deselect it.
	updated, _ = w2.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	w3 := updated.(*batchPreviewWidget)
	updated, _ = w3.Update(tea.KeyPressMsg{Text: " ", Code: ' '})
	w4 := updated.(*batchPreviewWidget)
	require.False(t, w4.rows[1].included, "expected the sibling row to be deselected")

	out2 := w4.render()
	assert.NotContains(t, out2, "-@"+editorTestOther, "expected the sibling's diff hidden once its row is deselected")
	assert.Contains(t, out2, "-@"+editorTestApprover+" +@"+editorTestOther,
		"expected the focused row's own diff to remain — it is always applied, never deselected")
}

func TestBatchPreviewWidget_Render_DetailsPaneShowsQualifiedDiffUnlessDeselected(t *testing.T) {
	focused := focusedMR()                                  // current reviewer+approver: alice
	sib := siblingMR(20, editorTestOther)                   // current reviewer+approver: bob
	staged := []stagedReviewer{{Username: editorTestOther}} // bob, as a plain reviewer (not approver)

	w := newBatchPreviewWidget(staged, []domain.MergeRequest{sib}, focused, nil, Styles{}, DefaultBatchPreviewKeyMap)

	out := w.render()
	assert.Contains(t, out, "+@"+editorTestOther+" (reviewer)",
		"expected the cursor-focused row's qualified diff in the details pane")
	assert.Contains(t, out, "-@"+editorTestApprover+" (approver)",
		"expected the cursor-focused row's qualified diff in the details pane")

	// Move down to the sibling row: the details pane should now describe it instead.
	updated, _ := w.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	w2 := updated.(*batchPreviewWidget)
	out2 := w2.render()
	assert.Contains(t, out2, "-@"+editorTestOther+" (approver)", "expected the sibling row's qualified diff once focused")

	// Deselect the sibling row: its qualified diff must disappear from the pane.
	updated, _ = w2.Update(tea.KeyPressMsg{Text: " ", Code: ' '})
	w3 := updated.(*batchPreviewWidget)
	out3 := w3.render()
	assert.NotContains(t, out3, "(approver)", "expected the qualified diff hidden once its row is deselected")
}

// --- reviewerWriteDiff dedup ---

func TestReviewerWriteDiff_ApproverAdditionDropsRedundantReviewerEntry(t *testing.T) {
	mr := siblingMR(20) // no reviewers yet
	staged := []stagedReviewer{{Username: editorTestApprover, IsApprover: true}}

	reviewersAdded, reviewersRemoved, approversAdded, approversRemoved := reviewerWriteDiff(staged, mr)

	assert.Empty(t, reviewersAdded, "a brand new approver must not also show as a plain reviewer addition")
	assert.Empty(t, reviewersRemoved)
	assert.Equal(t, []string{editorTestApprover}, approversAdded)
	assert.Empty(t, approversRemoved)
}

func TestReviewerWriteDiff_ApproverRemovalDropsRedundantReviewerEntry(t *testing.T) {
	mr := siblingMR(20, editorTestApprover) // currently a reviewer and approver
	var staged []stagedReviewer             // removed entirely

	reviewersAdded, reviewersRemoved, approversAdded, approversRemoved := reviewerWriteDiff(staged, mr)

	assert.Empty(t, reviewersAdded)
	assert.Empty(t, approversAdded)
	assert.Equal(t, []string{editorTestApprover}, approversRemoved)
	assert.Empty(t, reviewersRemoved, "a removed approver must not also show as a plain reviewer removal")
}

// --- Shared reviewer-write use case (mrr-arch-improve-2026-08-j83.3) ---

func TestMakeReviewerWriteCmd_ReusesSeededKnownIDs(t *testing.T) {
	// A staged reviewer with an unresolved UserID must still avoid a
	// GetProjectMembers call when the caller already seeded knownIDs for that
	// username — this is the fix for the divergence where the batch-write path
	// used to always start knownIDs empty, unlike the single-edit save path.
	target := siblingMR(20, editorTestApprover)
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().SetReviewers(mock.Anything, int64(100), int64(20), []int64{1}).Return(nil).Once()
	src.EXPECT().FetchMR(mock.Anything, int64(100), int64(20)).Return(target, nil).Once()

	staged := []stagedReviewer{{Username: editorTestApprover, IsApprover: true}} // UserID unresolved
	knownIDs := map[string]int64{editorTestApprover: 1}

	cmd := makeReviewerWriteCmd(context.Background(), src, target, staged, knownIDs)
	result := cmd()
	msg, ok := result.(ReviewersSavedMsg)
	require.True(t, ok, "expected ReviewersSavedMsg, got %T", result)
	require.NoError(t, msg.Err)
	// No GetProjectMembers expectation was registered above; an unexpected
	// call on a mockery mock panics immediately, so the absence of a panic
	// here is what proves it wasn't called.
}

func TestMakeReviewerWriteCmd_DerivesOrigApproversFromTarget(t *testing.T) {
	// origApprovers must come from the target MR's own current reviewers, not
	// from any editor-side snapshot — each target in a batch has its own
	// approver set to compare against.
	target := siblingMR(20, editorTestApprover) // approver: editorTestApprover
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().SetReviewers(mock.Anything, int64(100), int64(20), mock.Anything).Return(nil).Once()
	src.EXPECT().SaveApprovers(mock.Anything, int64(100), int64(20), []int64{2}).Return(nil).Once()
	src.EXPECT().FetchMR(mock.Anything, int64(100), int64(20)).Return(target, nil).Once()

	// Staged approver differs from the target's own current approver.
	staged := []stagedReviewer{{Username: editorTestOther, IsApprover: true, UserID: 2}}

	cmd := makeReviewerWriteCmd(context.Background(), src, target, staged, nil)
	result := cmd()
	msg, ok := result.(ReviewersSavedMsg)
	require.True(t, ok, "expected ReviewersSavedMsg, got %T", result)
	require.NoError(t, msg.Err)
	assert.True(t, msg.ApproversChanged, "expected a change against the target's own approver set")
}
