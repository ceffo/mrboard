package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

func draftMR() domain.MergeRequest {
	return domain.MergeRequest{
		ID: 1, IID: 99, ProjectID: 7, Title: "Draft: fix(OD-1): add feature",
		Author: "olivia", ProjectPath: "org/delta", Phase: domain.PhaseDraft,
	}
}

func TestUndraftConfirmDialog_RunsUndraftOnYes(t *testing.T) {
	mr := draftMR()
	dlg := newUndraftConfirmDialog(mr, NewStyles(LoadThemeByName("default"), true), DefaultConfirmKeyMap)

	body := dlg.render()
	assert.Contains(t, body, "!99")
	assert.Contains(t, body, mr.Title)

	_, cmd := dlg.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	msgs := runCmds(t, batchCmds(t, cmd))

	assert.Contains(t, msgs, tea.Msg(dismissOverlayMsg{}))
	assert.Contains(t, msgs, tea.Msg(undraftRequestedMsg{MR: mr}))
}

// modelWithMR builds a board-state Model loaded with a single MR, focused by
// construction (the board focuses the first card of the first non-empty
// column on load).
func modelWithMR(t *testing.T, cfg *config.Config, mr domain.MergeRequest) (Model, *mocks.MockMergeRequestSource) {
	t.Helper()
	src := mocks.NewMockMergeRequestSource(t)
	m := New(context.Background(), cfg, src, noopStore{}, noopSnapshotStore{},
		nil, nil, nil, nil, "dev", Options{})
	next, _ := m.Update(FetchResultMsg{MRs: []domain.MergeRequest{mr}})
	return next.(Model), src
}

func TestHandleKeyBoard_Undraft_OpensConfirmForFocusedDraftMR(t *testing.T) {
	m, _ := modelWithMR(t, &config.Config{}, draftMR())
	require.Nil(t, m.confirm)

	next, _ := m.handleKeyBoard(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = next.(Model)

	require.NotNil(t, m.confirm, "U on a focused draft MR must open the confirm dialog")
}

func TestHandleKeyBoard_Undraft_NoOpForNonDraftMR(t *testing.T) {
	mr := draftMR()
	mr.Phase = domain.PhaseNeedsReview
	m, _ := modelWithMR(t, &config.Config{}, mr)

	next, _ := m.handleKeyBoard(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = next.(Model)

	assert.Nil(t, m.confirm, "U on a non-draft MR must be a silent no-op")
}

func TestHandleUndraftResult_MovesMRToNewColumnAndPreservesFocus(t *testing.T) {
	original := draftMR()
	m, _ := modelWithMR(t, &config.Config{}, original)
	require.Equal(t, original.Key(), m.Selected(), "the sole MR must be focused on load")

	updated := original
	updated.Phase = domain.PhaseNeedsReview

	next, _ := m.handleUndraftResult(UndraftResultMsg{MR: updated})
	m = next.(Model)

	require.Len(t, m.AllMRs(), 1)
	assert.Equal(t, domain.PhaseNeedsReview, m.AllMRs()[0].Phase, "undrafting must move the MR out of PhaseDraft")
	assert.Equal(t, updated.Key(), m.Selected(), "focus must stay on the undrafted MR across the column change")

	_, dirty := m.Dirty()[updated.Key()]
	assert.True(t, dirty, "a successful undraft must mark the MR dirty (docs/adr/0005)")
}

func TestHandleUndraftResult_Error_LeavesMRUnchanged(t *testing.T) {
	original := draftMR()
	m, _ := modelWithMR(t, &config.Config{}, original)

	next, _ := m.handleUndraftResult(UndraftResultMsg{MR: original, Err: assert.AnError})
	m = next.(Model)

	require.Len(t, m.AllMRs(), 1)
	assert.Equal(t, domain.PhaseDraft, m.AllMRs()[0].Phase, "a failed undraft must not change the MR's phase")
	_, dirty := m.Dirty()[original.Key()]
	assert.False(t, dirty, "a failed undraft must not mark the MR dirty")
}

// TestHandleUndraftResult_ChainsAutoAssign pins the "refactor to be reusable"
// requirement: a successful undraft reuses the exact same eligibility check
// and write the post-fetch pipeline uses (docs/adr/0009), via the shared
// tryAutoAssignReviewersCmd.
func TestHandleUndraftResult_ChainsAutoAssign(t *testing.T) {
	original := draftMR() // title carries ticket key "OD-1"
	cfg := &config.Config{AutoAssignReviewers: config.AutoAssignReviewers{Enabled: true}}
	m, src := modelWithMR(t, cfg, original)
	m.teamRoster = []domain.User{{ID: 1, Username: "olivia"}, {ID: 2, Username: "nina"}}

	updated := original
	updated.Phase = domain.PhaseNeedsReview

	src.EXPECT().SetReviewers(mock.Anything, int64(updated.ProjectID), int64(updated.IID), []int64{2}).
		Return(nil).Once()

	_, cmd := m.handleUndraftResult(UndraftResultMsg{MR: updated})
	runCmd(t, cmd)
	// SetReviewers's Once() expectation is asserted via t.Cleanup.
}
