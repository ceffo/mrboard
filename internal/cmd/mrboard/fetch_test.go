package mrboardcmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
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
			ExcludedAssignees: []string{userBob},
			ExcludeTicketless: true,
		},
	}

	got := boardFilterOptions(state, domain.ViewMine, userAlice, domain.NewTicketKeyMatcher(false))

	assert.True(t, got.MyView)
	assert.Equal(t, userAlice, got.CurrentUser)
	assert.Equal(t, "age", got.SortField)
	assert.True(t, got.SortDesc)
	assert.Equal(t, []string{userBob}, got.ExcludedAssignees)
	assert.True(t, got.ExcludeTicketless)
}

func TestBoardFilterOptions_MineWithoutUserDegradesToAll(t *testing.T) {
	got := boardFilterOptions(domain.AppState{}, domain.ViewMine, "", domain.NewTicketKeyMatcher(false))

	assert.False(t, got.MyView)
}

const (
	testPathMain  = "main.go"
	testPathGoSum = "go.sum"
)

func testSizing(verbose bool) sizeOptions {
	return sizeOptions{
		exclusions: config.ReviewSize{DefaultExcludes: true}.ExclusionsFor(),
		thresholds: domain.DefaultSizeThresholds,
		verbose:    verbose,
	}
}

func TestToMRJSON_ReviewSize(t *testing.T) {
	mr := domain.MergeRequest{DiffStats: []domain.FileDiffStat{
		{Path: testPathMain, Additions: 100, Deletions: 20},
		{Path: testPathGoSum, Additions: 500},
	}}

	got := toMRJSON(mr, "", testSizing(false))

	require.NotNil(t, got.ReviewSize)
	assert.Equal(t, 120, *got.ReviewSize)
	assert.Equal(t, "M", got.SizeBucket)
	assert.Nil(t, got.SizeFiles, "files are listed in verbose mode only")
}

func TestToMRJSON_ReviewSizeVerboseListsFiles(t *testing.T) {
	mr := domain.MergeRequest{DiffStats: []domain.FileDiffStat{
		{Path: testPathMain, Additions: 3},
		{Path: testPathGoSum, Additions: 500},
	}}

	got := toMRJSON(mr, "", testSizing(true))

	require.NotNil(t, got.SizeFiles)
	assert.Equal(t, []domain.FileDiffStat{{Path: testPathMain, Additions: 3}}, got.SizeFiles.Counted)
	assert.Equal(t, []domain.FileDiffStat{{Path: testPathGoSum, Additions: 500}}, got.SizeFiles.Excluded)
}

func TestToMRJSON_UnknownReviewSize(t *testing.T) {
	got := toMRJSON(domain.MergeRequest{}, "", testSizing(true))

	assert.Nil(t, got.ReviewSize)
	assert.Equal(t, "?", got.SizeBucket)
	assert.Nil(t, got.SizeFiles)

	data, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"review_size":null`)
}

func TestToMRJSON_GeneratedRulesExcludeFiles(t *testing.T) {
	mr := domain.MergeRequest{
		DiffStats: []domain.FileDiffStat{
			{Path: testPathMain, Additions: 10},
			{Path: "gen/client.go", Additions: 900},
		},
		GeneratedRules: domain.ParseGeneratedRules("gen/** linguist-generated\n"),
	}

	got := toMRJSON(mr, "", testSizing(true))

	require.NotNil(t, got.ReviewSize)
	assert.Equal(t, 10, *got.ReviewSize, "files the MR's .gitattributes marks generated are not counted")
	require.NotNil(t, got.SizeFiles)
	assert.Equal(t, []domain.FileDiffStat{{Path: "gen/client.go", Additions: 900}}, got.SizeFiles.Excluded)
}

// TestReviewSize_FetchMatchesLoadPerMR checks fetch output and the review load
// measure each MR with the same per-MR exclusions, when two projects' .gitattributes
// mark different files generated.
func TestReviewSize_FetchMatchesLoadPerMR(t *testing.T) {
	stats := []domain.FileDiffStat{
		{Path: "api.gen.go", Additions: 300},
		{Path: "docs/guide.md", Additions: 40},
		{Path: testPathMain, Additions: 20},
	}
	mrs := []domain.MergeRequest{
		{
			IID: 1, ProjectID: 1, Title: "feat(OD-1): a", DiffStats: stats,
			GeneratedRules: domain.ParseGeneratedRules("*.gen.go linguist-generated\n"),
		},
		{
			IID: 2, ProjectID: 2, Title: "feat(OD-1): b", DiffStats: stats,
			GeneratedRules: domain.ParseGeneratedRules("docs/** gitlab-generated\n"),
		},
	}
	sizing := testSizing(false)

	load := mrsvc.ComputeLoad(mrs, mrsvc.LoadOptions{
		SprintKeys: map[string]bool{"OD-1": true},
		Exclusions: sizing.exclusions,
		Thresholds: sizing.thresholds,
		Group:      mrsvc.LoadByTicket,
	})

	require.Len(t, load.Groups, 1)
	require.Len(t, load.Groups[0].Lines, 2)
	loadSize := map[int]int{}
	for _, l := range load.Groups[0].Lines {
		require.True(t, l.Size.Known)
		loadSize[l.MR.IID] = l.Size.Lines
	}
	for _, mr := range mrs {
		got := toMRJSON(mr, "", sizing)
		require.NotNil(t, got.ReviewSize)
		assert.Equal(t, loadSize[mr.IID], *got.ReviewSize, "MR !%d", mr.IID)
	}
	assert.Equal(t, 60, loadSize[1])
	assert.Equal(t, 320, loadSize[2])
}

// contractKeys are the keys every `mrboard fetch` MR object carries, whatever
// the MR looks like. size_files is verbose-only and not part of this list.
var contractKeys = []string{
	"id", "iid", "project_path", "title", "web_url", "head_sha", "updated_at", "phase", "author",
	"reviewer_states", "approvals_required", "time_in_phase", "time_open", "merged_at",
	"round_trips", "roles", "needs_attention", "review_size", "size_bucket",
}

func encodeMR(t *testing.T, mr domain.MergeRequest, currentUser string) map[string]json.RawMessage {
	t.Helper()
	data, err := json.Marshal(toMRJSON(mr, currentUser, testSizing(false)))
	require.NoError(t, err)
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &obj))
	return obj
}

func requireContractKeys(t *testing.T, obj map[string]json.RawMessage) {
	t.Helper()
	for _, k := range contractKeys {
		assert.Contains(t, obj, k)
	}
	assert.NotContains(t, obj, "size_files")
}

func TestToMRJSON_NewFieldsCarryMRValues(t *testing.T) {
	updated := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)
	mr := domain.MergeRequest{IID: 42, ProjectPath: "group/repo", HeadSHA: "abc123", UpdatedAt: updated}

	got := toMRJSON(mr, "", testSizing(false))

	assert.Equal(t, 42, got.IID)
	assert.Equal(t, "group/repo", got.ProjectPath)
	assert.Equal(t, "abc123", got.HeadSHA)
	assert.Equal(t, "2026-05-01T10:30:00Z", got.UpdatedAt)
}

func TestToMRJSON_OpenMRNoReviewersNoUser(t *testing.T) {
	obj := encodeMR(t, domain.MergeRequest{Author: userAlice}, "")

	requireContractKeys(t, obj)
	assert.JSONEq(t, `[]`, string(obj["reviewer_states"]))
	assert.JSONEq(t, `[]`, string(obj["roles"]))
	assert.JSONEq(t, `null`, string(obj["merged_at"]))
	assert.JSONEq(t, `false`, string(obj["needs_attention"]))
	assert.JSONEq(t, `0`, string(obj["approvals_required"]))
}

func TestToMRJSON_MergedMR(t *testing.T) {
	merged := time.Date(2026, 5, 2, 8, 0, 0, 0, time.UTC)
	obj := encodeMR(t, domain.MergeRequest{MergedAt: merged}, "")

	requireContractKeys(t, obj)
	assert.JSONEq(t, `"2026-05-02T08:00:00Z"`, string(obj["merged_at"]))
}

func TestToMRJSON_AuthoredByCurrentUser(t *testing.T) {
	obj := encodeMR(t, domain.MergeRequest{Author: userAlice}, userAlice)

	requireContractKeys(t, obj)
	assert.JSONEq(t, `["author"]`, string(obj["roles"]))
}

func TestToMRJSON_NonApproverReviewerOnApproverMR(t *testing.T) {
	mr := domain.MergeRequest{
		Author: userAlice,
		Reviewers: []domain.ReviewerInfo{
			{Username: userBob, IsApprover: false},
			{Username: "carol", IsApprover: true},
		},
	}

	obj := encodeMR(t, mr, userBob)

	requireContractKeys(t, obj)
	assert.JSONEq(t, `[]`, string(obj["roles"]))
	assert.JSONEq(t, `1`, string(obj["approvals_required"]))
	assert.Contains(t, string(obj["reviewer_states"]), `"is_approver":false`)
	assert.Contains(t, string(obj["reviewer_states"]), `"is_approver":true`)
}
