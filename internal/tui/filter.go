package tui

import "github.com/ceffo/mrboard/internal/domain"

// FilterCriteria is the single source of truth for all active filter state.
// An empty/zero value means no filtering (show everything).
type FilterCriteria struct {
	// Phases is nil/empty = show all phases; otherwise only listed phases are shown.
	Phases map[domain.MRPhase]bool
	// Authors is nil/empty = show all authors.
	Authors []string
	// Reviewers is nil/empty = show all reviewers.
	Reviewers []string
}
