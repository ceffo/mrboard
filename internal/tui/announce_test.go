package tui

import (
	"context"
	"slices"
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

// discoveredReqFor is claimReqFor for a set found on the first look at mr.
func discoveredReqFor(mr domain.MergeRequest, prior []string) mrsvc.ClaimRequest {
	req := claimReqFor(mr, prior)
	req.Discovered = true
	return req
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
	f.claims.EXPECT().Claim(mock.Anything, discoveredReqFor(mr, nil)).Return(mrsvc.ClaimOutcome{Owned: true}, nil)
	f.notifier.EXPECT().NotifyApproverChange(mock.Anything, mock.Anything, mock.Anything).Return(nil)

	res := f.refresh(mr)
	require.Len(t, res, 1)
	assert.True(t, res[0].Announced)
}

func TestApproverAnnounce_OldMRFirstSight_PriorIsItsCurrentSet(t *testing.T) {
	f := newAnnounceFixture(t)
	mr := announceMR(72*time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, discoveredReqFor(mr, []string{editorTestApprover})).
		Return(mrsvc.ClaimOutcome{}, nil)

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
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	require.Len(t, f.refresh(mr), 1)

	assert.Empty(t, f.refresh(mr))
	assert.Empty(t, f.refresh(mr))
}

func TestApproverAnnounce_ChangeIsClaimedOnlyWhenSeenOnTwoRefreshes(t *testing.T) {
	f := newAnnounceFixture(t)
	first := announceMR(time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	require.Len(t, f.refresh(first), 1)

	changed := announceMR(time.Hour, editorTestApprover, editorTestOther)
	assert.Empty(t, f.refresh(changed), "a change seen once may be a stale read")

	f.claims.EXPECT().Claim(mock.Anything, claimReqFor(changed, []string{editorTestApprover})).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	assert.Len(t, f.refresh(changed), 1)
}

func TestApproverAnnounce_OnlyTheFirstSightIsMarkedDiscovered(t *testing.T) {
	f := newAnnounceFixture(t)
	var got []mrsvc.ClaimRequest
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req mrsvc.ClaimRequest) { got = append(got, req) }).
		Return(mrsvc.ClaimOutcome{}, nil).Times(2)

	f.refresh(announceMR(time.Hour, editorTestApprover))
	changed := announceMR(time.Hour, editorTestApprover, editorTestOther)
	f.refresh(changed)
	f.refresh(changed)

	require.Len(t, got, 2)
	assert.True(t, got[0].Discovered, "the first look at an MR finds its set without seeing it change")
	assert.False(t, got[1].Discovered, "a change seen happening is not a discovery")
}

func TestApproverAnnounce_ChangeThatRevertsBeforeConfirmation_IsDropped(t *testing.T) {
	f := newAnnounceFixture(t)
	original := announceMR(time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
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
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	require.Len(t, f.refresh(first), 1)

	changed := announceMR(time.Hour, editorTestApprover, editorTestOther)
	require.Empty(t, f.refresh(changed))
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).Return(mrsvc.ClaimOutcome{}, assert.AnError).Once()
	res := f.refresh(changed)
	require.Len(t, res, 1)
	require.Error(t, res[0].Err)

	next, _ := f.m.handleApproverAnnounceResult(res[0])
	f.m = next.(Model)

	require.Empty(t, f.refresh(changed), "after a failure the change is pending again")
	f.claims.EXPECT().Claim(mock.Anything, claimReqFor(changed, []string{editorTestApprover})).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	assert.Len(t, f.refresh(changed), 1)
}

func TestApproverAnnounce_NotPermitted_IsNotRetried(t *testing.T) {
	f := newAnnounceFixture(t)
	first := announceMR(time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	require.Len(t, f.refresh(first), 1)

	changed := announceMR(time.Hour, editorTestApprover, editorTestOther)
	require.Empty(t, f.refresh(changed))
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).Return(mrsvc.ClaimOutcome{}, mrsvc.ErrClaimNotPermitted).Once()
	res := f.refresh(changed)
	require.Len(t, res, 1)

	next, _ := f.m.handleApproverAnnounceResult(res[0])
	f.m = next.(Model)

	assert.Empty(t, f.refresh(changed), "a refused write cannot succeed on retry")
	assert.Empty(t, f.refresh(changed))
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
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	require.Len(t, f.refresh(before), 1)

	// A project rule keeps making dave an approver although he is no reviewer.
	saved := before
	saved.Reviewers = []domain.ReviewerInfo{{Username: editorTestOther}}
	saved.ApproverRules = []domain.ApproverRule{
		{Name: editorTestRepoRule, Approvers: []string{testUserDave}, ApprovalsRequired: 1},
	}
	saved.SetManagedApprovers([]string{editorTestOther})
	f.claims.EXPECT().Claim(mock.Anything, mrsvc.ClaimRequest{
		ProjectID: 7, MRIID: 42, Approvers: []string{editorTestOther, testUserDave},
		Prior: []string{editorTestApprover}, Authoritative: true,
	}).Return(mrsvc.ClaimOutcome{Owned: true, Previous: []string{editorTestApprover}}, nil).Once()
	f.notifier.EXPECT().NotifyApproverChange(mock.Anything, mock.Anything, domain.ApproverChange{
		Gained: []string{editorTestOther, testUserDave}, Lost: []string{editorTestApprover},
		Source: domain.ApproverChangedByMrboard,
	}).Return(nil).Once()

	_, cmd := f.m.handleReviewersSaved(ReviewersSavedMsg{MR: saved, ApproversChanged: true})
	var announced int
	for _, r := range announceResults(cmd) {
		if r.Announced {
			announced++
		}
	}
	assert.Equal(t, 1, announced)

	confirmed := before
	confirmed.Approvers = []string{testUserDave, editorTestOther}
	f.m.dirty = dirtySet{}
	assert.Empty(t, f.refresh(confirmed), "the refresh that confirms the edit must not announce it again")
}

// A managed rule of alice and bob, where GitLab does not let bob approve (he
// pushed a commit): saving carol in claims alice and carol, never bob, and the
// read that confirms the save claims nothing.
func TestApproverAnnounce_EditorSave_NeverClaimsAnIneligibleManagedMember(t *testing.T) {
	const carol = "carol"
	f := newAnnounceFixture(t)
	before := announceMR(time.Hour)
	before.Reviewers = []domain.ReviewerInfo{{Username: editorTestApprover}, {Username: editorTestOther}}
	before.ApplyApproverRules([]domain.ApproverRule{{
		Name: domain.ManagedApproverRuleName, Approvers: []string{editorTestApprover},
		Members: []string{editorTestApprover, editorTestOther}, ApprovalsRequired: 2, Managed: true,
	}})
	f.m.allMRs = []domain.MergeRequest{before}
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	require.Len(t, f.refresh(before), 1)

	saved := before
	saved.Reviewers = append(slices.Clone(before.Reviewers), domain.ReviewerInfo{Username: carol})
	saved.SetManagedApprovers([]string{editorTestApprover, editorTestOther, carol})
	f.claims.EXPECT().Claim(mock.Anything, mrsvc.ClaimRequest{
		ProjectID: 7, MRIID: 42, Approvers: []string{editorTestApprover, carol},
		Prior: []string{editorTestApprover}, Authoritative: true,
	}).Return(mrsvc.ClaimOutcome{Owned: true}, nil).Once()
	f.notifier.EXPECT().NotifyApproverChange(mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	_, cmd := f.m.handleReviewersSaved(ReviewersSavedMsg{MR: saved, ApproversChanged: true})
	require.Len(t, announceResults(cmd), 1)
	flags := map[string]bool{}
	for _, r := range f.m.allMRs[0].Reviewers {
		flags[r.Username] = r.IsApprover
	}
	assert.Equal(t, map[string]bool{editorTestApprover: true, editorTestOther: false, carol: true}, flags,
		"bob gets no approver marker on the board")

	confirmed := before
	confirmed.Reviewers = saved.Reviewers
	confirmed.ApplyApproverRules([]domain.ApproverRule{{
		Name: domain.ManagedApproverRuleName, Approvers: []string{editorTestApprover, carol},
		Members: []string{editorTestApprover, editorTestOther, carol}, ApprovalsRequired: 3, Managed: true,
	}})
	f.m.dirty = dirtySet{}
	assert.Empty(t, f.refresh(confirmed), "the read confirming the save claims nothing")
	assert.Empty(t, f.refresh(confirmed))
}

func TestApproverAnnounce_ChangeFoundOnRefresh_IsAttributedElsewhere(t *testing.T) {
	f := newAnnounceFixture(t)
	first := announceMR(72*time.Hour, editorTestApprover)
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	require.Len(t, f.refresh(first), 1)

	changed := announceMR(72*time.Hour, editorTestOther)
	require.Empty(t, f.refresh(changed))
	f.claims.EXPECT().Claim(mock.Anything, claimReqFor(changed, []string{editorTestApprover})).
		Return(mrsvc.ClaimOutcome{Owned: true, Previous: []string{editorTestApprover}}, nil).Once()
	f.notifier.EXPECT().NotifyApproverChange(mock.Anything, mock.Anything, domain.ApproverChange{
		Gained: []string{editorTestOther}, Lost: []string{editorTestApprover},
		Source: domain.ApproverChangedElsewhere,
	}).Return(nil).Once()

	res := f.refresh(changed)
	require.Len(t, res, 1)
	assert.True(t, res[0].Announced)
}

func TestApproverAnnounce_ChangeInAnOptionalRuleOnly_MakesNoClaim(t *testing.T) {
	f := newAnnounceFixture(t)
	required := domain.ApproverRule{
		Name: editorTestRepoRule, Approvers: []string{editorTestApprover}, ApprovalsRequired: 1,
	}
	mr := announceMR(72 * time.Hour)
	mr.ApplyApproverRules([]domain.ApproverRule{required})
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	require.Len(t, f.refresh(mr), 1)

	changed := announceMR(72 * time.Hour)
	changed.ApplyApproverRules([]domain.ApproverRule{required, {
		Name: "optional", Approvers: []string{editorTestOther, testUserDave}, ApprovalsRequired: 1,
	}})
	assert.Empty(t, f.refresh(changed))
	assert.Empty(t, f.refresh(changed), "a member of a rule needing one of two approvals is not required")
}

// An editor save whose Teams delivery fails is released and re-claimed by the
// refreshes that confirm it: exactly one card goes out, and it reads "outside
// mrboard", because the re-claim is not the editor's (docs/adr/0014).
func TestApproverAnnounce_EditorSaveWithFailedDelivery_IsAnnouncedOnceByTheRefreshes(t *testing.T) {
	f := newAnnounceFixture(t)
	before := announceMR(72*time.Hour, editorTestApprover)
	f.m.allMRs = []domain.MergeRequest{before}
	f.claims.EXPECT().Claim(mock.Anything, mock.Anything).
		Return(mrsvc.ClaimOutcome{}, nil).Once()
	require.Len(t, f.refresh(before), 1)

	saved := before
	saved.Approvers = []string{editorTestOther}
	edited := mrsvc.ClaimRequest{
		ProjectID: 7, MRIID: 42, Approvers: []string{editorTestOther},
		Prior: []string{editorTestApprover}, Authoritative: true,
	}
	owned := mrsvc.ClaimOutcome{Owned: true, Previous: []string{editorTestApprover}}
	f.claims.EXPECT().Claim(mock.Anything, edited).Return(owned, nil).Once()
	f.notifier.EXPECT().NotifyApproverChange(mock.Anything, mock.Anything, mock.MatchedBy(
		func(c domain.ApproverChange) bool { return c.Source == domain.ApproverChangedByMrboard },
	)).Return(assert.AnError).Once()
	f.claims.EXPECT().Release(mock.Anything, int64(7), int64(42), []string{editorTestOther}).Return(nil).Once()

	res := announceResults(f.m.announceEditedApprovers(saved, before.Approvers))
	require.Len(t, res, 1)
	require.Error(t, res[0].Err)
	next, _ := f.m.handleApproverAnnounceResult(res[0])
	f.m = next.(Model)

	var delivered []domain.ApproverChange
	f.claims.EXPECT().Claim(mock.Anything, claimReqFor(saved, []string{editorTestApprover})).
		Return(owned, nil).Once()
	f.notifier.EXPECT().NotifyApproverChange(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ domain.MergeRequest, c domain.ApproverChange) {
			delivered = append(delivered, c)
		}).Return(nil).Once()

	assert.Empty(t, f.refresh(saved), "the first refresh only sees the change once")
	res = f.refresh(saved)
	require.Len(t, res, 1)
	assert.True(t, res[0].Announced)
	assert.Equal(t, []domain.ApproverChange{{
		Gained: []string{editorTestOther}, Lost: []string{editorTestApprover},
		Source: domain.ApproverChangedElsewhere,
	}}, delivered)
	assert.Empty(t, f.refresh(saved), "a later refresh announces nothing more")
}
