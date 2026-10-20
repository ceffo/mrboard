package mrsvc_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

const (
	userDave = "dave"
	userErin = "erin"

	ticketOD1  = "OD-1"
	ticketOD2  = "OD-2"
	line1Alice = "!1@alice"
	line2Alice = "!2@alice"
	line1Bob   = "!1@bob"
	line3Dave  = "!3@dave"
	line5Carol = "!5@carol"
	line7Erin  = "!7@erin"
)

var loadSprintStart = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)

// loadMR builds an MR in project "g/r" with the given title, phase and approvers.
// lines < 0 leaves the diff stats unknown; mergedDay > 0 merges it on that October day.
func loadMR(iid int, title string, lines, mergedDay int, approvers ...domain.ReviewerInfo) domain.MergeRequest {
	m := domain.MergeRequest{
		IID: iid, ProjectID: 1, ProjectPath: "g/r", Title: title,
		Author: "author", Phase: domain.PhaseNeedsReview, Reviewers: approvers,
	}
	if lines >= 0 {
		m.DiffStats = []domain.FileDiffStat{{Path: "a.go", Additions: lines}}
	}
	if mergedDay > 0 {
		m.MergedAt = time.Date(2026, 10, mergedDay, 12, 0, 0, 0, time.UTC)
	}
	return m
}

func approver(username string, approved bool) domain.ReviewerInfo {
	state := domain.ReviewerNotStarted
	if approved {
		state = domain.ReviewerApproved
	}
	return domain.ReviewerInfo{Username: username, Name: username, IsApprover: true, State: state}
}

// loadFixture is a sprint with tickets OD-1 and OD-2 and team alice, bob, carol.
func loadFixture() []domain.MergeRequest {
	draft := loadMR(5, "chore: e", 10, 0, approver(userCarol, false))
	draft.Phase = domain.PhaseDraft
	return []domain.MergeRequest{
		loadMR(1, "feat(OD-1): a", 100, 0, approver(userAlice, true), approver(userBob, false)),
		loadMR(2, "feat(OD-1): b", -1, 0, approver(userAlice, false)),
		loadMR(3, "fix(OD-2): c", 30, 10, approver(userBob, false), approver(userDave, false)),
		loadMR(4, "fix(OD-2): d", 50, 1, approver(userBob, false)), // merged before the sprint
		draft, // unticketed draft
		loadMR(6, "feat(OD-9): f", 70, 0, approver(userBob, false)),                // ticket not in the sprint
		loadMR(7, "chore: g", 5, 12, approver(userErin, false)),                    // unticketed, merged
		loadMR(8, "fix(OD-1): h", 40, 0, domain.ReviewerInfo{Username: userCarol}), // reviewer, not approver
	}
}

func loadOpts() mrsvc.LoadOptions {
	return mrsvc.LoadOptions{
		SprintStart: loadSprintStart,
		SprintKeys:  map[string]bool{ticketOD1: true, ticketOD2: true},
		Team:        []string{userAlice, userBob, userCarol},
		Exclusions:  domain.UniformSizeExclusions(domain.NewSizeExclusions(true, nil, nil)),
		Thresholds:  domain.DefaultSizeThresholds,
		Sort:        mrsvc.LoadSortSize,
	}
}

// groupSummary is a load group reduced to what the tests compare. A line reads
// "!iid" in a ticket group and "!iid@approver" in a person group.
type groupSummary struct {
	Key   string
	Total mrsvc.LoadTotal
	Lines []string
}

func summarize(l mrsvc.Load) []groupSummary {
	out := make([]groupSummary, 0, len(l.Groups))
	for _, g := range l.Groups {
		key := g.Key
		if g.CatchAll {
			key = "*"
		}
		s := groupSummary{Key: key, Total: g.Total, Lines: []string{}}
		for _, line := range g.Lines {
			id := fmt.Sprintf("!%d", line.MR.IID)
			if line.Approver != "" {
				id += "@" + line.Approver
			}
			s.Lines = append(s.Lines, id)
		}
		out = append(out, s)
	}
	return out
}

func total(lines int, partial bool) mrsvc.LoadTotal {
	return mrsvc.LoadTotal{Lines: lines, Partial: partial}
}

func TestComputeLoad(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*mrsvc.LoadOptions)
		want       []groupSummary
		wantTeam   mrsvc.LoadTotal
		wantCount  int
		wantPopCnt int
	}{
		{
			name: "by person, size sort: full credit per approver, others last",
			want: []groupSummary{
				{Key: userBob, Total: total(130, false), Lines: []string{line1Bob, "!3@bob"}},
				{Key: userAlice, Total: total(100, true), Lines: []string{line1Alice, line2Alice}},
				{Key: userCarol, Total: total(10, false), Lines: []string{line5Carol}},
				{Key: "*", Total: total(35, false), Lines: []string{line3Dave, line7Erin}},
			},
			wantTeam: total(185, true), wantCount: 6, wantPopCnt: 6,
		},
		{
			name:   "by person, person sort",
			mutate: func(o *mrsvc.LoadOptions) { o.Sort = mrsvc.LoadSortPerson },
			want: []groupSummary{
				{Key: userAlice, Total: total(100, true), Lines: []string{line1Alice, line2Alice}},
				{Key: userBob, Total: total(130, false), Lines: []string{line1Bob, "!3@bob"}},
				{Key: userCarol, Total: total(10, false), Lines: []string{line5Carol}},
				{Key: "*", Total: total(35, false), Lines: []string{line3Dave, line7Erin}},
			},
			wantTeam: total(185, true), wantCount: 6, wantPopCnt: 6,
		},
		{
			name: "by ticket, ticket sort: each MR once, no ticket last",
			mutate: func(o *mrsvc.LoadOptions) {
				o.Group, o.Sort = mrsvc.LoadByTicket, mrsvc.LoadSortTicket
			},
			want: []groupSummary{
				{Key: ticketOD1, Total: total(140, true), Lines: []string{"!1", "!2", "!8"}},
				{Key: ticketOD2, Total: total(30, false), Lines: []string{"!3"}},
				{Key: "*", Total: total(15, false), Lines: []string{"!5", "!7"}},
			},
			wantTeam: total(185, true), wantCount: 6, wantPopCnt: 6,
		},
		{
			name:   "by ticket, size sort: lines heaviest first, unknown last",
			mutate: func(o *mrsvc.LoadOptions) { o.Group = mrsvc.LoadByTicket },
			want: []groupSummary{
				{Key: ticketOD1, Total: total(140, true), Lines: []string{"!1", "!8", "!2"}},
				{Key: ticketOD2, Total: total(30, false), Lines: []string{"!3"}},
				{Key: "*", Total: total(15, false), Lines: []string{"!5", "!7"}},
			},
			wantTeam: total(185, true), wantCount: 6, wantPopCnt: 6,
		},
		{
			name:   "remaining only: approved and merged lines hidden, draft stays",
			mutate: func(o *mrsvc.LoadOptions) { o.RemainingOnly = true },
			want: []groupSummary{
				{Key: userBob, Total: total(100, false), Lines: []string{line1Bob}},
				{Key: userCarol, Total: total(10, false), Lines: []string{line5Carol}},
				{Key: userAlice, Total: total(0, true), Lines: []string{line2Alice}},
			},
			wantTeam: total(150, true), wantCount: 4, wantPopCnt: 6,
		},
		{
			name: "remaining only by ticket: merged MRs hidden",
			mutate: func(o *mrsvc.LoadOptions) {
				o.Group, o.RemainingOnly = mrsvc.LoadByTicket, true
			},
			want: []groupSummary{
				{Key: ticketOD1, Total: total(140, true), Lines: []string{"!1", "!8", "!2"}},
				{Key: "*", Total: total(10, false), Lines: []string{"!5"}},
			},
			wantTeam: total(150, true), wantCount: 4, wantPopCnt: 6,
		},
		{
			name: "excluded reviewer loses their row, MRs stay",
			mutate: func(o *mrsvc.LoadOptions) {
				o.Filter.ExcludedReviewers = []string{userBob, userDave}
			},
			want: []groupSummary{
				{Key: userAlice, Total: total(100, true), Lines: []string{line1Alice, line2Alice}},
				{Key: userCarol, Total: total(10, false), Lines: []string{line5Carol}},
				{Key: "*", Total: total(5, false), Lines: []string{line7Erin}},
			},
			wantTeam: total(185, true), wantCount: 6, wantPopCnt: 6,
		},
		{
			name: "phase filter narrows open MRs only",
			mutate: func(o *mrsvc.LoadOptions) {
				o.Group = mrsvc.LoadByTicket
				o.Filter.Phases = map[domain.MRPhase]bool{domain.PhaseDraft: true}
			},
			want: []groupSummary{
				{Key: ticketOD2, Total: total(30, false), Lines: []string{"!3"}},
				{Key: "*", Total: total(15, false), Lines: []string{"!5", "!7"}},
			},
			wantTeam: total(45, false), wantCount: 3, wantPopCnt: 6,
		},
		{
			name: "ticket and ticketless exclusions",
			mutate: func(o *mrsvc.LoadOptions) {
				o.Group = mrsvc.LoadByTicket
				o.Filter.ExcludedTicketKeys = []string{ticketOD1}
				o.Filter.ExcludeTicketless = true
			},
			want: []groupSummary{
				{Key: ticketOD2, Total: total(30, false), Lines: []string{"!3"}},
			},
			wantTeam: total(30, false), wantCount: 1, wantPopCnt: 6,
		},
		{
			name: "me scope and assignee exclusion",
			mutate: func(o *mrsvc.LoadOptions) {
				o.Group = mrsvc.LoadByTicket
				o.Filter.MyView, o.Filter.CurrentUser = true, userBob
				o.Filter.ExcludedAssignees = []string{"zed"}
			},
			want: []groupSummary{
				{Key: ticketOD1, Total: total(100, false), Lines: []string{"!1"}},
			},
			wantTeam: total(100, false), wantCount: 1, wantPopCnt: 6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mrs := loadFixture()
			mrs[2].Assignee = "zed" // !3: excluded by the assignee filter case
			opts := loadOpts()
			if tt.mutate != nil {
				tt.mutate(&opts)
			}

			got := mrsvc.ComputeLoad(mrs, opts)

			assert.Equal(t, tt.want, summarize(got))
			assert.Equal(t, tt.wantTeam, got.TeamTotal)
			assert.Equal(t, tt.wantCount, got.MRCount)
			assert.Equal(t, tt.wantPopCnt, got.PopulationCount)
		})
	}
}

func TestComputeLoad_TeamMemberWithoutLoadKeepsRow(t *testing.T) {
	opts := loadOpts()
	opts.Team = append(opts.Team, "frank")

	got := mrsvc.ComputeLoad(loadFixture(), opts)

	require.Len(t, got.Groups, 5)
	assert.Equal(t, "frank", got.Groups[3].Key)
	assert.Empty(t, got.Groups[3].Lines)
	assert.Equal(t, total(0, false), got.Groups[3].Total)
	assert.True(t, got.Groups[4].CatchAll)
}

func TestComputeLoad_CatchAllLastEvenWhenHeaviest(t *testing.T) {
	mrs := []domain.MergeRequest{
		loadMR(1, "feat(OD-1): a", 5, 0, approver(userAlice, false)),
		loadMR(2, "chore: b", 900, 0, approver(userDave, false)),
	}
	for _, group := range []mrsvc.LoadGrouping{mrsvc.LoadByPerson, mrsvc.LoadByTicket} {
		for _, sortBy := range []mrsvc.LoadSort{mrsvc.LoadSortPerson, mrsvc.LoadSortTicket, mrsvc.LoadSortSize} {
			opts := loadOpts()
			opts.Team = []string{userAlice}
			opts.Group, opts.Sort = group, sortBy

			got := mrsvc.ComputeLoad(mrs, opts)

			require.Len(t, got.Groups, 2)
			assert.False(t, got.Groups[0].CatchAll, "group %d sort %d", group, sortBy)
			assert.True(t, got.Groups[1].CatchAll, "group %d sort %d", group, sortBy)
		}
	}
}

func TestComputeLoad_TicketSortIsNumeric(t *testing.T) {
	mrs := []domain.MergeRequest{
		loadMR(1, "feat(OD-10): a", 5, 0, approver(userAlice, false)),
		loadMR(2, "feat(OD-9): b", 5, 0, approver(userAlice, false)),
	}
	opts := loadOpts()
	opts.SprintKeys = map[string]bool{"OD-9": true, "OD-10": true}
	opts.Group, opts.Sort = mrsvc.LoadByTicket, mrsvc.LoadSortTicket

	got := mrsvc.ComputeLoad(mrs, opts)

	require.Len(t, got.Groups, 2)
	assert.Equal(t, "OD-9", got.Groups[0].Key)
	assert.Equal(t, "OD-10", got.Groups[1].Key)
}

func TestComputeLoad_LineMarks(t *testing.T) {
	got := mrsvc.ComputeLoad(loadFixture(), loadOpts())

	lines := map[string]mrsvc.LoadLine{}
	for _, g := range got.Groups {
		for _, l := range g.Lines {
			lines[fmt.Sprintf("!%d@%s", l.MR.IID, l.Approver)] = l
		}
	}
	require.Contains(t, lines, line1Alice)
	require.Contains(t, lines, line2Alice)
	require.Contains(t, lines, line3Dave)
	require.Contains(t, lines, line5Carol)

	assert.True(t, lines[line1Alice].Approved)
	assert.Equal(t, domain.SizeBucketS, lines[line1Alice].Bucket)
	assert.Equal(t, []mrsvc.LoadApprover{{Username: userBob, Name: userBob, InTeam: true}},
		lines[line1Alice].CoApprovers())
	assert.Equal(t, domain.SizeBucketUnknown, lines[line2Alice].Bucket)
	assert.False(t, lines[line2Alice].Size.Known)
	assert.True(t, lines[line3Dave].Merged)
	assert.False(t, lines[line3Dave].Approvers[1].InTeam)
	assert.True(t, lines[line5Carol].Draft)
	assert.Equal(t, "", lines[line5Carol].Ticket)
}

func TestComputeLoad_OpenAndMergedCopiesCountOnce(t *testing.T) {
	open := loadMR(1, "feat(OD-1): a", 10, 0, approver(userAlice, false))
	merged := loadMR(1, "feat(OD-1): a", 10, 8, approver(userAlice, true))
	opts := loadOpts()
	opts.Group = mrsvc.LoadByTicket

	got := mrsvc.ComputeLoad([]domain.MergeRequest{open, merged}, opts)

	assert.Equal(t, 1, got.MRCount)
	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Lines, 1)
	assert.True(t, got.Groups[0].Lines[0].Merged)
}

func TestComputeLoad_SprintStartIsADayInItsLocation(t *testing.T) {
	cest := time.FixedZone("CEST", 2*60*60)
	merged := func(iid int, at time.Time) domain.MergeRequest {
		m := loadMR(iid, "feat(OD-1): a", 1, 0, approver(userAlice, false))
		m.MergedAt = at
		return m
	}
	mrs := []domain.MergeRequest{
		merged(1, time.Date(2026, 10, 4, 21, 30, 0, 0, time.UTC)), // 23:30 CEST the day before
		merged(2, time.Date(2026, 10, 4, 22, 30, 0, 0, time.UTC)), // 00:30 CEST on the start day
		merged(3, time.Date(2026, 10, 5, 7, 0, 0, 0, cest)),       // start day, before the start time
	}
	tests := []struct {
		name  string
		start time.Time
		want  []string
	}{
		{"start in CEST counts from CEST midnight", time.Date(2026, 10, 5, 8, 0, 0, 0, cest), []string{"!2", "!3"}},
		{"start in UTC counts from UTC midnight", time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC), []string{"!3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := loadOpts()
			opts.SprintStart = tt.start
			opts.Group = mrsvc.LoadByTicket

			got := summarize(mrsvc.ComputeLoad(mrs, opts))

			require.Len(t, got, 1)
			assert.Equal(t, tt.want, got[0].Lines)
		})
	}
}

func TestComputeLoad_ExclusionsPerMR(t *testing.T) {
	stats := []domain.FileDiffStat{{Path: "api.gen.go", Additions: 300}, {Path: "main.go", Additions: 20}}
	a := loadMR(1, "feat(OD-1): a", 0, 0, approver(userAlice, false))
	a.DiffStats = stats
	b := loadMR(2, "feat(OD-1): b", 0, 0, approver(userAlice, false))
	b.ProjectID, b.DiffStats = 2, stats
	opts := loadOpts()
	opts.Group = mrsvc.LoadByTicket
	opts.Exclusions = func(mr domain.MergeRequest) domain.SizeExclusions {
		if mr.ProjectID == 1 {
			return domain.NewSizeExclusions(true, nil,
				[]domain.GeneratedRule{{Glob: "*.gen.go", Attr: domain.AttrLinguistGenerated, State: domain.AttrSet}})
		}
		return domain.NewSizeExclusions(true, nil, nil)
	}

	got := mrsvc.ComputeLoad([]domain.MergeRequest{a, b}, opts)

	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Lines, 2) // size sort: project 2 counts the generated file
	assert.Equal(t, domain.ReviewSize{Lines: 320, Known: true}, got.Groups[0].Lines[0].Size)
	assert.Equal(t, domain.ReviewSize{Lines: 20, Known: true}, got.Groups[0].Lines[1].Size)
}
