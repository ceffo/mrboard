package tui

// ViewMode controls whether the board shows all MRs or only the current user's.
type ViewMode int

// ViewMode values.
const (
	ViewAll  ViewMode = iota
	ViewMine          // filters to current_user's MRs
)

// State holds UI preferences persisted across sessions.
type State struct {
	SortField string   `yaml:"sort_field"` // "repo_iid" | "author" | "age"
	SortDesc  bool     `yaml:"sort_desc"`
	ViewMode  ViewMode `yaml:"view_mode"`
}

// DefaultState returns the out-of-box UI state.
func DefaultState() State { return State{SortField: "repo_iid", ViewMode: ViewAll} }

// StateStore is the driven port for persisting UI state across sessions.
type StateStore interface {
	Load() (State, error)
	Save(State) error
}
