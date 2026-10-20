package demoadpt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

// TestFixtureMRsCarryDiffStats checks every demo MR reports a known review size
// that matches the diff the diff view shows.
func TestFixtureMRsCarryDiffStats(t *testing.T) {
	forEachFixture(t, func(t *testing.T, a *Adapter) {
		for _, mr := range a.ds.all() {
			diff, ok := a.ds.diffFor(mr.Key())
			require.True(t, ok)
			assert.Equal(t, diff.HeadSHA, mr.HeadSHA)
			require.Len(t, mr.DiffStats, len(diff.Files))
			for i, f := range diff.Files {
				assert.Equal(t, f.NewPath, mr.DiffStats[i].Path)
				assert.Equal(t, f.LinesAdded, mr.DiffStats[i].Additions)
				assert.Equal(t, f.LinesRemoved, mr.DiffStats[i].Deletions)
			}
		}
	})
}

// TestReviewSizeFixture_GeneratedRules checks a project's .gitattributes reaches
// its MRs and excludes the files it marks generated, and only those.
func TestReviewSizeFixture_GeneratedRules(t *testing.T) {
	ds, err := loadFixture("review-size", time.Now(), "https://gitlab.demo.invalid")
	require.NoError(t, err)

	sizes := map[int]int{}
	for _, mr := range ds.all() {
		m := mr.MeasureReviewSize(domain.NewSizeExclusions(true, nil, mr.GeneratedRules))
		require.True(t, m.Size.Known)
		sizes[mr.IID] = m.Size.Lines
	}

	assert.Equal(t, map[int]int{601: 20, 602: 50}, sizes)
}
