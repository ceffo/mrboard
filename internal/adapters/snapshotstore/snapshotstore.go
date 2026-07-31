// Package snapshotstore provides a JSON-backed implementation of domain.SnapshotStore.
package snapshotstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
)

const (
	dirMode  = 0o700
	fileMode = 0o600

	// currentVersion is bumped whenever fileFormat's shape changes incompatibly.
	// A mismatch on load is treated as a cold cache, not an error (see Load).
	currentVersion = 1
)

// Config holds configuration for the JSON-backed snapshot store.
type Config struct {
	Dir string // XDG cache dir: ~/.cache/mrboard/
}

// fileFormat is the on-disk shape of snapshot.json.
type fileFormat struct {
	Version   int                   `json:"version"`
	WrittenAt time.Time             `json:"written_at"`
	MRs       []domain.MergeRequest `json:"mrs"`
}

// JSONStore persists the last-known set of MRs to {Dir}/snapshot.json.
type JSONStore struct {
	path string
}

// New creates a JSONStore, ensuring the cache directory exists (mode 0700).
func New(cfg Config) (*JSONStore, error) {
	if err := os.MkdirAll(cfg.Dir, dirMode); err != nil {
		return nil, fmt.Errorf("snapshotstore: create dir %q: %w", cfg.Dir, err)
	}
	return &JSONStore{path: filepath.Join(cfg.Dir, "snapshot.json")}, nil
}

// Load reads the persisted snapshot. An absent file, a corrupt file, and a
// version mismatch all yield (nil, zero time.Time, nil): losing this file
// must cost nothing but one slow fetch, so no error is ever surfaced.
func (s *JSONStore) Load() ([]domain.MergeRequest, time.Time, error) {
	data, err := os.ReadFile(filepath.Clean(s.path))
	if err != nil {
		return nil, time.Time{}, nil
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, time.Time{}, nil
	}
	if f.Version != currentVersion {
		return nil, time.Time{}, nil
	}
	return f.MRs, f.WrittenAt, nil
}

// Save writes the snapshot to disk with mode 0600, stamped with the current time.
func (s *JSONStore) Save(mrs []domain.MergeRequest) error {
	f := fileFormat{Version: currentVersion, WrittenAt: time.Now(), MRs: mrs}
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("snapshotstore: marshal: %w", err)
	}
	if err := os.WriteFile(filepath.Clean(s.path), data, fileMode); err != nil {
		return fmt.Errorf("snapshotstore: write %q: %w", s.path, err)
	}
	return nil
}
