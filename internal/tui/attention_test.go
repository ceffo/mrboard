package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

func TestAttentionKeys(t *testing.T) {
	mine := domain.MergeRequest{ProjectID: 1, IID: 1, Author: "me", Phase: domain.PhaseReadyToMerge}
	waiting := domain.MergeRequest{ProjectID: 1, IID: 2, Author: "me", Phase: domain.PhaseNeedsReview}
	others := domain.MergeRequest{ProjectID: 1, IID: 3, Author: "x", Phase: domain.PhaseReadyToMerge}

	got := attentionKeys([]domain.MergeRequest{mine, waiting, others}, "me")

	assert.Equal(t, map[domain.MRKey]bool{mine.Key(): true}, got)
}

func TestAttentionKeys_NoUserOrNothingNeeded(t *testing.T) {
	mr := domain.MergeRequest{ProjectID: 1, IID: 1, Author: "me", Phase: domain.PhaseReadyToMerge}

	assert.Nil(t, attentionKeys([]domain.MergeRequest{mr}, ""))
	assert.Nil(t, attentionKeys(nil, "me"))
}

func TestBoardSetMRs_MarksAttentionCards(t *testing.T) {
	needs := domain.MergeRequest{ProjectID: 1, IID: 1, Author: "me", Phase: domain.PhaseReadyToMerge}
	waits := domain.MergeRequest{ProjectID: 1, IID: 2, Author: "me", Phase: domain.PhaseNeedsReview}
	b := newBoardWidget(
		NewStyles(LoadThemeByName("default"), true), defaultBoardWidth, defaultBoardHeight,
		IssueTypeIconResolver{}, domain.NewTicketKeyMatcher(false),
	)

	b.SetAttention(attentionKeys([]domain.MergeRequest{needs, waits}, "me"))
	b.SetMRs([]domain.MergeRequest{needs, waits}, domain.MRKey{})

	ready := b.columns[domain.PhaseReadyToMerge].cards
	review := b.columns[domain.PhaseNeedsReview].cards
	require.Len(t, ready, 1)
	require.Len(t, review, 1)
	assert.True(t, ready[0].attention)
	assert.False(t, review[0].attention)
}
