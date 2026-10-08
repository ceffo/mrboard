package gitlabadpt

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// fakeLedgerClient keeps an MR's notes in memory with ascending IDs. The
// embedded interface is nil, so any call outside the ledger path panics.
type fakeLedgerClient struct {
	gitLabClient
	notes     []*gl.Note
	nextID    int64
	liveRules []*gl.MergeRequestApprovalRule
	// beforeCreate runs just before each CreateMRNote, letting a test play a
	// rival instance that appends its own entry first.
	beforeCreate func(*fakeLedgerClient)
	accessLevel  int
	// createsPublic plays GitLab ignoring the internal flag.
	createsPublic bool
	deleted       []int64
}

func newFakeLedgerClient(live ...string) *fakeLedgerClient {
	return &fakeLedgerClient{
		nextID:      100,
		liveRules:   []*gl.MergeRequestApprovalRule{approvalRule("approvers", live...)},
		accessLevel: int(gl.DeveloperPermissions),
	}
}

func (f *fakeLedgerClient) add(body string, system bool) {
	f.nextID++
	f.notes = append(f.notes, &gl.Note{ID: f.nextID, Body: body, System: system, Internal: !f.createsPublic})
}

func (f *fakeLedgerClient) CurrentUserAccessLevel(_ context.Context, _ int64) (int, error) {
	return f.accessLevel, nil
}

func (f *fakeLedgerClient) DeleteMRNote(_ context.Context, _, _, noteID int64) error {
	f.deleted = append(f.deleted, noteID)
	f.notes = slices.DeleteFunc(f.notes, func(n *gl.Note) bool { return n.ID == noteID })
	return nil
}

func (f *fakeLedgerClient) ListMRNotes(_ context.Context, _, _ int64) ([]*gl.Note, error) {
	return append([]*gl.Note(nil), f.notes...), nil
}

func (f *fakeLedgerClient) CreateMRNote(_ context.Context, _, _ int64, body string, internal bool) (*gl.Note, error) {
	if f.beforeCreate != nil {
		hook := f.beforeCreate
		f.beforeCreate = nil
		hook(f)
	}
	if !internal {
		panic("ledger entries must be internal notes")
	}
	f.add(body, false)
	return f.notes[len(f.notes)-1], nil
}

func (f *fakeLedgerClient) GetMRApprovalRules(_ context.Context, _, _ int64) ([]*gl.MergeRequestApprovalRule, error) {
	return f.liveRules, nil
}

func (f *fakeLedgerClient) claims() []domain.ApproverClaim { return claimsFromNotes(f.notes) }

func claimReq(approvers, prior []string) mrsvc.ClaimRequest {
	return mrsvc.ClaimRequest{ProjectID: 1, MRIID: 2, Approvers: approvers, Prior: prior}
}

func TestClaim_FirstSightOfNewMR_Announces(t *testing.T) {
	c := newFakeLedgerClient(testUserAlice)
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
	require.NoError(t, err)
	assert.True(t, owned)
	require.Len(t, c.claims(), 1)
	assert.False(t, c.claims()[0].Silent)
}

func TestClaim_FirstSightOfOldMR_WritesNothing(t *testing.T) {
	c := newFakeLedgerClient(testUserAlice)
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, []string{testUserAlice}))
	require.NoError(t, err)
	assert.False(t, owned)
	assert.Empty(t, c.notes, "every note emails the MR's participants, so nothing to announce means no note")
}

func TestClaim_NoApproversAndNoHistory_WritesNothing(t *testing.T) {
	c := newFakeLedgerClient()
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq(nil, nil))
	require.NoError(t, err)
	assert.False(t, owned)
	assert.Empty(t, c.notes)
}

func TestClaim_ChangeOnEmptyLedger_WritesOneEntry(t *testing.T) {
	c := newFakeLedgerClient(testUserBob)
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserBob}, []string{testUserAlice}))
	require.NoError(t, err)
	assert.True(t, owned)
	require.Len(t, c.claims(), 1)
	assert.False(t, c.claims()[0].Silent)
}

func TestClaim_AllApproversRemovedOnEmptyLedger_Announces(t *testing.T) {
	c := newFakeLedgerClient()
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq(nil, []string{testUserAlice}))
	require.NoError(t, err)
	assert.True(t, owned)
	assert.Len(t, c.claims(), 1)
}

// A rival that met the MR only after the change records nothing, so it cannot
// slip an entry between the change and its claim and steal the announcement.
func TestClaim_RivalDiscoversMRAfterChange_ChangeStillAnnounced(t *testing.T) {
	c := newFakeLedgerClient(testUserBob)
	rival := &GitLabAdapter{client: c}
	c.beforeCreate = func(*fakeLedgerClient) {
		owned, err := rival.Claim(context.Background(), claimReq([]string{testUserBob}, []string{testUserBob}))
		require.NoError(t, err)
		assert.False(t, owned)
	}
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserBob}, []string{testUserAlice}))
	require.NoError(t, err)
	assert.True(t, owned)
	assert.Len(t, c.claims(), 1)
}

func TestClaim_LegacySilentEntry_StillBaselinesTheLedger(t *testing.T) {
	c := newFakeLedgerClient(testUserAlice)
	c.add(domain.FormatApproverClaim(domain.ApproverClaim{Approvers: []string{testUserAlice}, Silent: true}), false)
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
	require.NoError(t, err)
	assert.False(t, owned, "the silent entry already recorded this set")
	assert.Len(t, c.notes, 1)
}

func TestClaim_SetAlreadyRecorded_WritesNothing(t *testing.T) {
	c := newFakeLedgerClient(testUserAlice)
	c.add(domain.FormatApproverClaim(domain.ApproverClaim{Approvers: []string{testUserAlice}}), false)
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
	require.NoError(t, err)
	assert.False(t, owned)
	assert.Len(t, c.notes, 1)
}

func TestClaim_ChangeBackToEarlierSet_Announces(t *testing.T) {
	c := newFakeLedgerClient(testUserAlice)
	c.add(domain.FormatApproverClaim(domain.ApproverClaim{Approvers: []string{testUserAlice}}), false)
	c.add(domain.FormatApproverClaim(domain.ApproverClaim{Approvers: []string{testUserBob}}), false)
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
	require.NoError(t, err)
	assert.True(t, owned)
}

func TestClaim_RivalInstanceAppendsFirst_LoserDoesNotOwn(t *testing.T) {
	c := newFakeLedgerClient(testUserAlice)
	c.beforeCreate = func(f *fakeLedgerClient) {
		f.add(domain.FormatApproverClaim(domain.ApproverClaim{Approvers: []string{testUserAlice}}), false)
	}
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
	require.NoError(t, err)
	assert.False(t, owned)
	assert.Len(t, c.claims(), 2, "the loser's entry stays in the ledger as a redundant record")
}

func TestClaim_StaleObservation_WritesNothing(t *testing.T) {
	c := newFakeLedgerClient(testUserBob) // live set has moved on from what the caller saw
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
	require.NoError(t, err)
	assert.False(t, owned)
	assert.Empty(t, c.notes)
}

func TestClaim_IgnoresSystemNotesAndPlainComments(t *testing.T) {
	c := newFakeLedgerClient(testUserAlice)
	c.add(domain.FormatApproverClaim(domain.ApproverClaim{Approvers: []string{testUserAlice}}), true)
	c.add("looks good", false)
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
	require.NoError(t, err)
	assert.True(t, owned, "a system note cannot carry a claim")
}

func TestRelease_LetsTheNextObserverAnnounceTheSameSet(t *testing.T) {
	c := newFakeLedgerClient(testUserAlice)
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
	require.NoError(t, err)
	require.True(t, owned)
	require.NoError(t, a.Release(context.Background(), 1, 2, []string{testUserAlice}))

	owned, err = a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
	require.NoError(t, err)
	assert.True(t, owned)
}

func TestClaim_BelowPlannerAccess_WritesNothing(t *testing.T) {
	for name, level := range map[string]int{"guest": int(gl.GuestPermissions), "non-member": 0} {
		t.Run(name, func(t *testing.T) {
			c := newFakeLedgerClient(testUserAlice)
			c.accessLevel = level
			a := &GitLabAdapter{client: c}

			owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
			require.ErrorIs(t, err, mrsvc.ErrClaimNotPermitted)
			assert.False(t, owned)
			assert.Empty(t, c.notes)
		})
	}
}

func TestClaim_NoteCreatedPublic_IsDeletedAndRefused(t *testing.T) {
	c := newFakeLedgerClient(testUserAlice)
	c.createsPublic = true
	a := &GitLabAdapter{client: c}

	owned, err := a.Claim(context.Background(), claimReq([]string{testUserAlice}, nil))
	require.ErrorIs(t, err, mrsvc.ErrClaimNotPermitted)
	assert.False(t, owned)
	assert.Empty(t, c.notes)
	assert.Len(t, c.deleted, 1)
}

func TestRelease_BelowPlannerAccess_WritesNothing(t *testing.T) {
	c := newFakeLedgerClient(testUserAlice)
	c.accessLevel = int(gl.GuestPermissions)
	a := &GitLabAdapter{client: c}

	err := a.Release(context.Background(), 1, 2, []string{testUserAlice})
	require.ErrorIs(t, err, mrsvc.ErrClaimNotPermitted)
	assert.Empty(t, c.notes)
}

func TestClaim_Authoritative_SkipsTheLiveCheck(t *testing.T) {
	c := newFakeLedgerClient() // the live read still shows the set from before the write
	a := &GitLabAdapter{client: c}

	req := claimReq([]string{testUserAlice}, nil)
	req.Authoritative = true
	owned, err := a.Claim(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, owned)
}
