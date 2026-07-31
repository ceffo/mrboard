package gitlabadpt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// gqlApprovalRule builds a GQLApprovalRule for ApprovalState.Rules.
func gqlApprovalRule(name string, usernames ...string) pkggitlab.GQLApprovalRule {
	eligible := make([]pkggitlab.GQLUser, len(usernames))
	for i, u := range usernames {
		eligible[i] = pkggitlab.GQLUser{Username: u, Name: u}
	}
	return pkggitlab.GQLApprovalRule{Name: name, EligibleApprovers: eligible}
}

// TestMergeMRFromGraphQL_UnchangedMRReusesDiscussionDerivedFields verifies the
// core merge rule from docs/adr/0005 "What the cache is allowed to answer for":
// on a cache hit, Reviewers, OpenThreads, and RoundTripCount come from the
// cached MR untouched (no discussions were fetched to recompute them).
func TestMergeMRFromGraphQL_UnchangedMRReusesDiscussionDerivedFields(t *testing.T) {
	cached := domain.MergeRequest{
		Reviewers: []domain.ReviewerInfo{
			{Username: testUserAlice, Name: testUserAliceName, State: domain.ReviewerCommented},
		},
		OpenThreads:    4,
		RoundTripCount: 3,
	}
	mr := pkggitlab.GQLMergeRequest{}
	mr.DetailedMergeStatus = detailedMergeStatusMergeable
	mr.Reviewers.Nodes = []pkggitlab.GQLUser{{Username: testUserAlice, Name: testUserAliceName}}

	result := MergeMRFromGraphQL(mr, cached)

	assert.Equal(t, cached.OpenThreads, result.OpenThreads)
	assert.Equal(t, cached.RoundTripCount, result.RoundTripCount)
	require.Len(t, result.Reviewers, 1)
	assert.Equal(t, domain.ReviewerCommented, result.Reviewers[0].State, "reviewer state must come from cache")
}

// TestMergeMRFromGraphQL_DetailedMergeStatusAlwaysFresh verifies phase-1's
// DetailedMergeStatus always overwrites the cached value, per the ADR: it is
// not covered by updatedAt and can silently change (mergeable -> conflict).
func TestMergeMRFromGraphQL_DetailedMergeStatusAlwaysFresh(t *testing.T) {
	cached := domain.MergeRequest{DetailedMergeStatus: detailedMergeStatusMergeable}
	mr := pkggitlab.GQLMergeRequest{}
	mr.DetailedMergeStatus = "CONFLICT"

	result := MergeMRFromGraphQL(mr, cached)

	assert.Equal(t, "conflict", result.DetailedMergeStatus, "fresh phase-1 status must win over the cached one")
}

// TestMergeMRFromGraphQL_DraftAlwaysFresh verifies a changed draft flag still
// moves the MR to/from PhaseDraft on a cache hit, even though Reviewers itself
// is reused from cache — Draft is phase-1's freshest signal and ClassifyPhase's
// first check.
func TestMergeMRFromGraphQL_DraftAlwaysFresh(t *testing.T) {
	cached := domain.MergeRequest{Phase: domain.PhaseNeedsReview}
	mr := pkggitlab.GQLMergeRequest{}
	mr.Draft = true

	result := MergeMRFromGraphQL(mr, cached)

	assert.Equal(t, domain.PhaseDraft, result.Phase, "fresh draft=true must move the MR to PhaseDraft")
}

// TestMergeMRFromGraphQL_IsApproverRecomputedFromFreshRules verifies IsApprover
// is recomputed from phase-1's approvalState.rules rather than reused from
// cache — the ADR keeps that resolver always-fresh specifically so a teammate's
// approver edit in the GitLab web UI (which doesn't bump the MR's updatedAt) is
// still visible on a cache hit.
func TestMergeMRFromGraphQL_IsApproverRecomputedFromFreshRules(t *testing.T) {
	cached := domain.MergeRequest{
		Reviewers: []domain.ReviewerInfo{
			{Username: testUserAlice, Name: testUserAliceName, State: domain.ReviewerApproved, IsApprover: false},
		},
	}
	mr := pkggitlab.GQLMergeRequest{}
	mr.DetailedMergeStatus = detailedMergeStatusMergeable
	mr.ApprovalState.Rules = []pkggitlab.GQLApprovalRule{gqlApprovalRule(approversRuleName, testUserAlice)}

	result := MergeMRFromGraphQL(mr, cached)

	require.Len(t, result.Reviewers, 1)
	assert.True(t, result.Reviewers[0].IsApprover, "IsApprover must reflect fresh approvalState.rules")
	assert.Equal(t, domain.ReviewerApproved, result.Reviewers[0].State, "State must still come from cache")
	assert.Equal(t, domain.PhaseReadyToMerge, result.Phase,
		"once the freshly-fresh Approvers rule matches an already-approved reviewer, the MR is ready to merge")
}

// TestMergeMRFromGraphQL_CachedReviewersNotMutated verifies the merge clones
// cached.Reviewers before applying the fresh approver flag, so repeated merges
// against the same Previous snapshot don't corrupt it via aliasing.
func TestMergeMRFromGraphQL_CachedReviewersNotMutated(t *testing.T) {
	cached := domain.MergeRequest{
		Reviewers: []domain.ReviewerInfo{{Username: testUserAlice, Name: testUserAliceName, IsApprover: false}},
	}
	mr := pkggitlab.GQLMergeRequest{}
	mr.ApprovalState.Rules = []pkggitlab.GQLApprovalRule{gqlApprovalRule(approversRuleName, testUserAlice)}

	_ = MergeMRFromGraphQL(mr, cached)

	assert.False(t, cached.Reviewers[0].IsApprover, "the cached snapshot passed in must not be mutated")
}
