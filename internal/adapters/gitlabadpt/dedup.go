package gitlabadpt

import "github.com/ceffo/mrboard/internal/domain"

// mrKey uniquely identifies an MR across all sources.
type mrKey struct {
	projectID int
	iid       int
}

// MRDeduplicator deduplicates and excludes domain.MergeRequest slices by project+IID key.
type MRDeduplicator struct {
	ExcludedAuthors []string
}

// Deduplicate returns mrs with excluded authors removed and project+IID duplicates collapsed.
// Order of first occurrence is preserved. Pure — no side effects.
func (d MRDeduplicator) Deduplicate(mrs []domain.MergeRequest) []domain.MergeRequest {
	excluded := make(map[string]bool, len(d.ExcludedAuthors))
	for _, u := range d.ExcludedAuthors {
		excluded[u] = true
	}
	seen := make(map[mrKey]bool, len(mrs))
	out := make([]domain.MergeRequest, 0, len(mrs))
	for _, mr := range mrs {
		if excluded[mr.Author] {
			continue
		}
		k := mrKey{projectID: mr.ProjectID, iid: mr.IID}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, mr)
	}
	return out
}
