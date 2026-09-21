package domain

import "time"

// FilterCriteria is the persisted filter state. Zero value means no filtering.
//
// Assignee/Reviewer/Issue-ID selection is exclusion-based: every value is
// shown by default, and a list names what to hide. This is the inverse of
// an older inclusion-based format (an "assignees"/"reviewers"/"ticket_keys"
// list once meant "show only these") — a state.yaml written by that version
// uses the same YAML shape under different keys, so it simply fails to
// populate these fields and any previously-active filter resets to "show
// all" rather than being silently reinterpreted as its own opposite.
type FilterCriteria struct {
	// Phases is nil/empty = show all phases; otherwise only listed phases are shown.
	Phases map[MRPhase]bool `yaml:"phases,omitempty"`
	// ExcludedAssignees is nil/empty = show every assignee; otherwise MRs
	// assigned to a listed username are hidden.
	ExcludedAssignees []string `yaml:"excluded_assignees,omitempty"`
	// ExcludedReviewers is nil/empty = show every reviewer; otherwise MRs
	// that have a listed username among their reviewers are hidden.
	ExcludedReviewers []string `yaml:"excluded_reviewers,omitempty"`
	// ExcludedTicketKeys is nil/empty = show every issue ID; otherwise MRs
	// whose extracted issue ID is listed are hidden.
	ExcludedTicketKeys []string `yaml:"excluded_ticket_keys,omitempty"`
	// ExcludeTicketless, when true, hides MRs with no detectable issue ID.
	ExcludeTicketless bool `yaml:"exclude_ticketless,omitempty"`
}

// ViewMode controls whether the board shows all MRs or only the current user's.
type ViewMode int

// ViewMode values.
const (
	ViewAll  ViewMode = iota
	ViewMine          // filters to current_user's MRs
)

// AppState is the persisted subset of UI state — fields that survive across sessions.
type AppState struct {
	SortField          string         `yaml:"sort_field"` // "repo_iid" | "author" | "age"
	SortDesc           bool           `yaml:"sort_desc"`
	ViewMode           ViewMode       `yaml:"view_mode"`
	ThemeName          string         `yaml:"theme_name"` // "" means "default"
	ThemeMode          string         `yaml:"theme_mode"` // "" means "auto"
	Filter             FilterCriteria `yaml:"filter,omitempty"`
	IncludeReviewerMRs bool           `yaml:"include_reviewer_mrs,omitempty"`
}

// DefaultAppState returns the out-of-box persisted state.
func DefaultAppState() AppState {
	return AppState{SortField: "repo_iid", ViewMode: ViewAll, ThemeMode: "auto"}
}

// StateStore is the driven port for persisting app state across sessions.
type StateStore interface {
	Load() (AppState, error)
	Save(AppState) error
}

// SnapshotStore is the driven port for persisting the last-known set of MRs,
// used to support incremental fetch (see docs/adr/0005). Load returning an
// empty slice and a zero time.Time means there is nothing usable to diff
// against — a cold cache, not an error — including when the adapter discards
// a snapshot written by an older, incompatible schema version. The returned
// time is when the snapshot was written, for the header's age indicator.
type SnapshotStore interface {
	Load() ([]MergeRequest, time.Time, error)
	Save([]MergeRequest) error
}
