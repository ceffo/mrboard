package mrboardcmd

import (
	"encoding/json"
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

const (
	testPathMain  = "main.go"
	testPathGoSum = "go.sum"
)

func testSizing(verbose bool) sizeOptions {
	return sizeOptions{
		exclusions: domain.NewSizeExclusions(true, nil, nil),
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
