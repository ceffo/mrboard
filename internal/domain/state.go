package domain

// FilterCriteria is the persisted filter state. Zero value means no filtering.
type FilterCriteria struct {
	// Phases is nil/empty = show all phases; otherwise only listed phases are shown.
	Phases map[MRPhase]bool `yaml:"phases,omitempty"`
	// Authors is nil/empty = show all authors.
	Authors []string `yaml:"authors,omitempty"`
	// Reviewers is nil/empty = show all reviewers.
	Reviewers []string `yaml:"reviewers,omitempty"`
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
