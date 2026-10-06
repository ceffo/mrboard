package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	attnMe    = "me"
	attnOther = "other"
)

func TestRolesOf(t *testing.T) {
	tests := []struct {
		name string
		mr   MergeRequest
		want []Role
	}{
		{"unrelated", MergeRequest{Author: attnOther}, nil},
		{"author", MergeRequest{Author: attnMe}, []Role{RoleAuthor}},
		{"assignee", MergeRequest{Author: attnOther, Assignee: attnMe}, []Role{RoleAssignee}},
		{
			"author and assignee",
			MergeRequest{Author: attnMe, Assignee: attnMe},
			[]Role{RoleAuthor, RoleAssignee},
		},
		{
			"reviewer in any state",
			MergeRequest{Author: attnOther, Reviewers: []ReviewerInfo{{Username: attnMe, State: ReviewerApproved}}},
			[]Role{RoleReviewer},
		},
		{
			"approver is a reviewer",
			MergeRequest{Author: attnOther, Reviewers: []ReviewerInfo{{Username: attnMe, IsApprover: true}}},
			[]Role{RoleReviewer},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.mr.RolesOf(attnMe))
		})
	}
}

func TestRolesOf_EmptyUsernameHasNoRoles(t *testing.T) {
	assert.Empty(t, MergeRequest{Author: "", Assignee: ""}.RolesOf(""))
}

func TestConcerns(t *testing.T) {
	assert.True(t, MergeRequest{Author: attnMe}.Concerns(attnMe))
	assert.False(t, MergeRequest{Author: attnOther}.Concerns(attnMe))
}

func TestNeedsAttention_OwnerRole(t *testing.T) {
	tests := []struct {
		name  string
		phase MRPhase
		want  bool
	}{
		{"draft", PhaseDraft, false},
		{"waiting on reviewers", PhaseNeedsReview, false},
		{"reviewers commented", PhaseNeedsAuthorAction, true},
		{"ready to merge", PhaseReadyToMerge, true},
	}
	for _, tt := range tests {
		t.Run("author/"+tt.name, func(t *testing.T) {
			mr := MergeRequest{Author: attnMe, Phase: tt.phase}
			assert.Equal(t, tt.want, mr.NeedsAttention(attnMe))
		})
		t.Run("assignee/"+tt.name, func(t *testing.T) {
			mr := MergeRequest{Author: attnOther, Assignee: attnMe, Phase: tt.phase}
			assert.Equal(t, tt.want, mr.NeedsAttention(attnMe))
		})
	}
}

func TestNeedsAttention_ReviewerState(t *testing.T) {
	tests := []struct {
		state ReviewerState
		want  bool
	}{
		{ReviewerNotStarted, true},
		{ReviewerReReviewRequested, true},
		{ReviewerCommented, false},
		{ReviewerApproved, false},
	}
	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			mr := MergeRequest{
				Author: attnOther, Phase: PhaseNeedsReview,
				Reviewers: []ReviewerInfo{{Username: attnMe, State: tt.state}},
			}
			assert.Equal(t, tt.want, mr.NeedsAttention(attnMe))
		})
	}
}

func TestNeedsAttention_ReviewerOnDraftIsNotPrompted(t *testing.T) {
	mr := MergeRequest{
		Author: attnOther, Phase: PhaseDraft,
		Reviewers: []ReviewerInfo{{Username: attnMe, State: ReviewerNotStarted}},
	}
	assert.False(t, mr.NeedsAttention(attnMe))
}

func TestNeedsAttention_NonApproverIsNotPromptedWhenApproversExist(t *testing.T) {
	mr := MergeRequest{
		Author: attnOther, Phase: PhaseNeedsReview,
		Reviewers: []ReviewerInfo{
			{Username: attnMe, State: ReviewerNotStarted},
			{Username: "approver", State: ReviewerNotStarted, IsApprover: true},
		},
	}
	assert.False(t, mr.NeedsAttention(attnMe), "non-approver reviewer is not prompted")
	assert.True(t, mr.Concerns(attnMe), "but the MR still concerns them")
}

func TestNeedsAttention_ApproverIsPrompted(t *testing.T) {
	mr := MergeRequest{
		Author: attnOther, Phase: PhaseNeedsReview,
		Reviewers: []ReviewerInfo{{Username: attnMe, State: ReviewerNotStarted, IsApprover: true}},
	}
	assert.True(t, mr.NeedsAttention(attnMe))
}

func TestNeedsAttention_EmptyUsername(t *testing.T) {
	mr := MergeRequest{Phase: PhaseReadyToMerge}
	assert.False(t, mr.NeedsAttention(""))
}

func TestNeedsAttention_AuthorOfWaitingMRWithApproversIsNotPrompted(t *testing.T) {
	mr := MergeRequest{
		Author: attnMe, Phase: PhaseNeedsReview,
		Reviewers: []ReviewerInfo{{Username: "approver", State: ReviewerNotStarted, IsApprover: true}},
	}
	assert.False(t, mr.NeedsAttention(attnMe))
	assert.True(t, mr.Concerns(attnMe), "the user's own MR stays visible")
}
