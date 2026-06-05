package mrsvc

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
	// Phases restricts visible MRs to those whose Phase is true in the map.
	// nil or empty map means all phases are shown.
	Phases map[domain.MRPhase]bool
	// Authors restricts visible MRs to those whose author is in the set.
	// nil or empty slice means all authors are shown.
	Authors []string
	// Reviewers restricts visible MRs to those that include any of the given reviewer usernames.
	// nil or empty slice means all reviewers are shown.
	Reviewers []string
}

// FilterAndSort applies all active filters and then sorts the slice.
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
	if len(opts.Phases) > 0 {
		filtered := make([]domain.MergeRequest, 0, len(mrs))
		for _, mr := range mrs {
			if opts.Phases[mr.Phase] {
				filtered = append(filtered, mr)
			}
		}
		mrs = filtered
	}
	if len(opts.Authors) > 0 {
		authorSet := make(map[string]bool, len(opts.Authors))
		for _, a := range opts.Authors {
			authorSet[a] = true
		}
		// Current user's MRs always pass the author filter regardless of selection.
		if opts.CurrentUser != "" {
			authorSet[opts.CurrentUser] = true
		}
		filtered := make([]domain.MergeRequest, 0, len(mrs))
		for _, mr := range mrs {
			if authorSet[mr.Author] {
				filtered = append(filtered, mr)
			}
		}
		mrs = filtered
	}
	if len(opts.Reviewers) > 0 {
		reviewerSet := make(map[string]bool, len(opts.Reviewers))
		for _, r := range opts.Reviewers {
			reviewerSet[r] = true
		}
		filtered := make([]domain.MergeRequest, 0, len(mrs))
		for _, mr := range mrs {
			for _, r := range mr.Reviewers {
				if reviewerSet[r.Username] {
					filtered = append(filtered, mr)
					break
				}
			}
		}
		mrs = filtered
	}
	return sortedMRs(mrs, opts.SortField, opts.SortDesc)
}

// mrIsRelevantToUser reports whether an MR should appear in "my view".
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

// BuildUserMap creates a username → full-name lookup table from all author and reviewer info in the MR list.
func BuildUserMap(mrs []domain.MergeRequest) map[string]string {
	m := make(map[string]string)
	for _, mr := range mrs {
		if mr.Author != "" && mr.AuthorName != "" {
			m[mr.Author] = mr.AuthorName
		}
		for _, r := range mr.Reviewers {
			if r.Username != "" && r.Name != "" {
				m[r.Username] = r.Name
			}
		}
	}
	return m
}

// DisplayName returns the full name for username from userMap, or username if not found.
func DisplayName(userMap map[string]string, username string) string {
	if n, ok := userMap[username]; ok {
		return n
	}
	return username
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
			less, equal = b.CreatedAt.Before(a.CreatedAt), a.CreatedAt.Equal(b.CreatedAt)
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
