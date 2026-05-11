package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	stateDirMode  = 0o700
	stateFileMode = 0o600
)

// State holds UI preferences persisted across sessions.
type State struct {
	SortField string `yaml:"sort_field"` // "repo_iid" | "author" | "age"
	SortDesc  bool   `yaml:"sort_desc"`
	MyView    bool   `yaml:"my_view"`
}

// DefaultState returns the out-of-box UI state.
func DefaultState() State {
	return State{SortField: "repo_iid"}
}

// LoadState reads persisted state from the XDG config dir.
// Returns defaults silently on any error (file absent, parse failure, etc.).
func LoadState() State {
	dir := XDGConfigDir()
	if dir == "" {
		return DefaultState()
	}
	f, err := os.Open(filepath.Clean(filepath.Join(dir, "state.yaml")))
	if err != nil {
		return DefaultState()
	}
	defer f.Close()
	var s State
	if err := yaml.NewDecoder(f).Decode(&s); err != nil {
		return DefaultState()
	}
	return s
}

// SaveState persists UI state to the XDG config dir, creating the dir if needed.
// Errors are silently swallowed — state persistence is best-effort.
func SaveState(s State) {
	dir := XDGConfigDir()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, stateDirMode); err != nil {
		return
	}
	path := filepath.Join(dir, "state.yaml")
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, stateFileMode)
	if err != nil {
		return
	}
	defer f.Close()
	enc := yaml.NewEncoder(f)
	if err := enc.Encode(s); err != nil {
		return
	}
	_ = enc.Close()
}
