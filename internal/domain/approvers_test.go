package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func mrWithApprovers(usernames ...string) MergeRequest {
	return MergeRequest{Approvers: usernames}
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
	a := MergeRequest{Reviewers: []ReviewerInfo{{Username: "dave"}}} // reviewer, no approver rule
	b := MergeRequest{}
	assert.False(t, ApproversConflict(a, b), "MRs with no eligible approvers should not conflict")
}

func TestApproversConflict_ReviewerRosterIgnored(t *testing.T) {
	// Regression test: two MRs sharing the identical "Approvers" rule must not
	// be flagged as conflicting just because they currently have a different
	// subset of that rule's members added as reviewers.
	a := MergeRequest{
		Approvers: []string{testUsername, testOther},
		Reviewers: []ReviewerInfo{{Username: testUsername, IsApprover: true}},
	}
	b := MergeRequest{
		Approvers: []string{testUsername, testOther},
		Reviewers: []ReviewerInfo{
			{Username: testUsername, IsApprover: true},
			{Username: testOther, IsApprover: true},
		},
	}
	assert.False(t, ApproversConflict(a, b), "a differing reviewer roster must not affect the approver-rule comparison")
}
