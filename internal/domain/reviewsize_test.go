package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	globGoSum  = "go.sum"
	globVendor = "**/vendor/**"
)

func TestMatchSizeGlob(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"*.lock", "Cargo.lock", true},
		{"*.lock", "deep/dir/yarn.lock", true},
		{globGoSum, "tools/go.sum", true},
		{"*.min.js", "web/app.min.js", true},
		{"*.min.js", "web/app.js", false},
		{globVendor, "vendor/x/y.go", true},
		{globVendor, "a/b/vendor/x.go", true},
		{globVendor, "vendored/x.go", false},
		{"docs/**", "docs/a/b.md", true},
		{"docs/**", "src/docs/b.md", false},
		{"gen/", "gen/a.go", true},
		{"/foo.pb.go", "foo.pb.go", true},
		{"/foo.pb.go", "api/foo.pb.go", false},
		{"foo.pb.go", "api/foo.pb.go", true},
		{"/api/*.go", "api/x.go", true},
		{"/api/*.go", "api/sub/x.go", false},
		{"internal/**/mocks/*.go", "internal/mocks/m.go", true},
		{"internal/**/mocks/*.go", "internal/a/b/mocks/m.go", true},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, matchSizeGlob(c.glob, c.path), "glob %q path %q", c.glob, c.path)
	}
}

func TestMeasureReviewSize(t *testing.T) {
	ex := NewSizeExclusions(true, []string{"testdata/**"},
		[]GeneratedRule{{Glob: "*.gen.ts", Attr: AttrLinguistGenerated, State: AttrSet}})
	stats := []FileDiffStat{
		{Path: "main.go", Additions: 10, Deletions: 5},
		{Path: "main_test.go", Additions: 20, Deletions: 0},
		{Path: globGoSum, Additions: 300, Deletions: 100},
		{Path: "testdata/big.json", Additions: 900, Deletions: 0},
		{Path: "web/api.gen.ts", Additions: 50, Deletions: 50},
	}

	m := ex.MeasureReviewSize(stats)

	assert.Equal(t, ReviewSize{Lines: 35, Known: true}, m.Size, "tests count; lock, configured and generated files don't")
	assert.Len(t, m.Counted, 2)
	assert.Len(t, m.Excluded, 3)
}

func TestMeasureReviewSize_NoDefaults(t *testing.T) {
	m := NewSizeExclusions(false, nil, nil).MeasureReviewSize([]FileDiffStat{{Path: globGoSum, Additions: 4}})
	assert.Equal(t, ReviewSize{Lines: 4, Known: true}, m.Size)
}

func TestMeasureReviewSize_UnknownVersusEmpty(t *testing.T) {
	ex := NewSizeExclusions(true, nil, nil)
	assert.Equal(t, UnknownReviewSize, ex.MeasureReviewSize(nil).Size)
	assert.Equal(t, ReviewSize{Known: true}, ex.MeasureReviewSize([]FileDiffStat{}).Size)
}

func TestSizeThresholdsBucket(t *testing.T) {
	th := DefaultSizeThresholds
	cases := []struct {
		size ReviewSize
		want SizeBucket
	}{
		{UnknownReviewSize, SizeBucketUnknown},
		{ReviewSize{Lines: 0, Known: true}, SizeBucketS},
		{ReviewSize{Lines: 100, Known: true}, SizeBucketS},
		{ReviewSize{Lines: 101, Known: true}, SizeBucketM},
		{ReviewSize{Lines: 400, Known: true}, SizeBucketM},
		{ReviewSize{Lines: 1000, Known: true}, SizeBucketL},
		{ReviewSize{Lines: 1001, Known: true}, SizeBucketXL},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, th.Bucket(c.size), "size %+v", c.size)
	}
}

func TestSizeThresholdsValidate(t *testing.T) {
	require.NoError(t, DefaultSizeThresholds.Validate())
	assert.Error(t, SizeThresholds{S: 0, M: 1, L: 2}.Validate())
	assert.Error(t, SizeThresholds{S: 5, M: 5, L: 9}.Validate())
	assert.Error(t, SizeThresholds{S: 1, M: 9, L: 3}.Validate())
}

func TestValidateSizeGlob(t *testing.T) {
	for _, g := range DefaultSizeExcludeGlobs {
		assert.NoError(t, ValidateSizeGlob(g), g)
	}
	assert.Error(t, ValidateSizeGlob(""))
	assert.Error(t, ValidateSizeGlob("src/[.go"))
}
