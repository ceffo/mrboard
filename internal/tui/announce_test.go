package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	domainmocks "github.com/ceffo/mrboard/internal/domain/mocks"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

const announceWindow = 24 * time.Hour

type announceFixture struct {
	m        Model
	claims   *mocks.MockApproverClaims
	notifier *domainmocks.MockNotifier
}

func newAnnounceFixture(t *testing.T) announceFixture {
	t.Helper()
	claims := mocks.NewMockApproverClaims(t)
	notifier := domainmocks.NewMockNotifier(t)
	m := New(context.Background(), &config.Config{}, mocks.NewMockMergeRequestSource(t), noopStore{},
		noopSnapshotStore{}, notifier, nil, nil, nil, "dev", Options{}).
		WithApproverAnnouncements(claims, announceWindow)
	return announceFixture{m: m, claims: claims, notifier: notifier}
}

func announceMR(age time.Duration, approvers ...string) domain.MergeRequest {
	return domain.MergeRequest{
		ID: 1, IID: 42, ProjectID: 7, ProjectPath: editorTestRepo,
		CreatedAt: time.Now().Add(-age), Approvers: approvers,
	}
}

func claimReqFor(mr domain.MergeRequest, prior []string) mrsvc.ClaimRequest {
	return mrsvc.ClaimRequest{
		ProjectID: int64(mr.ProjectID), MRIID: int64(mr.IID),
		Approvers: domain.NormalizeApprovers(mr.Approvers), Prior: prior,
	}
}

// announceResults executes cmd, flattening a batch, and returns the announce results.
func announceResults(cmd tea.Cmd) []approverAnnounceResultMsg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []approverAnnounceResultMsg
		for _, c := range msg {
			out = append(out, announceResults(c)...)
		}
		return out
	case approverAnnounceResultMsg:
		return []approverAnnounceResultMsg{msg}
	}
	return nil
}

func (f *announceFixture) refresh(mrs ...domain.MergeRequest) []approverAnnounceResultMsg {
	f.m.allMRs = mrs
	return announceResults(f.m.makeApproverAnnounceCmds())
}

func TestApproverAnnounce_NewMRWithApprovers_IsClaimedWithEmptyPrior(t *testing.T) {
	f := newAnnounceFixture(t)
	mr := announceMR(time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, claimReqFor(mr, nil)).Return(true, nil)
	f.notifier.EXPECT().Notify(mock.Anything, mock.Anything).Return(nil)

	res := f.refresh(mr)
	require.Len(t, res, 1)
	assert.True(t, res[0].Announced)
}

func TestApproverAnnounce_OldMRFirstSight_PriorIsItsCurrentSet(t *testing.T) {
	f := newAnnounceFixture(t)
	mr := announceMR(72*time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, claimReqFor(mr, []string{editorTestApprover})).Return(false, nil)

	res := f.refresh(mr)
	require.Len(t, res, 1)
	assert.False(t, res[0].Announced)
}

func TestApproverAnnounce_NoApproversAndNoHistory_MakesNoCalls(t *testing.T) {
	f := newAnnounceFixture(t)
	assert.Empty(t, f.refresh(announceMR(time.Hour)))
}

func TestApproverAnnounce_UnchangedSet_MakesNoFurtherCalls(t *testing.T) {
	f := newAnnounceFixture(t)
	mr := announceMR(time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).Return(false, nil).Once()
	require.Len(t, f.refresh(mr), 1)

	assert.Empty(t, f.refresh(mr))
	assert.Empty(t, f.refresh(mr))
}

func TestApproverAnnounce_ChangeIsClaimedOnlyWhenSeenOnTwoRefreshes(t *testing.T) {
	f := newAnnounceFixture(t)
	first := announceMR(time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).Return(false, nil).Once()
	require.Len(t, f.refresh(first), 1)

	changed := announceMR(time.Hour, editorTestApprover, editorTestOther)
	assert.Empty(t, f.refresh(changed), "a change seen once may be a stale read")

	f.claims.EXPECT().Claim(mock.Anything, claimReqFor(changed, []string{editorTestApprover})).Return(false, nil).Once()
	assert.Len(t, f.refresh(changed), 1)
}

func TestApproverAnnounce_ChangeThatRevertsBeforeConfirmation_IsDropped(t *testing.T) {
	f := newAnnounceFixture(t)
	original := announceMR(time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).Return(false, nil).Once()
	require.Len(t, f.refresh(original), 1)

	assert.Empty(t, f.refresh(announceMR(time.Hour, editorTestOther)))
	assert.Empty(t, f.refresh(original), "the stale read went away; nothing was ever claimed")
	assert.Empty(t, f.refresh(original))
}

func TestApproverAnnounce_UnconfirmedLocalWrite_IsSkipped(t *testing.T) {
	f := newAnnounceFixture(t)
	mr := announceMR(time.Hour, editorTestApprover)
	f.m.dirty.Mark(mr.Key(), time.Now())

	assert.Empty(t, f.refresh(mr))
}

func TestApproverAnnounce_FailureRestoresThePreviousSetSoTheChangeIsRetried(t *testing.T) {
	f := newAnnounceFixture(t)
	first := announceMR(time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).Return(false, nil).Once()
	require.Len(t, f.refresh(first), 1)

	changed := announceMR(time.Hour, editorTestApprover, editorTestOther)
	require.Empty(t, f.refresh(changed))
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).Return(false, assert.AnError).Once()
	res := f.refresh(changed)
	require.Len(t, res, 1)
	require.Error(t, res[0].Err)

	next, _ := f.m.handleApproverAnnounceResult(res[0])
	f.m = next.(Model)

	require.Empty(t, f.refresh(changed), "after a failure the change is pending again")
	f.claims.EXPECT().Claim(mock.Anything, claimReqFor(changed, []string{editorTestApprover})).Return(false, nil).Once()
	assert.Len(t, f.refresh(changed), 1)
}

func TestApproverAnnounce_Disabled_MakesNoCalls(t *testing.T) {
	m := New(context.Background(), &config.Config{}, mocks.NewMockMergeRequestSource(t), noopStore{},
		noopSnapshotStore{}, domainmocks.NewMockNotifier(t), nil, nil, nil, "dev", Options{})
	m.allMRs = []domain.MergeRequest{announceMR(time.Hour, editorTestApprover)}

	assert.Nil(t, m.makeApproverAnnounceCmds())
}

func TestApproverAnnounce_EditorSave_IsClaimedAuthoritativelyAndNotRepeatedOnRefresh(t *testing.T) {
	f := newAnnounceFixture(t)
	before := announceMR(time.Hour, editorTestApprover)
	f.m.allMRs = []domain.MergeRequest{before}
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).Return(false, nil).Once()
	require.Len(t, f.refresh(before), 1)

	saved := before
	saved.Reviewers = []domain.ReviewerInfo{{Username: editorTestOther, IsApprover: true}}
	f.claims.EXPECT().Claim(mock.Anything, mrsvc.ClaimRequest{
		ProjectID: 7, MRIID: 42, Approvers: []string{editorTestOther},
		Prior: []string{editorTestApprover}, Authoritative: true,
	}).Return(true, nil).Once()
	f.notifier.EXPECT().Notify(mock.Anything, mock.Anything).Return(nil).Once()

	_, cmd := f.m.handleReviewersSaved(ReviewersSavedMsg{MR: saved, ApproversChanged: true})
	var announced int
	for _, r := range announceResults(cmd) {
		if r.Announced {
			announced++
		}
	}
	assert.Equal(t, 1, announced)

	confirmed := before
	confirmed.Approvers = []string{editorTestOther}
	f.m.dirty = dirtySet{}
	assert.Empty(t, f.refresh(confirmed), "the refresh that confirms the edit must not announce it again")
}
