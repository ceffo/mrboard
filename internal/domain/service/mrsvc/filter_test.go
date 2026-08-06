package mrsvc_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// helpers

const (
	userAlice    = "alice"
	userBob      = "bob"
	userCarol    = "carol"
	sortAssignee = "assignee"
	sortAge      = "age"
	sortRepoID   = "repo_iid"
)

func mr(
	id int, assignee, repo string, iid int, created time.Time, reviewers ...domain.ReviewerInfo,
) domain.MergeRequest {
	return domain.MergeRequest{
		ID:          id,
		IID:         iid,
		Author:      assignee, // kept equal to Assignee for test simplicity
		Assignee:    assignee,
		ProjectPath: repo,
		CreatedAt:   created,
		Reviewers:   reviewers,
	}
}

// reviewer builds a ReviewerInfo for alice (the fixed test user) in the given state.
func reviewer(state domain.ReviewerState) domain.ReviewerInfo {
	return domain.ReviewerInfo{Username: userAlice, State: state}
}

func ids(mrs []domain.MergeRequest) []int {
	out := make([]int, len(mrs))
	for i, mr := range mrs {
		out[i] = mr.ID
	}
	return out
}

func iids(mrs []domain.MergeRequest) []int {
	out := make([]int, len(mrs))
	for i, mr := range mrs {
		out[i] = mr.IID
	}
	return out
}

func assignees(mrs []domain.MergeRequest) []string {
	out := make([]string, len(mrs))
	for i, mr := range mrs {
		out[i] = mr.Assignee
	}
	return out
}

var (
	t0 = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 = t0.Add(24 * time.Hour)
	t2 = t0.Add(48 * time.Hour)
)

// FilterAndSort — my-view filtering

func TestFilterAndSort_MyViewOff_ReturnsAll(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 1, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: false, CurrentUser: userAlice})
	assert.Len(t, got, 2)
}

func TestFilterAndSort_MyViewOn_FiltersByAssignee(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 1, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	require.Len(t, got, 1)
	assert.Equal(t, userAlice, got[0].Assignee)
}

func TestFilterAndSort_MyViewOn_IncludesNotStartedReviewer(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "bob", "repo/a", 1, t0, reviewer(domain.ReviewerNotStarted)),
		mr(2, userBob, "repo/b", 2, t0, reviewer(domain.ReviewerApproved)),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].IID)
}

func TestFilterAndSort_MyViewOn_IncludesReReviewRequested(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "bob", "repo/a", 1, t0, reviewer(domain.ReviewerReReviewRequested)),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	assert.Len(t, got, 1)
}

func TestFilterAndSort_MyViewOn_ExcludesCommentedReviewer(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "bob", "repo/a", 1, t0, reviewer(domain.ReviewerCommented)),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	assert.Empty(t, got)
}

func TestFilterAndSort_MyViewOn_ApproversAssigned_ExcludesNonApproverReviewer(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "bob", "repo/a", 1, t0,
			domain.ReviewerInfo{Username: userAlice, State: domain.ReviewerNotStarted, IsApprover: false},
			domain.ReviewerInfo{Username: userBob, State: domain.ReviewerNotStarted, IsApprover: true},
		),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	assert.Empty(t, got, "alice is a reviewer but not an approver")
}

func TestFilterAndSort_MyViewOn_ApproversAssigned_IncludesApproverReviewer(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "bob", "repo/a", 1, t0,
			domain.ReviewerInfo{Username: userAlice, State: domain.ReviewerNotStarted, IsApprover: true},
			domain.ReviewerInfo{Username: userBob, State: domain.ReviewerNotStarted, IsApprover: true},
		),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	assert.Len(t, got, 1, "alice is an approver")
}

func TestFilterAndSort_MyViewOn_ApproversAssigned_AssigneeStillIncluded(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0,
			domain.ReviewerInfo{Username: userBob, State: domain.ReviewerNotStarted, IsApprover: true},
		),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	assert.Len(t, got, 1, "alice is the assignee")
}

func TestFilterAndSort_MyViewOn_ApproversAssigned_AuthorOnlyExcluded(t *testing.T) {
	mrs := []domain.MergeRequest{
		{
			ID: 1, IID: 1, ProjectPath: "repo/a", Author: userAlice, Assignee: "",
			Reviewers: []domain.ReviewerInfo{{Username: userBob, State: domain.ReviewerNotStarted, IsApprover: true}},
		},
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	assert.Empty(t, got, "alice is only the author, not assignee or approver")
}

func TestFilterAndSort_MyViewOn_EmptyUserReturnsAll(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 1, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: ""})
	assert.Len(t, got, 2)
}

// FilterAndSort — sort by repo_iid

func TestFilterAndSort_SortRepoIID_Ascending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(2, userBob, "repo/b", 5, t0),
		mr(1, userAlice, "repo/a", 10, t0),
		mr(3, userCarol, "repo/a", 2, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortRepoID})
	want := []int{2, 10, 5} // repo/a IID 2, repo/a IID 10, repo/b IID 5
	assert.Equal(t, want, iids(got))
}

func TestFilterAndSort_SortRepoIID_Descending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(3, userCarol, "repo/a", 2, t0),
		mr(1, userAlice, "repo/a", 10, t0),
		mr(2, userBob, "repo/b", 5, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortRepoID, SortDesc: true})
	want := []int{5, 10, 2}
	assert.Equal(t, want, iids(got))
}

// FilterAndSort — sort by assignee

func TestFilterAndSort_SortAssignee_Ascending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "carol", "repo/a", 1, t0),
		mr(2, "alice", "repo/b", 2, t0),
		mr(3, "bob", "repo/c", 3, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortAssignee})
	wantAssignees := []string{"alice", "bob", "carol"}
	assert.Equal(t, wantAssignees, assignees(got))
}

func TestFilterAndSort_SortAssignee_Descending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, "carol", "repo/b", 2, t0),
		mr(3, "bob", "repo/c", 3, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortAssignee, SortDesc: true})
	wantAssignees := []string{"carol", "bob", "alice"}
	assert.Equal(t, wantAssignees, assignees(got))
}

// FilterAndSort — sort by age

func TestFilterAndSort_SortAge_Ascending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t2),
		mr(2, userBob, "repo/b", 2, t0),
		mr(3, userCarol, "repo/c", 3, t1),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortAge})
	wantIDs := []int{1, 3, 2} // t2, t1, t0 — youngest (smallest age) first
	assert.Equal(t, wantIDs, ids(got))
}

func TestFilterAndSort_SortAge_Descending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t2),
		mr(2, userBob, "repo/b", 2, t0),
		mr(3, userCarol, "repo/c", 3, t1),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortAge, SortDesc: true})
	wantIDs := []int{2, 3, 1} // t0, t1, t2 — oldest (largest age) first
	assert.Equal(t, wantIDs, ids(got))
}

// FilterAndSort — multi-select Assignees

func TestFilterAndSort_Assignees_SingleMatch(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 2, t0),
		mr(3, userCarol, "repo/c", 3, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Assignees: []string{userAlice}})
	require.Len(t, got, 1)
	assert.Equal(t, userAlice, got[0].Assignee)
}

func TestFilterAndSort_Assignees_MultiMatch(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 2, t0),
		mr(3, userCarol, "repo/c", 3, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Assignees: []string{userAlice, userBob}})
	assert.Len(t, got, 2)
}

func TestFilterAndSort_Assignees_EmptyShowsAll(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 2, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Assignees: nil})
	assert.Len(t, got, 2)
}

// FilterAndSort — multi-select Reviewers

func TestFilterAndSort_Reviewers_SingleMatch(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userBob, "repo/a", 1, t0, domain.ReviewerInfo{Username: userAlice, State: domain.ReviewerNotStarted}),
		mr(2, userCarol, "repo/b", 2, t0, domain.ReviewerInfo{Username: userBob, State: domain.ReviewerNotStarted}),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Reviewers: []string{userAlice}})
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].IID)
}

func TestFilterAndSort_Reviewers_MultiMatch(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userBob, "repo/a", 1, t0, domain.ReviewerInfo{Username: userAlice, State: domain.ReviewerNotStarted}),
		mr(2, userCarol, "repo/b", 2, t0, domain.ReviewerInfo{Username: userBob, State: domain.ReviewerNotStarted}),
		mr(3, userAlice, "repo/c", 3, t0, domain.ReviewerInfo{Username: userCarol, State: domain.ReviewerNotStarted}),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Reviewers: []string{userAlice, userBob}})
	assert.Len(t, got, 2)
}

func TestFilterAndSort_Reviewers_EmptyShowsAll(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userBob, "repo/a", 1, t0, domain.ReviewerInfo{Username: userAlice, State: domain.ReviewerNotStarted}),
		mr(2, userCarol, "repo/b", 2, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Reviewers: nil})
	assert.Len(t, got, 2)
}

// FilterAndSort — sprint filter

func TestFilterAndSort_SprintFilter_IncludesOnlySprintMRs(t *testing.T) {
	mrs := []domain.MergeRequest{
		{ID: 1, IID: 1, Title: "feat(OD-100): in sprint"},
		{ID: 2, IID: 2, Title: "feat(OD-200): also in sprint"},
		{ID: 3, IID: 3, Title: "fix: no jira id"},
		{ID: 4, IID: 4, Title: "feat(OD-999): not in sprint"},
	}
	sprintKeys := map[string]bool{"OD-100": true, "OD-200": true}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SprintFilter: true, SprintKeys: sprintKeys})
	assert.Len(t, got, 2)
}

func TestFilterAndSort_SprintFilter_CaseInsensitiveMatchesRegardlessOfTitleCase(t *testing.T) {
	mrs := []domain.MergeRequest{{ID: 1, IID: 1, Title: "fix(od-500): lowercase title key"}}
	sprintKeys := map[string]bool{"OD-500": true}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{
		SprintFilter: true, SprintKeys: sprintKeys, KeyMatcher: domain.NewTicketKeyMatcher(true),
	})
	assert.Len(t, got, 1, "lowercase title key should match the uppercase sprint key")
}

func TestFilterAndSort_SprintFilter_CaseSensitiveRequiresExactCase(t *testing.T) {
	mrs := []domain.MergeRequest{{ID: 1, IID: 1, Title: "fix(od-500): lowercase title key"}}
	sprintKeys := map[string]bool{"OD-500": true}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{
		SprintFilter: true, SprintKeys: sprintKeys, KeyMatcher: domain.NewTicketKeyMatcher(false),
	})
	assert.Empty(t, got, "a differently-cased title key must not match when case-sensitive")
}

func TestFilterAndSort_SprintFilter_OffShowsAll(t *testing.T) {
	mrs := []domain.MergeRequest{
		{ID: 1, IID: 1, Title: "feat(OD-100): in sprint"},
		{ID: 2, IID: 2, Title: "feat(OD-999): not in sprint"},
	}
	sprintKeys := map[string]bool{"OD-100": true}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SprintFilter: false, SprintKeys: sprintKeys})
	assert.Len(t, got, 2, "SprintFilter is off")
}

func TestFilterAndSort_SprintFilter_NilKeysShowsAll(t *testing.T) {
	mrs := []domain.MergeRequest{
		{ID: 1, IID: 1, Title: "feat(OD-100): some mr"},
		{ID: 2, IID: 2, Title: "fix: no jira id"},
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SprintFilter: true, SprintKeys: nil})
	assert.Len(t, got, 2, "SprintKeys is nil")
}

// FilterAndSort — does not mutate input

func TestFilterAndSort_DoesNotMutateInput(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "carol", "repo/a", 1, t0),
		mr(2, "alice", "repo/b", 2, t0),
	}
	original := make([]domain.MergeRequest, len(mrs))
	copy(original, mrs)
	mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortAssignee})
	assert.Equal(t, original, mrs, "input slice was mutated")
}
