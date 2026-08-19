package tui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHandleSprintIssueKeys_FetchFailureSurfacesToast is the regression test for
// mrboard silently losing sprint awareness: a failed sprint refresh (e.g. a
// transient network blip at boot) used to be logged and otherwise dropped,
// leaving m.sprintIssueKeys frozen at its last value with nothing telling the
// user their sprint filter may be stale. Every sibling background-fetch
// handler in this file (handleTeamResolved, handleTicketLinkResult, ...)
// surfaces its failure via a toast; sprint fetch must do the same.
func TestHandleSprintIssueKeys_FetchFailureSurfacesToast(t *testing.T) {
	m := makeModel(t, someMRs(), "")

	next, cmd := m.Update(SprintIssueKeysMsg{Err: errors.New("context deadline exceeded")})
	m2 := next.(Model)

	assert.NotNil(t, cmd, "a failed sprint refresh must surface a toast")
	assert.Nil(t, m2.sprintIssueKeys, "a failed refresh must not fabricate sprint membership")
}

func TestHandleSprintIssueKeys_NoActiveSprintShowsNoToast(t *testing.T) {
	m := makeModel(t, someMRs(), "")

	next, cmd := m.Update(SprintIssueKeysMsg{Keys: nil})
	m2 := next.(Model)

	assert.Nil(t, cmd, "no active sprint is a valid state, not a failure")
	assert.Nil(t, m2.sprintIssueKeys)
}

func TestHandleSprintIssueKeys_SuccessAppliesKeysAndShowsNoToast(t *testing.T) {
	m := makeModel(t, someMRs(), "")

	next, cmd := m.Update(SprintIssueKeysMsg{Keys: []string{"OPS-1", "OPS-2"}})
	m2 := next.(Model)

	assert.Nil(t, cmd, "a successful refresh needs no toast")
	assert.True(t, m2.sprintIssueKeys["OPS-1"])
	assert.True(t, m2.sprintIssueKeys["OPS-2"])
}
