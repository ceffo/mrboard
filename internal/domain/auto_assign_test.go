package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	autoAssignTicketTitle = "feat(OD-3345): add something"
	userCarol             = "carol"
	userDave              = "dave"
)

func autoAssignTeam() []User {
	return []User{
		{ID: 1, Username: testUsername},
		{ID: 2, Username: testOther},
		{ID: 3, Username: userCarol},
	}
}

func eligibleMR() MergeRequest {
	return MergeRequest{
		Author: testUsername,
		Title:  autoAssignTicketTitle,
		Phase:  PhaseNeedsReview,
	}
}

func TestAutoAssignCandidates_EligibleMR(t *testing.T) {
	reviewers, issueKey, ok := AutoAssignCandidates(eligibleMR(), autoAssignTeam(), NewTicketKeyMatcher(false))

	require.True(t, ok)
	assert.Equal(t, "OD-3345", issueKey)
	assert.ElementsMatch(t, []User{{ID: 2, Username: testOther}, {ID: 3, Username: userCarol}}, reviewers)
}

func TestAutoAssignCandidates_ExcludesAuthorFromReviewers(t *testing.T) {
	reviewers, _, ok := AutoAssignCandidates(eligibleMR(), autoAssignTeam(), NewTicketKeyMatcher(false))

	require.True(t, ok)
	for _, r := range reviewers {
		assert.NotEqual(t, testUsername, r.Username)
	}
}

func TestAutoAssignCandidates_AuthorNotOnTeam(t *testing.T) {
	mr := eligibleMR()
	mr.Author = userDave

	_, _, ok := AutoAssignCandidates(mr, autoAssignTeam(), NewTicketKeyMatcher(false))

	assert.False(t, ok)
}

func TestAutoAssignCandidates_NoTicketKeyInTitle(t *testing.T) {
	mr := eligibleMR()
	mr.Title = "fix: unrelated cleanup"

	_, _, ok := AutoAssignCandidates(mr, autoAssignTeam(), NewTicketKeyMatcher(false))

	assert.False(t, ok)
}

func TestAutoAssignCandidates_AlreadyHasReviewers(t *testing.T) {
	mr := eligibleMR()
	mr.Reviewers = []ReviewerInfo{{Username: testOther}}

	_, _, ok := AutoAssignCandidates(mr, autoAssignTeam(), NewTicketKeyMatcher(false))

	assert.False(t, ok)
}

func TestAutoAssignCandidates_Draft(t *testing.T) {
	mr := eligibleMR()
	mr.Phase = PhaseDraft

	_, _, ok := AutoAssignCandidates(mr, autoAssignTeam(), NewTicketKeyMatcher(false))

	assert.False(t, ok)
}

func TestAutoAssignCandidates_TeamIsOnlyTheAuthor(t *testing.T) {
	soloTeam := []User{{ID: 1, Username: testUsername}}
	reviewers, _, ok := AutoAssignCandidates(eligibleMR(), soloTeam, NewTicketKeyMatcher(false))

	assert.False(t, ok)
	assert.Nil(t, reviewers)
}

func TestAutoAssignCandidates_EmptyTeamRoster(t *testing.T) {
	_, _, ok := AutoAssignCandidates(eligibleMR(), nil, NewTicketKeyMatcher(false))

	assert.False(t, ok)
}

func TestUsernames(t *testing.T) {
	got := Usernames([]User{{Username: testUsername}, {Username: testOther}})

	assert.Equal(t, []string{testUsername, testOther}, got)
}
