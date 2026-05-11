package service

import (
	"sort"

	"github.com/ceffo/mrboard/internal/domain"
)

// FilterOptions controls which MRs are shown and in what order.
// The zero value shows all MRs sorted by repo+IID ascending.
type FilterOptions struct {
	MyView      bool
	CurrentUser string
	SortField   string // "repo_iid" | "author" | "age"
	SortDesc    bool
}

// FilterAndSort applies the my-view filter and then sorts the slice.
// It always returns a new slice; mrs is never mutated.
func FilterAndSort(mrs []domain.MergeRequest, opts FilterOptions) []domain.MergeRequest {
	if opts.MyView && opts.CurrentUser != "" {
		filtered := make([]domain.MergeRequest, 0, len(mrs))
		for _, mr := range mrs {
			if mrIsRelevantToUser(mr, opts.CurrentUser) {
				filtered = append(filtered, mr)
			}
		}
		mrs = filtered
	}
	return sortedMRs(mrs, opts.SortField, opts.SortDesc)
}

// mrIsRelevantToUser reports whether an MR should appear in "my view".
// An MR is relevant if the user authored it, or is a reviewer whose turn it is.
func mrIsRelevantToUser(mr domain.MergeRequest, username string) bool {
	if mr.Author == username {
		return true
	}
	for _, r := range mr.Reviewers {
		if r.Username == username &&
			(r.State == domain.ReviewerNotStarted || r.State == domain.ReviewerReReviewRequested) {
			return true
		}
	}
	return false
}

func sortedMRs(mrs []domain.MergeRequest, field string, desc bool) []domain.MergeRequest {
	out := make([]domain.MergeRequest, len(mrs))
	copy(out, mrs)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		var less, equal bool
		switch field {
		case "author":
			less, equal = a.Author < b.Author, a.Author == b.Author
		case "age":
			less, equal = a.CreatedAt.Before(b.CreatedAt), a.CreatedAt.Equal(b.CreatedAt)
		default: // "repo_iid"
			if a.ProjectPath != b.ProjectPath {
				less = a.ProjectPath < b.ProjectPath
			} else {
				less, equal = a.IID < b.IID, a.IID == b.IID
			}
		}
		if !equal {
			if desc {
				return !less
			}
			return less
		}
		// Tiebreaker: repo asc, IID asc.
		if a.ProjectPath != b.ProjectPath {
			return a.ProjectPath < b.ProjectPath
		}
		return a.IID < b.IID
	})
	return out
}
