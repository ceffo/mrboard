package demoadpt

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

const (
	userAda       = "ada"
	userGrace     = "grace"
	userLinus     = "linus"
	userMargaret  = "margaret"
	userKatherine = "katherine"

	reviewersFixture  = "reviewers"
	autoAssignFixture = "auto-assign"

	relatedTicket   = "REV-100"
	focusedProject  = 101
	focusedMRNumber = 501

	// reviewerEditorPageSize mirrors the editor's and the preview's visible rows.
	reviewerEditorPageSize = 8
)

// reviewersOf returns an MR's reviewer usernames and approver usernames, sorted.
func reviewersOf(mr domain.MergeRequest) (reviewers, approvers []string) {
	for _, r := range mr.Reviewers {
		reviewers = append(reviewers, r.Username)
		if r.IsApprover {
			approvers = append(approvers, r.Username)
		}
	}
	slices.Sort(reviewers)
	slices.Sort(approvers)
	return reviewers, approvers
}

func fetchMR(t *testing.T, a *Adapter, project, iid int) domain.MergeRequest {
	t.Helper()
	mr, err := a.MRSource().FetchMR(context.Background(), int64(project), int64(iid))
	require.NoError(t, err)
	return mr
}

func applyChange(t *testing.T, a *Adapter, project, iid int, change mrsvc.ReviewerChange) {
	t.Helper()
	_, _, err := mrsvc.ApplyReviewerChanges(
		context.Background(), a.MRSource(), int64(project), int64(iid), change, map[string]int64{})
	require.NoError(t, err)
}

// --- reviewers fixture ---

func TestReviewersFixture_RunsWithoutAutoAssign(t *testing.T) {
	s := newTestAdapterFor(t, reviewersFixture).Settings()

	assert.False(t, s.AutoAssignReviewers,
		"the board must not rewrite reviewers on its own while the editor scenarios run")
	assert.NotEmpty(t, s.Team, "the editor's set-team action needs a roster")
}

func TestReviewersFixture_RelatedGroupFillsMoreThanAScreen(t *testing.T) {
	a := newTestAdapterFor(t, reviewersFixture)
	km := domain.NewTicketKeyMatcher(false)

	related := 0
	for _, mr := range a.ds.all() {
		if km.ExtractFromTitle(mr.Title) == relatedTicket {
			related++
		}
	}

	assert.Greater(t, related, reviewerEditorPageSize, "both lists must scroll to be exercised")
}

func TestReviewersFixture_EveryRelatedMRIsInADifferentStateFromTheFocusedOne(t *testing.T) {
	a := newTestAdapterFor(t, reviewersFixture)
	km := domain.NewTicketKeyMatcher(false)
	focused := fetchMR(t, a, focusedProject, focusedMRNumber)
	want := reviewerSignature(focused)

	for _, mr := range a.ds.all() {
		if km.ExtractFromTitle(mr.Title) != relatedTicket || mr.Key() == focused.Key() {
			continue
		}
		// !502 is the one deliberate "nothing to change" sibling.
		assert.Equal(t, mr.IID == 502, slices.Equal(reviewerSignature(mr), want),
			"MR !%d: identical to the focused MR is only intended for !502", mr.IID)
	}
}

// reviewerSignature lists each reviewer with their approver flag and review
// progress, sorted, so two MRs compare equal only when nothing about their
// reviewers differs.
func reviewerSignature(mr domain.MergeRequest) []string {
	sig := make([]string, 0, len(mr.Reviewers))
	for _, r := range mr.Reviewers {
		sig = append(sig, fmt.Sprintf("%s/%t/%s", r.Username, r.IsApprover, r.State))
	}
	slices.Sort(sig)
	return sig
}

func TestReviewersFixture_UnionKeepsWhatEachRelatedMRAlreadyHas(t *testing.T) {
	staged := []mrsvc.ReviewerEdit{
		{Username: userGrace, IsApprover: true},
		{Username: userLinus},
		{Username: userKatherine, IsApprover: true},
	}
	cases := []struct {
		project, iid  int
		wantReviewers []string
		wantApprovers []string
	}{
		{101, 502, []string{userGrace, userKatherine, userLinus}, []string{userGrace, userKatherine}},
		{102, 503, []string{userGrace, userKatherine, userLinus}, []string{userGrace, userKatherine}},
		{103, 504, []string{userGrace, userKatherine, userLinus, userMargaret}, []string{userGrace, userKatherine}},
		{101, 505, []string{userGrace, userKatherine, userLinus}, []string{userGrace, userKatherine}},
		{102, 506, []string{userGrace, userKatherine, userLinus}, []string{userGrace, userKatherine}},
		{103, 507, []string{userGrace, userKatherine, userLinus}, []string{userGrace, userKatherine}},
		{101, 508, []string{userGrace, userKatherine, userLinus}, []string{userGrace, userKatherine}},
		{
			102, 509,
			[]string{"edsger", userGrace, userKatherine, userLinus, userMargaret},
			[]string{userGrace, userKatherine, userMargaret},
		},
		{103, 510, []string{"barbara", userGrace, userKatherine, userLinus}, []string{userGrace, userKatherine}},
		// linus stays an approver although the staged edit lists him as a plain reviewer.
		{101, 511, []string{userGrace, userKatherine, userLinus}, []string{userGrace, userKatherine, userLinus}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("!%d", tc.iid), func(t *testing.T) {
			a := newTestAdapterFor(t, reviewersFixture)

			applyChange(t, a, tc.project, tc.iid, mrsvc.ReviewerChange{Staged: staged, Mode: mrsvc.ReviewerWriteUnion})

			reviewers, approvers := reviewersOf(fetchMR(t, a, tc.project, tc.iid))
			assert.Equal(t, tc.wantReviewers, reviewers)
			assert.Equal(t, tc.wantApprovers, approvers)
		})
	}
}

func TestReviewersFixture_UnionPreservesReviewerProgress(t *testing.T) {
	a := newTestAdapterFor(t, reviewersFixture)
	staged := []mrsvc.ReviewerEdit{{Username: userKatherine, IsApprover: true}}

	applyChange(t, a, 101, 508, mrsvc.ReviewerChange{Staged: staged, Mode: mrsvc.ReviewerWriteUnion})

	mr := fetchMR(t, a, 101, 508)
	states := map[string]domain.ReviewerState{}
	for _, r := range mr.Reviewers {
		states[r.Username] = r.State
	}
	assert.Equal(t, domain.ReviewerApproved, states[userGrace], "an approval must survive a union write")
	assert.Equal(t, domain.ReviewerCommented, states[userLinus], "a comment must survive a union write")
	assert.Equal(t, domain.ReviewerNotStarted, states[userKatherine], "a newcomer starts as not started")
}

func TestReviewersFixture_EditAppliesOnlyTheUsersChange(t *testing.T) {
	a := newTestAdapterFor(t, reviewersFixture)
	baseline := []mrsvc.ReviewerEdit{{Username: userGrace, IsApprover: true}, {Username: userLinus}}
	staged := []mrsvc.ReviewerEdit{{Username: userGrace, IsApprover: true}, {Username: userKatherine}}

	applyChange(t, a, focusedProject, focusedMRNumber,
		mrsvc.ReviewerChange{Staged: staged, Baseline: baseline, Mode: mrsvc.ReviewerWriteEdit})

	reviewers, approvers := reviewersOf(fetchMR(t, a, focusedProject, focusedMRNumber))
	assert.Equal(t, []string{userGrace, userKatherine}, reviewers, "linus removed, katherine added")
	assert.Equal(t, []string{userGrace}, approvers)
}

func TestReviewersFixture_EditKeepsAReviewerAddedWhileTheEditorWasOpen(t *testing.T) {
	a := newTestAdapterFor(t, reviewersFixture)
	baseline := []mrsvc.ReviewerEdit{{Username: userGrace, IsApprover: true}, {Username: userLinus}}
	staged := append(slices.Clone(baseline), mrsvc.ReviewerEdit{Username: userKatherine})

	// A teammate adds margaret on the server after the editor opened.
	person := a.ds.people[userMargaret]
	live := fetchMR(t, a, focusedProject, focusedMRNumber)
	ids := []int64{person.UserID}
	for _, r := range live.Reviewers {
		ids = append(ids, a.ds.people[r.Username].UserID)
	}
	require.NoError(t, a.MRSource().SetReviewers(context.Background(), focusedProject, focusedMRNumber, ids))

	applyChange(t, a, focusedProject, focusedMRNumber,
		mrsvc.ReviewerChange{Staged: staged, Baseline: baseline, Mode: mrsvc.ReviewerWriteEdit})

	reviewers, _ := reviewersOf(fetchMR(t, a, focusedProject, focusedMRNumber))
	assert.Equal(t, []string{userGrace, userKatherine, userLinus, userMargaret}, reviewers,
		"the editor's stale view must not strip the reviewer who joined meanwhile")
}

func TestReviewersFixture_LongTitlesAndPathsArePresent(t *testing.T) {
	a := newTestAdapterFor(t, reviewersFixture)

	longest, longestPath := 0, 0
	for _, mr := range a.ds.all() {
		longest = max(longest, len(mr.Title))
		longestPath = max(longestPath, len(mr.ProjectPath))
	}

	assert.Greater(t, longest, 120, "need a title far wider than any row to exercise truncation")
	assert.Greater(t, longestPath, 50, "need a repository path far wider than the usual ones")
}

func TestReviewersFixture_HasAnMRWithNoRelatedMRs(t *testing.T) {
	a := newTestAdapterFor(t, reviewersFixture)
	km := domain.NewTicketKeyMatcher(false)

	byKey := map[string]int{}
	for _, mr := range a.ds.all() {
		if key := km.ExtractFromTitle(mr.Title); key != "" {
			byKey[key]++
		}
	}

	assert.Equal(t, 1, byKey["REV-400"], "the editor's direct-save path needs an MR with no siblings")
}

// --- auto-assign fixture ---

func TestAutoAssignFixture_TurnsAutoAssignOn(t *testing.T) {
	s := newTestAdapterFor(t, autoAssignFixture).Settings()

	assert.True(t, s.AutoAssignReviewers)
	assert.NotEmpty(t, s.Team)
}

func autoAssignCandidates(t *testing.T, a *Adapter) map[int][]domain.User {
	t.Helper()
	roster, err := a.MRSource().ResolveUsers(context.Background(), a.Settings().Team)
	require.NoError(t, err)
	km := domain.NewTicketKeyMatcher(false)
	out := map[int][]domain.User{}
	for _, mr := range a.ds.all() {
		if reviewers, _, ok := domain.AutoAssignCandidates(mr, roster, km); ok {
			out[mr.IID] = reviewers
		}
	}
	return out
}

func TestAutoAssignFixture_OnlyTheQualifyingMRIsACandidate(t *testing.T) {
	a := newTestAdapterFor(t, autoAssignFixture)

	candidates := autoAssignCandidates(t, a)

	require.Len(t, candidates, 1, "!602 has a reviewer, !603 is a draft, !604 is by a non-member, !605 has no key")
	assert.Contains(t, candidates, 601)
	assert.NotContains(t, domain.Usernames(candidates[601]), userAda, "the author is never assigned to their own MR")
}

func TestAutoAssignFixture_AssignsOnceThenLeavesTheMRAlone(t *testing.T) {
	a := newTestAdapterFor(t, autoAssignFixture)
	reviewers := autoAssignCandidates(t, a)[601]
	src := a.MRSource()

	first, err := mrsvc.AutoAssignReviewers(context.Background(), src, 101, 601, reviewers)
	require.NoError(t, err)
	second, err := mrsvc.AutoAssignReviewers(context.Background(), src, 101, 601, reviewers)
	require.NoError(t, err)

	assert.True(t, first, "an MR with no reviewers is assigned")
	assert.False(t, second, "the same MR, now with reviewers, is not assigned again")
	got, _ := reviewersOf(fetchMR(t, a, 101, 601))
	assert.Equal(t, []string{userGrace, userKatherine, userLinus, userMargaret}, got)
}

func TestAutoAssignFixture_NeverReplacesAnMRThatAlreadyHasReviewers(t *testing.T) {
	a := newTestAdapterFor(t, autoAssignFixture)
	team, err := a.MRSource().ResolveUsers(context.Background(), a.Settings().Team)
	require.NoError(t, err)

	assigned, err := mrsvc.AutoAssignReviewers(context.Background(), a.MRSource(), 101, 602, team)
	require.NoError(t, err)

	assert.False(t, assigned)
	got, _ := reviewersOf(fetchMR(t, a, 101, 602))
	assert.Equal(t, []string{userLinus}, got, "!602's own reviewer must be untouched")
}
