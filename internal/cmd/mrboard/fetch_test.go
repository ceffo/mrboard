package mrboardcmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

const (
	testPathMain  = "main.go"
	testPathGoSum = "go.sum"
)

func testSizing(verbose bool) sizeOptions {
	return sizeOptions{
		exclusions: domain.UniformSizeExclusions(domain.NewSizeExclusions(true, nil, nil)),
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

// TestReviewSize_FetchMatchesLoadPerMR checks fetch output and the review load
// measure each MR with the same per-MR exclusions, when two projects mark
// different files generated.
func TestReviewSize_FetchMatchesLoadPerMR(t *testing.T) {
	stats := []domain.FileDiffStat{{Path: "api.gen.go", Additions: 300}, {Path: testPathMain, Additions: 20}}
	mrs := []domain.MergeRequest{
		{IID: 1, ProjectID: 1, Title: "feat(OD-1): a", DiffStats: stats},
		{IID: 2, ProjectID: 2, Title: "feat(OD-1): b", DiffStats: stats},
	}
	sizing := testSizing(false)
	sizing.exclusions = func(mr domain.MergeRequest) domain.SizeExclusions {
		if mr.ProjectID == 1 {
			return domain.NewSizeExclusions(true, nil, []string{"*.gen.go"})
		}
		return domain.NewSizeExclusions(true, nil, nil)
	}

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
	assert.Equal(t, 20, loadSize[1])
	assert.Equal(t, 320, loadSize[2])
}
