package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

func newTestBoard() boardWidget {
	return newBoardWidget(Styles{}, 80, 24, IssueTypeIconResolver{})
}

// TestBoardWidget_SetMRs_SameIIDDifferentProject_NoCollision is the bug
// TryRestoreFocus had before docs/adr/0005: it matched on IID alone, so two
// MRs from different projects sharing an IID would collide.
func TestBoardWidget_SetMRs_SameIIDDifferentProject_NoCollision(t *testing.T) {
	b := newTestBoard()
	mrA := domain.MergeRequest{ProjectID: 1, IID: 5, Phase: domain.PhaseNeedsReview}
	mrB := domain.MergeRequest{ProjectID: 2, IID: 5, Phase: domain.PhaseNeedsReview}

	selected := b.SetMRs([]domain.MergeRequest{mrA, mrB}, mrB.Key())

	assert.Equal(t, mrB.Key(), selected)
	require.NotNil(t, b.FocusedMR())
	assert.Equal(t, mrB.ProjectID, b.FocusedMR().ProjectID, "must resolve to project 2's MR, not project 1's")
}

// TestBoardWidget_SetMRs_FollowsSelectedAcrossColumns confirms focus follows
// the selected MR when its phase (and therefore its column) changes.
func TestBoardWidget_SetMRs_FollowsSelectedAcrossColumns(t *testing.T) {
	b := newTestBoard()
	mr := domain.MergeRequest{ProjectID: 1, IID: 7, Phase: domain.PhaseNeedsReview}
	selected := b.SetMRs([]domain.MergeRequest{mr}, domain.MRKey{})
	require.Equal(t, mr.Key(), selected)

	moved := mr
	moved.Phase = domain.PhaseReadyToMerge
	selected = b.SetMRs([]domain.MergeRequest{moved}, selected)

	assert.Equal(t, mr.Key(), selected)
	require.NotNil(t, b.FocusedMR())
	assert.Equal(t, domain.PhaseReadyToMerge, b.FocusedMR().Phase, "focus must follow the card to its new column")
}

// TestBoardWidget_SetMRs_AbsentMR_FocusLandsOnCardAtSameIndex covers the
// fallback when the selected MR disappears (merged/closed/filtered) but its
// column still has cards: focus stays at the same row index.
func TestBoardWidget_SetMRs_AbsentMR_FocusLandsOnCardAtSameIndex(t *testing.T) {
	b := newTestBoard()
	mr1 := domain.MergeRequest{ProjectID: 1, IID: 1, Phase: domain.PhaseNeedsReview}
	mr2 := domain.MergeRequest{ProjectID: 1, IID: 2, Phase: domain.PhaseNeedsReview}
	mr3 := domain.MergeRequest{ProjectID: 1, IID: 3, Phase: domain.PhaseNeedsReview}
	selected := b.SetMRs([]domain.MergeRequest{mr1, mr2, mr3}, domain.MRKey{})
	require.Equal(t, mr1.Key(), selected, "initial focus lands on the first card")

	b.MoveDown()
	selected = mr2.Key()

	// mr2 is removed (e.g. merged); mr3 shifts into its row index.
	selected = b.SetMRs([]domain.MergeRequest{mr1, mr3}, selected)

	assert.Equal(t, mr3.Key(), selected, "focus should land on the card that took mr2's index")
}

// TestBoardWidget_SetMRs_AbsentMR_ColumnEmpty_FallsBackToFirstNonEmpty covers
// the fallback when the selected MR's column becomes empty entirely.
func TestBoardWidget_SetMRs_AbsentMR_ColumnEmpty_FallsBackToFirstNonEmpty(t *testing.T) {
	b := newTestBoard()
	mrDraft := domain.MergeRequest{ProjectID: 1, IID: 1, Phase: domain.PhaseDraft}
	mrReview := domain.MergeRequest{ProjectID: 1, IID: 2, Phase: domain.PhaseNeedsReview}
	selected := b.SetMRs([]domain.MergeRequest{mrDraft, mrReview}, domain.MRKey{})
	require.Equal(t, mrDraft.Key(), selected)

	// mrDraft is removed; its column (Draft) becomes empty.
	selected = b.SetMRs([]domain.MergeRequest{mrReview}, selected)

	assert.Equal(t, mrReview.Key(), selected, "expected fallback to the first non-empty column")
}
