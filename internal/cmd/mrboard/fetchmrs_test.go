package mrboardcmd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/core"
	"github.com/ceffo/mrboard/internal/domain"
	domainmocks "github.com/ceffo/mrboard/internal/domain/mocks"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	mrsvcmocks "github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

var (
	testSince    = time.Date(2026, 10, 12, 0, 0, 0, 0, time.Local)
	testOpenMR   = domain.MergeRequest{ProjectID: 1, IID: 1}
	testMergedMR = domain.MergeRequest{ProjectID: 1, IID: 2, MergedAt: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
)

type fetchMocks struct {
	src    *mrsvcmocks.MockMergeRequestSource
	board  *domainmocks.MockSnapshotStore
	merged *domainmocks.MockMergedSnapshotStore
	core   *core.Core
}

func newFetchMocks(t *testing.T) fetchMocks {
	m := fetchMocks{
		src:    mrsvcmocks.NewMockMergeRequestSource(t),
		board:  domainmocks.NewMockSnapshotStore(t),
		merged: domainmocks.NewMockMergedSnapshotStore(t),
	}
	m.core = &core.Core{MRSource: m.src, SnapshotStore: m.board, MergedSnapshotStore: m.merged}
	return m
}

func TestFetchMRs_BoardOnlyNeverTouchesMergedSnapshot(t *testing.T) {
	m := newFetchMocks(t)
	m.board.EXPECT().Load().Return([]domain.MergeRequest{testOpenMR}, time.Time{}, nil)
	m.src.EXPECT().FetchAll(mock.Anything, mrsvc.FetchOptions{
		IncludeReviewerMRs: true, Previous: []domain.MergeRequest{testOpenMR},
	}).Return([]domain.MergeRequest{testOpenMR}, nil)

	mrs, errs, err := fetchMRs(context.Background(), m.core, fetchRequest{includeReviewerMRs: true})

	require.NoError(t, err)
	assert.Empty(t, errs)
	assert.Equal(t, []domain.MergeRequest{testOpenMR}, mrs)
}

func TestFetchMRs_MergedSinceDiffsAgainstAndRefreshesMergedSnapshot(t *testing.T) {
	m := newFetchMocks(t)
	m.board.EXPECT().Load().Return([]domain.MergeRequest{testOpenMR}, time.Time{}, nil)
	m.merged.EXPECT().Load(testSince).Return([]domain.MergeRequest{testMergedMR}, nil)
	m.src.EXPECT().FetchAll(mock.Anything, mrsvc.FetchOptions{
		Previous: []domain.MergeRequest{testOpenMR, testMergedMR}, MergedSince: testSince,
	}).Return([]domain.MergeRequest{testOpenMR, testMergedMR}, nil)
	m.merged.EXPECT().Save(testSince, []domain.MergeRequest{testMergedMR}).Return(nil)

	mrs, errs, err := fetchMRs(context.Background(), m.core, fetchRequest{mergedSince: testSince})

	require.NoError(t, err)
	assert.Empty(t, errs)
	assert.Len(t, mrs, 2)
}

func TestFetchMRs_ColdBypassesBothSnapshotsButStoresFreshMerged(t *testing.T) {
	m := newFetchMocks(t)
	m.src.EXPECT().FetchAll(mock.Anything, mrsvc.FetchOptions{Cold: true, MergedSince: testSince}).
		Return([]domain.MergeRequest{testMergedMR}, nil)
	m.merged.EXPECT().Save(testSince, []domain.MergeRequest{testMergedMR}).Return(nil)

	_, errs, err := fetchMRs(context.Background(), m.core, fetchRequest{cold: true, mergedSince: testSince})

	require.NoError(t, err)
	assert.Empty(t, errs)
}

func TestFetchMRs_FailedFetchWithNoMergedKeepsMergedSnapshot(t *testing.T) {
	m := newFetchMocks(t)
	m.board.EXPECT().Load().Return(nil, time.Time{}, nil)
	m.merged.EXPECT().Load(testSince).Return([]domain.MergeRequest{testMergedMR}, nil)
	m.src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(nil, []error{errors.New("merged listing down")})

	_, errs, err := fetchMRs(context.Background(), m.core, fetchRequest{mergedSince: testSince})

	require.NoError(t, err)
	assert.Len(t, errs, 1)
}

func TestParseMergedSince(t *testing.T) {
	got, err := parseMergedSince("2026-10-12")
	require.NoError(t, err)
	assert.Equal(t, testSince, got)

	_, err = parseMergedSince("12/10/2026")
	assert.ErrorContains(t, err, "YYYY-MM-DD")
}

func TestToMRJSON_MergedAt(t *testing.T) {
	assert.Equal(t, "2026-10-14T00:00:00Z", toMRJSON(testMergedMR, "", testSizing(false)).MergedAt)
	assert.Empty(t, toMRJSON(testOpenMR, "", testSizing(false)).MergedAt)
}
