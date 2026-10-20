package mrboardcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/core"
	"github.com/ceffo/mrboard/internal/domain"
	domainmocks "github.com/ceffo/mrboard/internal/domain/mocks"
	mrsvcmocks "github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
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

func TestSprintStartDay_IsMidnightOfTheStartsOwnCalendarDay(t *testing.T) {
	// A UTC-midnight start is the 11th on a UTC-5 machine; the sprint began on the 12th.
	orig := time.Local
	time.Local = time.FixedZone("behind", -5*3600)
	t.Cleanup(func() { time.Local = orig })
	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

	got := sprintStartDay(start)

	assert.Equal(t, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), got)
	assert.Equal(t, time.UTC, got.Location())

	cest := time.FixedZone("cest", 2*3600)
	assert.Equal(t, time.Date(2026, 10, 12, 0, 0, 0, 0, cest),
		sprintStartDay(time.Date(2026, 10, 12, 1, 0, 0, 0, cest)))
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

// loadTestRun runs execLoad against one open and two merged MRs of a sprint that
// started at midnight UTC on 2026-10-12, run on a UTC-8 machine. The fetch returns fetchErrs alongside the MRs.
func loadTestRun(t *testing.T, opts loadCmdOptions, fetchErrs []error) (string, error) {
	t.Helper()
	// A machine behind UTC must not move the sprint's start to the 11th.
	orig := time.Local
	time.Local = time.FixedZone("la", -8*3600)
	t.Cleanup(func() { time.Local = orig })
	src := mrsvcmocks.NewMockMergeRequestSource(t)
	cfg := &config.AppConfig{
		CurrentUser: userAlice,
		Jira:        config.Jira{BoardID: 7},
		Sources:     []config.Source{{Type: "user", IDs: []string{userAlice, userBob}}},
	}
	c := newTestCore(t, src, cfg)
	board := domainmocks.NewMockSnapshotStore(t)
	merged := domainmocks.NewMockMergedSnapshotStore(t)
	c.SnapshotStore, c.MergedSnapshotStore = board, merged
	board.EXPECT().Load().Return(nil, time.Time{}, nil)
	merged.EXPECT().Load(mock.Anything).Return(nil, nil)
	merged.EXPECT().Save(mock.Anything, mock.Anything).Return(nil).Maybe()

	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	enricher := ticketmocks.NewMockTicketEnricher(t)
	enricher.EXPECT().GetActiveSprint(mock.Anything, 7).Return(&ticketsvc.Sprint{
		Name: "Sprint 7", Start: start, End: start.AddDate(0, 0, 14), IssueKeys: []string{"OD-1", "OD-2"},
	}, nil)
	c.TicketEnricher = enricher

	approver := func(user string) []domain.ReviewerInfo {
		return []domain.ReviewerInfo{{Username: user, IsApprover: true}}
	}
	mkMR := func(iid int, title string, mergedAt time.Time, reviewers []domain.ReviewerInfo) domain.MergeRequest {
		return domain.MergeRequest{
			ProjectID: 1, IID: iid, ProjectPath: "g/repo", Title: title, MergedAt: mergedAt, Reviewers: reviewers,
		}
	}
	mrs := []domain.MergeRequest{
		mkMR(1, "feat(OD-1): open", time.Time{}, approver(userAlice)),
		mkMR(2, "feat(OD-2): merged after start", time.Date(2026, 10, 12, 0, 5, 0, 0, time.UTC), approver(userBob)),
		mkMR(3, "feat(OD-2): merged before start", time.Date(2026, 10, 11, 23, 55, 0, 0, time.UTC), approver(userBob)),
		mkMR(4, "feat(OD-9): not in sprint", time.Time{}, approver(userBob)),
	}
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(mrs, fetchErrs)

	var out bytes.Buffer
	err := execLoad(context.WithValue(context.Background(), coreKey{}, c), opts, &out)
	return out.String(), err
}

func TestExecLoad_ReportsTheSprintPopulation(t *testing.T) {
	out, err := loadTestRun(t, loadCmdOptions{color: colorNever}, nil)

	require.NoError(t, err)
	assert.Contains(t, out, "Sprint 7 · Oct 12 – Oct 26 · 2 MRs in scope · all")
	assert.Contains(t, out, "repo!1")
	assert.Contains(t, out, "repo!2")
	assert.NotContains(t, out, "repo!3", "merged before the sprint start")
	assert.NotContains(t, out, "repo!4", "ticket outside the sprint")
}

func TestExecLoad_JSONCountsMatchTheTree(t *testing.T) {
	out, err := loadTestRun(t, loadCmdOptions{json: true, remainingOnly: true}, nil)

	require.NoError(t, err)
	var got loadJSON
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, 1, got.MRCount, "remaining only drops the merged MR")
	assert.Equal(t, 2, got.PopulationCount)
	assert.False(t, got.Partial)
	assert.Equal(t, "2026-10-12T00:00:00Z", got.Sprint.Start)
}

func TestExecLoad_FetchErrorsPrintTheLoadThenFail(t *testing.T) {
	out, err := loadTestRun(t, loadCmdOptions{json: true}, []error{errors.New("merged listing failed")})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "partial")
	var got loadJSON
	require.NoError(t, json.Unmarshal([]byte(out), &got), "the report is still printed")
	assert.True(t, got.Partial)
	assert.Equal(t, 2, got.MRCount)
}
