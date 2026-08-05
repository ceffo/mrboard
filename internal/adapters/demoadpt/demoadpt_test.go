package demoadpt

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// anchor is a fixed boot instant so age assertions are exact.
var anchor = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

func newTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	a, err := New(Config{Now: anchor, BaseURL: "https://gitlab.demo.invalid", Latency: -1})
	require.NoError(t, err)
	return a
}

func TestFixtureLoads(t *testing.T) {
	a := newTestAdapter(t)

	all := a.ds.all()
	assert.Len(t, all, 11, "the fixture should hold every demo MR, reviewer-source ones included")
	assert.NotEmpty(t, a.ds.issueTypes)
	assert.NotEmpty(t, a.ds.sprintKeys)
	assert.Len(t, a.ds.projectPaths, 3)
}

// TestPhasePlacement pins every MR to the column it is meant to demonstrate.
// Phases are derived by domain.ClassifyPhase rather than read from the fixture,
// so this is what turns a future change in the phase rules into a red build
// instead of a silently reshuffled demo board.
func TestPhasePlacement(t *testing.T) {
	a := newTestAdapter(t)

	want := map[domain.MRKey]domain.MRPhase{
		{ProjectID: 101, IID: 412}: domain.PhaseDraft,
		{ProjectID: 102, IID: 88}:  domain.PhaseDraft,
		{ProjectID: 101, IID: 418}: domain.PhaseNeedsReview,
		{ProjectID: 102, IID: 91}:  domain.PhaseNeedsReview,
		{ProjectID: 103, IID: 37}:  domain.PhaseNeedsReview,
		{ProjectID: 101, IID: 420}: domain.PhaseNeedsReview,
		{ProjectID: 101, IID: 410}: domain.PhaseNeedsAuthorAction,
		{ProjectID: 102, IID: 94}:  domain.PhaseNeedsAuthorAction,
		{ProjectID: 103, IID: 39}:  domain.PhaseNeedsAuthorAction,
		{ProjectID: 101, IID: 415}: domain.PhaseReadyToMerge,
		{ProjectID: 102, IID: 96}:  domain.PhaseReadyToMerge,
	}

	got := map[domain.MRKey]domain.MRPhase{}
	for _, mr := range a.ds.all() {
		got[mr.Key()] = mr.Phase
	}
	assert.Equal(t, want, got)
}

// TestEveryReviewerStateIsShown keeps the demo board a complete showcase: if a
// state stops appearing, the icon for it never renders in the GIF.
func TestEveryReviewerStateIsShown(t *testing.T) {
	a := newTestAdapter(t)

	seen := map[domain.ReviewerState]bool{}
	approver, plain := false, false
	for _, mr := range a.ds.all() {
		for _, r := range mr.Reviewers {
			seen[r.State] = true
			if r.IsApprover {
				approver = true
			} else {
				plain = true
			}
		}
	}
	for _, st := range []domain.ReviewerState{
		domain.ReviewerNotStarted, domain.ReviewerCommented,
		domain.ReviewerReReviewRequested, domain.ReviewerApproved,
	} {
		assert.True(t, seen[st], "no reviewer in state %s", st)
	}
	assert.True(t, approver, "at least one designated approver must appear")
	assert.True(t, plain, "at least one plain reviewer must appear")
}

// TestAgeBucketsAreCovered asserts the fixture spans the ok/warn/urgent
// thresholds the demo config sets (72h and 120h), so all three age colours show.
func TestAgeBucketsAreCovered(t *testing.T) {
	a := newTestAdapter(t)

	var ok, warn, urgent bool
	for _, mr := range a.ds.all() {
		age := anchor.Sub(mr.CreatedAt)
		switch {
		case age >= 120*time.Hour:
			urgent = true
		case age >= 72*time.Hour:
			warn = true
		default:
			ok = true
		}
	}
	assert.True(t, ok, "need an MR under the warn threshold")
	assert.True(t, warn, "need an MR between the warn and error thresholds")
	assert.True(t, urgent, "need an MR past the error threshold")
}

// TestOffsetsAreWholeMinutes protects frame-reproducibility: ages render
// truncated to the minute, so a sub-minute component would make the first frame
// of a recording differ from the next.
func TestOffsetsAreWholeMinutes(t *testing.T) {
	a := newTestAdapter(t)

	check := func(label string, ts time.Time) {
		if ts.IsZero() {
			return
		}
		assert.Zero(t, anchor.Sub(ts)%time.Minute, "%s is not a whole number of minutes from the anchor", label)
	}
	for _, mr := range a.ds.all() {
		check("created_ago", mr.CreatedAt)
		check("updated_ago", mr.UpdatedAt)
		for _, r := range mr.Reviewers {
			check("waiting_ago", r.WaitingSince)
			check("approved_ago", r.ApprovedAt)
		}
	}
	check("snapshot_written_ago", a.ds.snapshotWrittenAt)
}

// TestTicketedMRsCarryBackLinkMarker is the guard against the demo mutating
// itself: without the marker the board considers the back-link missing and
// issues a description write on every fetch.
func TestTicketedMRsCarryBackLinkMarker(t *testing.T) {
	a := newTestAdapter(t)

	ticketed := 0
	for _, mr := range a.ds.all() {
		if domain.ExtractJiraID(mr.Title) == "" {
			continue
		}
		ticketed++
		assert.True(t, domain.HasJiraLink(mr.Description),
			"MR !%d has a ticket key but no back-link marker in its description", mr.IID)
	}
	assert.Positive(t, ticketed, "the demo must show some ticketed MRs")
}

func TestReviewerStateVocabularyIsExhaustive(t *testing.T) {
	for _, st := range []domain.ReviewerState{
		domain.ReviewerNotStarted, domain.ReviewerCommented,
		domain.ReviewerReReviewRequested, domain.ReviewerApproved,
	} {
		assert.Contains(t, reviewerStates, st.String(),
			"fixture vocabulary is missing reviewer state %s", st)
	}
}

func TestParseOffset(t *testing.T) {
	cases := map[string]time.Duration{
		"":        0,
		"45m":     45 * time.Minute,
		"9h37m":   9*time.Hour + 37*time.Minute,
		"26h":     26 * time.Hour,
		"2d":      48 * time.Hour,
		"124d4h":  124*24*time.Hour + 4*time.Hour,
		"6d2h30m": 6*24*time.Hour + 2*time.Hour + 30*time.Minute,
	}
	for in, want := range cases {
		got, err := parseOffset(in)
		require.NoError(t, err, "input %q", in)
		assert.Equal(t, want, got, "input %q", in)
	}

	for _, bad := range []string{"soon", "3w", "1h2d", "-5m", "12"} {
		_, err := parseOffset(bad)
		assert.Error(t, err, "input %q should be rejected", bad)
	}
}

// --- port behaviour ---

func TestFetchAllExcludesReviewerSourceUnlessRequested(t *testing.T) {
	a := newTestAdapter(t)
	src := a.MRSource()

	def, errs := src.FetchAll(context.Background(), mrsvc.FetchOptions{})
	assert.Empty(t, errs)
	assert.Len(t, def, 10)

	withReviewer, errs := src.FetchAll(context.Background(), mrsvc.FetchOptions{IncludeReviewerMRs: true})
	assert.Empty(t, errs)
	assert.Len(t, withReviewer, 11)
}

// TestSaveApproversMovesCardToApproved is the write beat the demo is built
// around: promoting the outstanding approver must actually re-classify the MR,
// not just flip a flag.
func TestSaveApproversMovesCardToApproved(t *testing.T) {
	a := newTestAdapter(t)
	src := a.MRSource()
	key := domain.MRKey{ProjectID: 101, IID: 418} // Needs Review: grace approver, not started

	before, ok := a.ds.find(key)
	require.True(t, ok)
	require.Equal(t, domain.PhaseNeedsReview, before.Phase)

	// Approve grace in place, then designate only her as approver.
	require.True(t, a.ds.mutate(key, func(mr *domain.MergeRequest) {
		for i := range mr.Reviewers {
			if mr.Reviewers[i].Username == "grace" {
				mr.Reviewers[i].State = domain.ReviewerApproved
				mr.Reviewers[i].ApprovedAt = anchor
			}
		}
	}))
	require.NoError(t, src.SaveApprovers(context.Background(), 101, 418, []int64{2}))

	after, ok := a.ds.find(key)
	require.True(t, ok)
	assert.Equal(t, domain.PhaseReadyToMerge, after.Phase, "the card must move to the Approved column")
	assert.False(t, after.ReadyToMergeSince.IsZero(), "ReadyToMergeSince must be derived")
}

func TestSetReviewersPreservesExistingStateAndIsVisibleToNextFetch(t *testing.T) {
	a := newTestAdapter(t)
	src := a.MRSource()

	// !418 has grace (approver) and linus. Keep grace, drop linus, add margaret.
	require.NoError(t, src.SetReviewers(context.Background(), 101, 418, []int64{2, 4}))

	got, err := src.FetchMR(context.Background(), 101, 418)
	require.NoError(t, err)
	require.Len(t, got.Reviewers, 2)
	assert.Equal(t, "grace", got.Reviewers[0].Username)
	assert.True(t, got.Reviewers[0].IsApprover, "a retained reviewer keeps their approver flag")
	assert.Equal(t, "margaret", got.Reviewers[1].Username)
	assert.Equal(t, domain.ReviewerNotStarted, got.Reviewers[1].State)

	// The mutation must be visible to a subsequent fetch, or the card would
	// visibly revert on the next refresh tick.
	all, _ := src.FetchAll(context.Background(), mrsvc.FetchOptions{})
	for _, mr := range all {
		if mr.Key() == (domain.MRKey{ProjectID: 101, IID: 418}) {
			assert.Len(t, mr.Reviewers, 2)
		}
	}
}

// TestReadsAreDeepCopied guards a real race: the reviewer-write path mutates
// mr.Reviewers[i] in place on whatever slice it was handed.
func TestReadsAreDeepCopied(t *testing.T) {
	a := newTestAdapter(t)

	first, ok := a.ds.find(domain.MRKey{ProjectID: 101, IID: 418})
	require.True(t, ok)
	require.NotEmpty(t, first.Reviewers)
	first.Reviewers[0].Username = "tampered"

	second, ok := a.ds.find(domain.MRKey{ProjectID: 101, IID: 418})
	require.True(t, ok)
	assert.NotEqual(t, "tampered", second.Reviewers[0].Username,
		"mutating a returned MR must not reach the dataset")
}

func TestSnapshotStoreServesWarmBootThenRoundTrips(t *testing.T) {
	a := newTestAdapter(t)
	store := a.SnapshotStore()

	seeded, writtenAt, err := store.Load()
	require.NoError(t, err)
	assert.Len(t, seeded, 10, "the warm-boot snapshot mirrors a default fetch")
	assert.Equal(t, anchor.Add(-14*time.Minute), writtenAt)

	require.NoError(t, store.Save(seeded[:2]))
	saved, savedAt, err := store.Load()
	require.NoError(t, err)
	assert.Len(t, saved, 2)
	assert.False(t, savedAt.IsZero())
}

func TestStateStoreStartsFromDefaults(t *testing.T) {
	a := newTestAdapter(t)

	got, err := a.StateStore().Load()
	require.NoError(t, err)
	assert.Equal(t, domain.DefaultAppState(), got,
		"demo mode must not inherit the user's saved view, sort, or filter")
}

func TestTicketsPortServesFixtureAndFallsBackOnUnknownKey(t *testing.T) {
	a := newTestAdapter(t)
	tk := a.Tickets()

	got, err := tk.GetIssueType(context.Background(), "DEMO-1422")
	require.NoError(t, err)
	assert.Equal(t, "Bug", got)

	// SEC-9 is deliberately absent so the generic ticket icon shows.
	missing, err := tk.GetIssueType(context.Background(), "SEC-9")
	require.NoError(t, err)
	assert.Empty(t, missing)

	keys, err := tk.GetActiveSprintIssueKeys(context.Background(), 1, false)
	require.NoError(t, err)
	assert.Contains(t, keys, "DEMO-1421")
}

func TestSprintFilterNarrowsTheBoard(t *testing.T) {
	a := newTestAdapter(t)

	inSprint := map[string]bool{}
	for _, k := range a.ds.sprintKeys {
		inSprint[k] = true
	}
	var matched int
	for _, mr := range a.ds.all() {
		if inSprint[domain.ExtractJiraID(mr.Title)] {
			matched++
		}
	}
	assert.Positive(t, matched, "the sprint filter must match something")
	assert.Less(t, matched, len(a.ds.all()), "the sprint filter must also exclude something")
}

func TestDiffsAndThreadsExistForTheShowcaseMRs(t *testing.T) {
	a := newTestAdapter(t)
	src := a.MRSource()

	diff, err := src.GetDiff(context.Background(), 101, 415)
	require.NoError(t, err)
	assert.NotEmpty(t, diff.BaseSHA)
	require.NotEmpty(t, diff.Files)

	var sawNew, sawRenamed, sawTooLarge bool
	for _, f := range diff.Files {
		sawNew = sawNew || f.NewFile
		sawRenamed = sawRenamed || f.RenamedFile
		sawTooLarge = sawTooLarge || f.TooLarge
	}
	assert.True(t, sawNew, "the diff should include an added file")
	assert.True(t, sawRenamed, "the diff should include a rename")
	assert.True(t, sawTooLarge, "the diff should include a too-large file")

	desc, threads, err := src.GetDetail(context.Background(), 101, 410)
	require.NoError(t, err)
	assert.Contains(t, desc, "## What")
	require.Len(t, threads, 3)

	var resolved int
	for _, th := range threads {
		if th.Resolved {
			resolved++
		}
		assert.NotEmpty(t, th.Notes)
	}
	assert.Equal(t, 1, resolved, "one thread should be resolved, to show both renderings")
}

// TestEveryMRHasADiff keeps the diff key useful from any card: an MR with no
// diff renders as "no files changed", which reads as a broken demo.
func TestEveryMRHasADiff(t *testing.T) {
	a := newTestAdapter(t)
	src := a.MRSource()

	for _, mr := range a.ds.all() {
		diff, err := src.GetDiff(context.Background(), int64(mr.ProjectID), int64(mr.IID))
		require.NoError(t, err, "MR !%d has no diff", mr.IID)
		assert.NotEmpty(t, diff.Files, "MR !%d has an empty diff", mr.IID)
		assert.NotEmpty(t, diff.BaseSHA, "MR !%d has no base SHA", mr.IID)
	}
}
