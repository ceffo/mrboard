package mrsvc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	domainmocks "github.com/ceffo/mrboard/internal/domain/mocks"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

var errDelivery = errors.New("webhook down")

func announceFixture(t *testing.T) (*mocks.MockApproverClaims, *domainmocks.MockNotifier, mrsvc.ClaimRequest) {
	t.Helper()
	req := mrsvc.ClaimRequest{ProjectID: 1, MRIID: 2, Approvers: []string{userAlice}}
	return mocks.NewMockApproverClaims(t), domainmocks.NewMockNotifier(t), req
}

func TestAnnounceApproverChange_OwnerDelivers(t *testing.T) {
	claims, notifier, req := announceFixture(t)
	claims.EXPECT().Claim(mock.Anything, req).Return(true, nil)
	notifier.EXPECT().Notify(mock.Anything, mock.Anything).Return(nil)

	announced, err := mrsvc.AnnounceApproverChange(context.Background(), claims, notifier, domain.MergeRequest{}, req)
	require.NoError(t, err)
	assert.True(t, announced)
}

func TestAnnounceApproverChange_NotOwner_DoesNotNotify(t *testing.T) {
	claims, notifier, req := announceFixture(t)
	claims.EXPECT().Claim(mock.Anything, req).Return(false, nil)

	announced, err := mrsvc.AnnounceApproverChange(context.Background(), claims, notifier, domain.MergeRequest{}, req)
	require.NoError(t, err)
	assert.False(t, announced)
}

func TestAnnounceApproverChange_ClaimError_DoesNotNotify(t *testing.T) {
	claims, notifier, req := announceFixture(t)
	claims.EXPECT().Claim(mock.Anything, req).Return(false, errDelivery)

	announced, err := mrsvc.AnnounceApproverChange(context.Background(), claims, notifier, domain.MergeRequest{}, req)
	assert.ErrorIs(t, err, errDelivery)
	assert.False(t, announced)
}

func TestAnnounceApproverChange_DeliveryFails_ReleasesTheClaim(t *testing.T) {
	claims, notifier, req := announceFixture(t)
	claims.EXPECT().Claim(mock.Anything, req).Return(true, nil)
	notifier.EXPECT().Notify(mock.Anything, mock.Anything).Return(errDelivery)
	claims.EXPECT().Release(mock.Anything, req.ProjectID, req.MRIID, req.Approvers).Return(nil)

	announced, err := mrsvc.AnnounceApproverChange(context.Background(), claims, notifier, domain.MergeRequest{}, req)
	assert.ErrorIs(t, err, errDelivery)
	assert.False(t, announced)
}

func TestAnnounceApproverChange_ReleaseRunsAfterContextIsCancelled(t *testing.T) {
	claims, notifier, req := announceFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	claims.EXPECT().Claim(mock.Anything, req).Return(true, nil)
	notifier.EXPECT().Notify(mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, domain.MergeRequest) error {
			cancel()
			return errDelivery
		})
	claims.EXPECT().Release(mock.Anything, req.ProjectID, req.MRIID, req.Approvers).RunAndReturn(
		func(ctx context.Context, _, _ int64, _ []string) error { return ctx.Err() })

	_, err := mrsvc.AnnounceApproverChange(ctx, claims, notifier, domain.MergeRequest{}, req)
	assert.ErrorIs(t, err, errDelivery)
	assert.NotErrorIs(t, err, context.Canceled, "the release must not inherit the cancelled context")
}

func TestDiscoveryPrior(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	window := 24 * time.Hour
	mr := domain.MergeRequest{Approvers: []string{userAlice}, CreatedAt: now.Add(-time.Hour)}

	assert.Empty(t, mrsvc.DiscoveryPrior(mr, now, window), "a young MR's approvers are news")

	mr.CreatedAt = now.Add(-48 * time.Hour)
	assert.Equal(t, []string{userAlice}, mrsvc.DiscoveryPrior(mr, now, window), "an old MR's approvers are established")

	mr.CreatedAt = time.Time{}
	assert.Equal(t, []string{userAlice}, mrsvc.DiscoveryPrior(mr, now, window), "an unknown age counts as old")
}
