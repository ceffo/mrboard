package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

func loadLine(iid int, size int, approver string, approvers ...mrsvc.LoadApprover) mrsvc.LoadLine {
	return mrsvc.LoadLine{
		MR:        domain.MergeRequest{ProjectID: 1, IID: iid, ProjectPath: "g/repo", Title: "title"},
		Ticket:    testTicket,
		Size:      domain.ReviewSize{Known: true, Lines: size},
		Bucket:    domain.SizeBucketS,
		Approver:  approver,
		Approvers: approvers,
	}
}

const (
	testTicket = "T-1"
	testAda    = "ada"
	testBob    = "bob"
)

var testSprint = LoadSprint{
	Name:  "Sprint 7",
	Start: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
	End:   time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
}

func TestRenderLoad_HeaderStatesMRsInScopeNotShown(t *testing.T) {
	load := mrsvc.Load{MRCount: 3, PopulationCount: 5, RemainingOnly: true}

	out := RenderLoad(load, testSprint, Styles{})

	assert.Contains(t, out, "Sprint 7 · Oct 12 – Oct 26 · 3 MRs in scope (of 5 in the sprint) · remaining only")
	assert.NotContains(t, out, "shown")
}

func TestRenderLoad_EmptyScopeIsStated(t *testing.T) {
	out := RenderLoad(mrsvc.Load{}, testSprint, Styles{})

	assert.Contains(t, out, "0 MRs in scope")
	assert.Contains(t, out, "No MRs in scope.")
}

func TestRenderLoad_PersonGroupingFootnotesDoubleCountingAndUnlistedMRs(t *testing.T) {
	ada := mrsvc.LoadApprover{Username: testAda, InTeam: true}
	bob := mrsvc.LoadApprover{Username: testBob, InTeam: true}
	l1 := loadLine(1, 100, testAda, ada, bob)
	l1a := loadLine(1, 100, testBob, ada, bob)
	load := mrsvc.Load{
		MRCount: 2, PopulationCount: 2,
		TeamTotal: mrsvc.LoadTotal{Lines: 100},
		Groups: []mrsvc.LoadGroup{
			{Key: testAda, Name: "Ada", Total: mrsvc.LoadTotal{Lines: 100}, Lines: []mrsvc.LoadLine{l1}},
			{Key: testBob, Name: "Bob", Total: mrsvc.LoadTotal{Lines: 100}, Lines: []mrsvc.LoadLine{l1a}},
		},
	}

	out := RenderLoad(load, testSprint, Styles{})

	assert.Contains(t, out, "Ada  100 lines")
	assert.Contains(t, out, "+bob")
	assert.Contains(t, out, "person loads add up to more than the team total (100 lines")
	assert.Contains(t, out, "1 MR is on no person line")
}

func TestRenderLoad_TicketGroupingListsApproversAndHasNoFootnotes(t *testing.T) {
	l := loadLine(4, 30, "", mrsvc.LoadApprover{Username: testAda, Approved: true}, mrsvc.LoadApprover{Username: testBob})
	load := mrsvc.Load{
		Group: mrsvc.LoadByTicket, MRCount: 1, PopulationCount: 1,
		TeamTotal: mrsvc.LoadTotal{Lines: 30, Partial: true},
		Groups: []mrsvc.LoadGroup{
			{Key: testTicket, Name: testTicket, Total: mrsvc.LoadTotal{Lines: 30, Partial: true}, Lines: []mrsvc.LoadLine{l}},
			{CatchAll: true},
		},
	}

	out := RenderLoad(load, testSprint, Styles{})

	assert.Contains(t, out, "T-1  30+ lines")
	assert.Contains(t, out, "repo!4")
	assert.Contains(t, out, "✓ada bob")
	assert.Contains(t, out, "(no ticket)  0 lines")
	assert.NotContains(t, out, "person loads")
}

func TestRenderLoad_ZeroStylesEmitNoEscapeCodes(t *testing.T) {
	l := loadLine(1, 10, testAda, mrsvc.LoadApprover{Username: testAda, InTeam: true})
	load := mrsvc.Load{
		MRCount: 1, PopulationCount: 1, TeamTotal: mrsvc.LoadTotal{Lines: 10},
		Groups: []mrsvc.LoadGroup{{Key: testAda, Name: "Ada", Total: mrsvc.LoadTotal{Lines: 10}, Lines: []mrsvc.LoadLine{l}}},
	}

	assert.NotContains(t, RenderLoad(load, testSprint, Styles{}), "\x1b[")
	assert.Contains(t, RenderLoad(load, testSprint, NewLoadStyles("", "dark")), "\x1b[")
}
