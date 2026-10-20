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

const reviewSizeFixture = "review-size"

func iidsOf(mrs []domain.MergeRequest) []int {
	out := make([]int, 0, len(mrs))
	for _, mr := range mrs {
		out = append(out, mr.IID)
	}
	return out
}

func TestFetchAll_MergedListedOnlyWhenMergedSinceReachesThem(t *testing.T) {
	src := newTestAdapterFor(t, reviewSizeFixture).MRSource()
	ctx := context.Background()

	board, errs := src.FetchAll(ctx, mrsvc.FetchOptions{})
	assert.Empty(t, errs)
	assert.ElementsMatch(t, []int{601, 602}, iidsOf(board), "a board fetch lists open MRs only")

	since, errs := src.FetchAll(ctx, mrsvc.FetchOptions{MergedSince: anchor.AddDate(0, 0, -3)})
	assert.Empty(t, errs)
	assert.ElementsMatch(t, []int{601, 602, 603}, iidsOf(since))

	tooLate, errs := src.FetchAll(ctx, mrsvc.FetchOptions{MergedSince: anchor.AddDate(0, 0, -1)})
	assert.Empty(t, errs)
	assert.ElementsMatch(t, []int{601, 602}, iidsOf(tooLate), "an MR merged before the bound is not listed")
}

func TestSnapshotStore_HoldsOpenMRsOnly(t *testing.T) {
	a := newTestAdapterFor(t, reviewSizeFixture)
	store := a.SnapshotStore()

	seeded, _, err := store.Load()
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{601, 602}, iidsOf(seeded), "the seeded board holds no merged MR")

	all, _ := a.MRSource().FetchAll(context.Background(), mrsvc.FetchOptions{MergedSince: anchor.AddDate(0, 0, -3)})
	require.NoError(t, store.Save(all))
	saved, _, err := store.Load()
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{601, 602}, iidsOf(saved))
}

func TestMergedSnapshotStore_TaggedWithSince(t *testing.T) {
	a := newTestAdapterFor(t, reviewSizeFixture)
	store := a.MergedSnapshotStore()
	since := anchor.AddDate(0, 0, -3)

	cold, err := store.Load(since)
	require.NoError(t, err)
	assert.Empty(t, cold, "the demo merged snapshot starts empty")

	all, _ := a.MRSource().FetchAll(context.Background(), mrsvc.FetchOptions{MergedSince: since})
	require.NoError(t, store.Save(since, all))
	saved, err := store.Load(since)
	require.NoError(t, err)
	assert.Equal(t, []int{603}, iidsOf(saved), "only merged MRs are stored")

	other, err := store.Load(since.Add(24 * time.Hour))
	require.NoError(t, err)
	assert.Empty(t, other)
	dropped, err := store.Load(since)
	require.NoError(t, err)
	assert.Empty(t, dropped, "loading for another start drops the snapshot")
}
