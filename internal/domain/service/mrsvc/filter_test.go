package mrsvc_test

import (
	"testing"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// helpers

const (
	userAlice  = "alice"
	userBob    = "bob"
	userCarol  = "carol"
	sortAuthor = "author"
	sortAge    = "age"
	sortRepoID = "repo_iid"
)

func mr(id int, author, repo string, iid int, created time.Time, reviewers ...domain.ReviewerInfo) domain.MergeRequest {
	return domain.MergeRequest{
		ID:          id,
		IID:         iid,
		Author:      author,
		ProjectPath: repo,
		CreatedAt:   created,
		Reviewers:   reviewers,
	}
}

// reviewer builds a ReviewerInfo for alice (the fixed test user) in the given state.
func reviewer(state domain.ReviewerState) domain.ReviewerInfo {
	return domain.ReviewerInfo{Username: userAlice, State: state}
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
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestFilterAndSort_MyViewOn_FiltersByAuthor(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 1, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	if len(got) != 1 || got[0].Author != userAlice {
		t.Fatalf("expected alice's MR only, got %v", got)
	}
}

func TestFilterAndSort_MyViewOn_IncludesNotStartedReviewer(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "bob", "repo/a", 1, t0, reviewer(domain.ReviewerNotStarted)),
		mr(2, userBob, "repo/b", 2, t0, reviewer(domain.ReviewerApproved)),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	if len(got) != 1 || got[0].IID != 1 {
		t.Fatalf("expected only the not-started reviewer MR, got %v", got)
	}
}

func TestFilterAndSort_MyViewOn_IncludesReReviewRequested(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "bob", "repo/a", 1, t0, reviewer(domain.ReviewerReReviewRequested)),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
}

func TestFilterAndSort_MyViewOn_ExcludesCommentedReviewer(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "bob", "repo/a", 1, t0, reviewer(domain.ReviewerCommented)),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: userAlice})
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestFilterAndSort_MyViewOn_EmptyUserReturnsAll(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 1, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{MyView: true, CurrentUser: ""})
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
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
	for i, mr := range got {
		if mr.IID != want[i] {
			t.Fatalf("pos %d: want IID %d, got %d", i, want[i], mr.IID)
		}
	}
}

func TestFilterAndSort_SortRepoIID_Descending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(3, userCarol, "repo/a", 2, t0),
		mr(1, userAlice, "repo/a", 10, t0),
		mr(2, userBob, "repo/b", 5, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortRepoID, SortDesc: true})
	want := []int{5, 10, 2}
	for i, mr := range got {
		if mr.IID != want[i] {
			t.Fatalf("pos %d: want IID %d, got %d", i, want[i], mr.IID)
		}
	}
}

// FilterAndSort — sort by author

func TestFilterAndSort_SortAuthor_Ascending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "carol", "repo/a", 1, t0),
		mr(2, "alice", "repo/b", 2, t0),
		mr(3, "bob", "repo/c", 3, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortAuthor})
	wantAuthors := []string{"alice", "bob", "carol"}
	for i, mr := range got {
		if mr.Author != wantAuthors[i] {
			t.Fatalf("pos %d: want %s, got %s", i, wantAuthors[i], mr.Author)
		}
	}
}

func TestFilterAndSort_SortAuthor_Descending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, "carol", "repo/b", 2, t0),
		mr(3, "bob", "repo/c", 3, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortAuthor, SortDesc: true})
	wantAuthors := []string{"carol", "bob", "alice"}
	for i, mr := range got {
		if mr.Author != wantAuthors[i] {
			t.Fatalf("pos %d: want %s, got %s", i, wantAuthors[i], mr.Author)
		}
	}
}

// FilterAndSort — sort by age

func TestFilterAndSort_SortAge_Ascending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t2),
		mr(2, userBob, "repo/b", 2, t0),
		mr(3, userCarol, "repo/c", 3, t1),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortAge})
	wantIDs := []int{2, 3, 1} // t0, t1, t2
	for i, mr := range got {
		if mr.ID != wantIDs[i] {
			t.Fatalf("pos %d: want ID %d, got %d", i, wantIDs[i], mr.ID)
		}
	}
}

func TestFilterAndSort_SortAge_Descending(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t2),
		mr(2, userBob, "repo/b", 2, t0),
		mr(3, userCarol, "repo/c", 3, t1),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortAge, SortDesc: true})
	wantIDs := []int{1, 3, 2} // t2, t1, t0
	for i, mr := range got {
		if mr.ID != wantIDs[i] {
			t.Fatalf("pos %d: want ID %d, got %d", i, wantIDs[i], mr.ID)
		}
	}
}

// FilterAndSort — multi-select Authors

func TestFilterAndSort_Authors_SingleMatch(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 2, t0),
		mr(3, userCarol, "repo/c", 3, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Authors: []string{userAlice}})
	if len(got) != 1 || got[0].Author != userAlice {
		t.Fatalf("expected only alice, got %v", got)
	}
}

func TestFilterAndSort_Authors_MultiMatch(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 2, t0),
		mr(3, userCarol, "repo/c", 3, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Authors: []string{userAlice, userBob}})
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestFilterAndSort_Authors_EmptyShowsAll(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userAlice, "repo/a", 1, t0),
		mr(2, userBob, "repo/b", 2, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Authors: nil})
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

// FilterAndSort — multi-select Reviewers

func TestFilterAndSort_Reviewers_SingleMatch(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userBob, "repo/a", 1, t0, domain.ReviewerInfo{Username: userAlice, State: domain.ReviewerNotStarted}),
		mr(2, userCarol, "repo/b", 2, t0, domain.ReviewerInfo{Username: userBob, State: domain.ReviewerNotStarted}),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Reviewers: []string{userAlice}})
	if len(got) != 1 || got[0].IID != 1 {
		t.Fatalf("expected only MR with alice as reviewer, got %v", got)
	}
}

func TestFilterAndSort_Reviewers_MultiMatch(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userBob, "repo/a", 1, t0, domain.ReviewerInfo{Username: userAlice, State: domain.ReviewerNotStarted}),
		mr(2, userCarol, "repo/b", 2, t0, domain.ReviewerInfo{Username: userBob, State: domain.ReviewerNotStarted}),
		mr(3, userAlice, "repo/c", 3, t0, domain.ReviewerInfo{Username: userCarol, State: domain.ReviewerNotStarted}),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Reviewers: []string{userAlice, userBob}})
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestFilterAndSort_Reviewers_EmptyShowsAll(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, userBob, "repo/a", 1, t0, domain.ReviewerInfo{Username: userAlice, State: domain.ReviewerNotStarted}),
		mr(2, userCarol, "repo/b", 2, t0),
	}
	got := mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{Reviewers: nil})
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

// FilterAndSort — does not mutate input

func TestFilterAndSort_DoesNotMutateInput(t *testing.T) {
	mrs := []domain.MergeRequest{
		mr(1, "carol", "repo/a", 1, t0),
		mr(2, "alice", "repo/b", 2, t0),
	}
	original := make([]domain.MergeRequest, len(mrs))
	copy(original, mrs)
	mrsvc.FilterAndSort(mrs, mrsvc.FilterOptions{SortField: sortAuthor})
	for i := range mrs {
		if mrs[i].ID != original[i].ID {
			t.Fatal("input slice was mutated")
		}
	}
}
