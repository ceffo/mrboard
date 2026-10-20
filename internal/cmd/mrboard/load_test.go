package mrboardcmd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/core"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/ticketsvc"
	ticketmocks "github.com/ceffo/mrboard/internal/domain/service/ticketsvc/mocks"
)

func sprintCore(t *testing.T, boardID int) (*core.Core, *ticketmocks.MockTicketEnricher) {
	enricher := ticketmocks.NewMockTicketEnricher(t)
	return &core.Core{
		TicketEnricher: enricher,
		Config:         &config.AppConfig{Jira: config.Jira{BoardID: boardID}},
	}, enricher
}

func TestActiveSprint_UnusableSprintIsAnError(t *testing.T) {
	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		sprint *ticketsvc.Sprint
		err    error
		want   string
	}{
		"no active sprint":  {want: "no active sprint"},
		"no issue keys":     {sprint: &ticketsvc.Sprint{Name: "S1", Start: start}, want: "has no issues"},
		"no start date":     {sprint: &ticketsvc.Sprint{Name: "S1", IssueKeys: []string{"A-1"}}, want: "no start date"},
		"ticket fetch fail": {err: errors.New("boom"), want: "reading the active sprint: boom"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c, enricher := sprintCore(t, 7)
			enricher.EXPECT().GetActiveSprint(mock.Anything, 7).Return(tc.sprint, tc.err)

			_, err := activeSprint(context.Background(), c)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestActiveSprint_NoBoardConfiguredIsAnError(t *testing.T) {
	c, _ := sprintCore(t, 0)

	_, err := activeSprint(context.Background(), c)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ticket board is configured")
}

func TestActiveSprint_ReturnsTheSprint(t *testing.T) {
	c, enricher := sprintCore(t, 7)
	want := &ticketsvc.Sprint{Name: "S1", Start: time.Now(), IssueKeys: []string{"A-1"}}
	enricher.EXPECT().GetActiveSprint(mock.Anything, 7).Return(want, nil)

	got, err := activeSprint(context.Background(), c)

	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestSprintStartDay_IsLocalMidnightOfTheStartsLocalDate(t *testing.T) {
	// 23:30 UTC on Oct 11 is already Oct 12 in a zone ahead of UTC by 2h or more.
	zone := time.FixedZone("ahead", 3*3600)
	orig := time.Local
	time.Local = zone
	t.Cleanup(func() { time.Local = orig })
	start := time.Date(2026, 10, 11, 23, 30, 0, 0, time.UTC)

	got := sprintStartDay(start)

	assert.Equal(t, time.Date(2026, 10, 12, 0, 0, 0, 0, zone), got)
	assert.Equal(t, zone, got.Location())
}

func TestLoadFilter_SavedFiltersApplyUnlessNoFilters(t *testing.T) {
	state := domain.DefaultAppState()
	state.ViewMode = domain.ViewMine
	state.Filter.ExcludedAssignees = []string{userBob}
	km := domain.NewTicketKeyMatcher(false)

	saved := loadFilter(loadCmdOptions{}, state, "ada", km)
	assert.True(t, saved.MyView)
	assert.Equal(t, []string{userBob}, saved.ExcludedAssignees)

	none := loadFilter(loadCmdOptions{noFilters: true}, state, "ada", km)
	assert.False(t, none.MyView)
	assert.Empty(t, none.ExcludedAssignees)

	all := domain.ViewAll
	overridden := loadFilter(loadCmdOptions{viewOverride: &all}, state, "ada", km)
	assert.False(t, overridden.MyView)
	assert.Equal(t, []string{userBob}, overridden.ExcludedAssignees)

	mine := domain.ViewMine
	forced := loadFilter(loadCmdOptions{noFilters: true, viewOverride: &mine}, state, "ada", km)
	assert.True(t, forced.MyView)
	assert.Empty(t, forced.ExcludedAssignees)
}

func TestParseLoadFlags(t *testing.T) {
	_, err := parseLoadGroup("team")
	assert.Error(t, err)
	_, err = parseLoadSort("age")
	assert.Error(t, err)
	g, err := parseLoadGroup("jira")
	require.NoError(t, err)
	assert.Equal(t, 1, int(g))
}
