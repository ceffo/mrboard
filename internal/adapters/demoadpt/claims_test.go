package demoadpt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

const (
	announceFixture = "announce"
	announceMRIID   = 801
)

func demoClaim(approvers, prior []string) mrsvc.ClaimRequest {
	return mrsvc.ClaimRequest{ProjectID: focusedProject, MRIID: announceMRIID, Approvers: approvers, Prior: prior}
}

func TestAnnounceFixture_TurnsAnnouncementsOn(t *testing.T) {
	a := newTestAdapterFor(t, announceFixture)

	assert.True(t, a.Settings().AnnounceApproverChanges)
	_, approvers := reviewersOf(fetchMR(t, a, focusedProject, announceMRIID))
	assert.Equal(t, []string{userGrace}, approvers, "the editor scenario starts from grace alone")
}

func TestApproverLedger_EstablishedSetOnFirstSight_IsNotOwned(t *testing.T) {
	l := newTestAdapter(t).ApproverClaims()

	out, err := l.Claim(context.Background(), demoClaim([]string{userGrace}, []string{userGrace}))
	require.NoError(t, err)
	assert.Equal(t, mrsvc.ClaimOutcome{}, out)
}

func TestApproverLedger_EditorSave_OwnedOnceWithThePriorItReplaces(t *testing.T) {
	l := newTestAdapter(t).ApproverClaims()
	ctx := context.Background()
	_, err := l.Claim(ctx, demoClaim([]string{userGrace}, []string{userGrace}))
	require.NoError(t, err)

	saved := demoClaim([]string{userGrace, userLinus}, []string{userGrace})
	saved.Authoritative = true
	out, err := l.Claim(ctx, saved)
	require.NoError(t, err)
	assert.Equal(t, mrsvc.ClaimOutcome{Owned: true, Previous: []string{userGrace}}, out)

	out, err = l.Claim(ctx, demoClaim([]string{userGrace, userLinus}, []string{userGrace}))
	require.NoError(t, err)
	assert.False(t, out.Owned, "the refresh confirming the save finds the set already recorded")
}

func TestApproverLedger_ChangeReportsTheRecordedSetNotTheCallerPrior(t *testing.T) {
	l := newTestAdapter(t).ApproverClaims()
	ctx := context.Background()
	_, err := l.Claim(ctx, demoClaim([]string{userGrace}, nil))
	require.NoError(t, err)

	out, err := l.Claim(ctx, demoClaim([]string{userLinus}, []string{userEdsger}))
	require.NoError(t, err)
	assert.Equal(t, mrsvc.ClaimOutcome{Owned: true, Previous: []string{userGrace}}, out)
}

func TestApproverLedger_Release_LetsTheSameSetBeAnnouncedAgain(t *testing.T) {
	l := newTestAdapter(t).ApproverClaims()
	ctx := context.Background()
	_, err := l.Claim(ctx, demoClaim([]string{userGrace}, nil))
	require.NoError(t, err)
	out, err := l.Claim(ctx, demoClaim([]string{userLinus}, []string{userGrace}))
	require.NoError(t, err)
	require.True(t, out.Owned)

	require.NoError(t, l.Release(ctx, focusedProject, announceMRIID, []string{userLinus}))

	out, err = l.Claim(ctx, demoClaim([]string{userLinus}, []string{userGrace}))
	require.NoError(t, err)
	assert.Equal(t, mrsvc.ClaimOutcome{Owned: true, Previous: []string{userGrace}}, out,
		"the released set was never delivered, so the change still starts from grace")
}

func TestApproverLedger_DifferentSetAfterARelease_ReportsTheLastDeliveredSet(t *testing.T) {
	l := newTestAdapter(t).ApproverClaims()
	ctx := context.Background()
	_, err := l.Claim(ctx, demoClaim([]string{userGrace}, nil))
	require.NoError(t, err)
	_, err = l.Claim(ctx, demoClaim([]string{userLinus}, []string{userGrace}))
	require.NoError(t, err)
	require.NoError(t, l.Release(ctx, focusedProject, announceMRIID, []string{userLinus}))

	out, err := l.Claim(ctx, demoClaim([]string{userMargaret}, []string{userLinus}))
	require.NoError(t, err)
	assert.Equal(t, mrsvc.ClaimOutcome{Owned: true, Previous: []string{userGrace}}, out)
}

func TestApproverLedger_KeepsEachMRApart(t *testing.T) {
	l := newTestAdapter(t).ApproverClaims()
	ctx := context.Background()
	_, err := l.Claim(ctx, demoClaim([]string{userGrace}, nil))
	require.NoError(t, err)

	other := demoClaim([]string{userGrace}, nil)
	other.MRIID = announceMRIID + 1
	out, err := l.Claim(ctx, other)
	require.NoError(t, err)
	assert.True(t, out.Owned, "another MR's ledger holds nothing yet")
}
