// Package statestore provides a YAML-backed implementation of domain.StateStore.
package statestore

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ceffo/mrboard/internal/domain"
)

const (
	dirMode  = 0o700
	fileMode = 0o600
)

// Config holds configuration for the YAML-backed state store.
type Config struct {
	Dir string // XDG data dir: ~/.local/share/mrboard/
}

// YAMLStore persists domain.AppState to {Dir}/state.yaml.
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

// legacyFilterCriteria captures the pre-exclusion-model MR filter keys, kept
// only so Load can warn when it finds one: state.yaml written before the
// Filters tab switched from an inclusion list ("show only these") to an
// exclusion list (see domain.FilterCriteria) uses these same keys, which no
// longer unmarshal into anything — the filter silently resets to "show all"
// rather than being reinterpreted as its own opposite.
type legacyFilterCriteria struct {
	Assignees  []string `yaml:"assignees"`
	Reviewers  []string `yaml:"reviewers"`
	TicketKeys []string `yaml:"ticket_keys"`
}

type legacyAppState struct {
	Filter legacyFilterCriteria `yaml:"filter"`
}

// Load reads persisted state. Returns domain.DefaultAppState() if the file is absent.
func (s *YAMLStore) Load() (domain.AppState, error) {
	data, err := os.ReadFile(filepath.Clean(s.path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.DefaultAppState(), nil
		}
		return domain.DefaultAppState(), fmt.Errorf("statestore: read %q: %w", s.path, err)
	}
	var st domain.AppState
	if err := yaml.Unmarshal(data, &st); err != nil {
		return domain.DefaultAppState(), fmt.Errorf("statestore: parse %q: %w", s.path, err)
	}
	warnLegacyFilter(data, s.path)
	return st, nil
}

// warnLegacyFilter logs once when data still carries the pre-exclusion-model
// filter keys, so a reset MR filter has a discoverable explanation instead of
// silently vanishing. Parse failure here is never fatal — Load has already
// parsed the same bytes into the current format successfully.
func warnLegacyFilter(data []byte, path string) {
	var legacy legacyAppState
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return
	}
	lf := legacy.Filter
	if len(lf.Assignees) == 0 && len(lf.Reviewers) == 0 && len(lf.TicketKeys) == 0 {
		return
	}
	slog.Default().Warn(
		"statestore: state.yaml has a pre-upgrade MR filter selection; "+
			"the Filters tab now excludes rather than includes, so it was reset to show all",
		"path", path)
}

// Save writes state to disk with mode 0600.
func (s *YAMLStore) Save(st domain.AppState) error {
	data, err := yaml.Marshal(st)
	if err != nil {
		return fmt.Errorf("statestore: marshal: %w", err)
	}
	if err := os.WriteFile(filepath.Clean(s.path), data, fileMode); err != nil {
		return fmt.Errorf("statestore: write %q: %w", s.path, err)
	}
	return nil
}
