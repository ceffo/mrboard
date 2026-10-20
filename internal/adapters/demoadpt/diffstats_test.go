package demoadpt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
