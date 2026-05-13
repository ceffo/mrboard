// Package statestore provides a YAML-backed implementation of tui.StateStore.
package statestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ceffo/mrboard/internal/tui"
)

const (
	dirMode  = 0o700
	fileMode = 0o600
)

// Config holds configuration for the YAML-backed state store.
type Config struct {
	Dir string // XDG data dir: ~/.local/share/mrboard/
}

// YAMLStore persists tui.State to {Dir}/state.yaml.
type YAMLStore struct {
	path string
}

// New creates a YAMLStore, ensuring the data directory exists (mode 0700).
func New(cfg Config) (*YAMLStore, error) {
	if err := os.MkdirAll(cfg.Dir, dirMode); err != nil {
		return nil, fmt.Errorf("statestore: create dir %q: %w", cfg.Dir, err)
	}
	return &YAMLStore{path: filepath.Join(cfg.Dir, "state.yaml")}, nil
}

// Load reads persisted state. Returns tui.DefaultState() if the file is absent.
func (s *YAMLStore) Load() (tui.State, error) {
	data, err := os.ReadFile(filepath.Clean(s.path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tui.DefaultState(), nil
		}
		return tui.DefaultState(), fmt.Errorf("statestore: read %q: %w", s.path, err)
	}
	var st tui.State
	if err := yaml.Unmarshal(data, &st); err != nil {
		return tui.DefaultState(), fmt.Errorf("statestore: parse %q: %w", s.path, err)
	}
	return st, nil
}

// Save writes state to disk with mode 0600.
func (s *YAMLStore) Save(st tui.State) error {
	data, err := yaml.Marshal(st)
	if err != nil {
		return fmt.Errorf("statestore: marshal: %w", err)
	}
	if err := os.WriteFile(filepath.Clean(s.path), data, fileMode); err != nil {
		return fmt.Errorf("statestore: write %q: %w", s.path, err)
	}
	return nil
}
