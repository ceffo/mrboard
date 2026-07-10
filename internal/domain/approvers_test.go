package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func mrWithApprovers(usernames ...string) MergeRequest {
	reviewers := make([]ReviewerInfo, len(usernames))
	for i, u := range usernames {
		reviewers[i] = ReviewerInfo{Username: u, IsApprover: true}
	}
	return MergeRequest{Reviewers: reviewers}
}

func TestApproversConflict_SameSet_NoConflict(t *testing.T) {
	a := mrWithApprovers("alice", "bob")
	b := mrWithApprovers("bob", "alice") // order must not matter
	assert.False(t, ApproversConflict(a, b), "identical approver sets should not conflict")
}

func TestApproversConflict_DifferentMembers_Conflict(t *testing.T) {
	a := mrWithApprovers("alice", "bob")
	b := mrWithApprovers("alice", "carol")
	assert.True(t, ApproversConflict(a, b), "different approver sets should conflict")
}

func TestApproversConflict_DifferentSize_Conflict(t *testing.T) {
	a := mrWithApprovers("alice", "bob")
	b := mrWithApprovers("alice")
	assert.True(t, ApproversConflict(a, b), "approver sets of different sizes should conflict")
}

func TestApproversConflict_BothEmpty_NoConflict(t *testing.T) {
	a := MergeRequest{Reviewers: []ReviewerInfo{{Username: "dave"}}} // reviewer, not approver
	b := MergeRequest{}
	assert.False(t, ApproversConflict(a, b), "MRs with no designated approvers should not conflict")
}

func TestApproversConflict_NonApproverReviewersIgnored(t *testing.T) {
	a := MergeRequest{Reviewers: []ReviewerInfo{
		{Username: "alice", IsApprover: true},
		{Username: "eve", IsApprover: false},
	}}
	b := mrWithApprovers("alice")
	assert.False(t, ApproversConflict(a, b), "a non-approver reviewer must not affect the approver-set comparison")
}
