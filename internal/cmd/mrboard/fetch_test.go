package mrboardcmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

func TestParseViewMode(t *testing.T) {
	mine, err := parseViewMode("mine")
	require.NoError(t, err)
	assert.Equal(t, domain.ViewMine, mine)

	all, err := parseViewMode("all")
	require.NoError(t, err)
	assert.Equal(t, domain.ViewAll, all)

	_, err = parseViewMode("team")
	assert.Error(t, err)
}

func TestBoardFilterOptions_CarriesSavedState(t *testing.T) {
	state := domain.AppState{
		SortField: "age",
		SortDesc:  true,
		Filter: domain.FilterCriteria{
			ExcludedAssignees: []string{"bob"},
			ExcludeTicketless: true,
		},
	}

	got := boardFilterOptions(state, domain.ViewMine, "alice", domain.NewTicketKeyMatcher(false))

	assert.True(t, got.MyView)
	assert.Equal(t, "alice", got.CurrentUser)
	assert.Equal(t, "age", got.SortField)
	assert.True(t, got.SortDesc)
	assert.Equal(t, []string{"bob"}, got.ExcludedAssignees)
	assert.True(t, got.ExcludeTicketless)
}

func TestBoardFilterOptions_MineWithoutUserDegradesToAll(t *testing.T) {
	got := boardFilterOptions(domain.AppState{}, domain.ViewMine, "", domain.NewTicketKeyMatcher(false))

	assert.False(t, got.MyView)
}
