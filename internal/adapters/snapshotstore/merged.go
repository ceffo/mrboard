package snapshotstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
)

// mergedFileFormat is the on-disk shape of merged_snapshot.json.
type mergedFileFormat struct {
	Version   int                   `json:"version"`
	WrittenAt time.Time             `json:"written_at"`
	Since     time.Time             `json:"since"`
	MRs       []domain.MergeRequest `json:"mrs"`
}

// MergedJSONStore persists the merged MRs listed since a date to
// {Dir}/merged_snapshot.json, tagged with that date.
type MergedJSONStore struct {
	path string
}

// NewMerged creates a MergedJSONStore, ensuring the cache directory exists (mode 0700).
func NewMerged(cfg Config) (*MergedJSONStore, error) {
	if err := os.MkdirAll(cfg.Dir, dirMode); err != nil {
		return nil, fmt.Errorf("snapshotstore: create dir %q: %w", cfg.Dir, err)
	}
	return &MergedJSONStore{path: filepath.Join(cfg.Dir, "merged_snapshot.json")}, nil
}

// Load returns the merged MRs stored for since. A snapshot tagged with another
// date is deleted; it and an absent, corrupt or other-version file all yield
// (nil, nil), as for the board snapshot.
func (s *MergedJSONStore) Load(since time.Time) ([]domain.MergeRequest, error) {
	data, err := os.ReadFile(filepath.Clean(s.path))
	if err != nil {
		return nil, nil
	}
	var f mergedFileFormat
	if err := json.Unmarshal(data, &f); err != nil || f.Version != currentVersion {
		return nil, nil
	}
	if !f.Since.Equal(since) {
		if err := os.Remove(filepath.Clean(s.path)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("snapshotstore: drop merged snapshot %q: %w", s.path, err)
		}
		return nil, nil
	}
	return f.MRs, nil
}

// Save replaces the stored snapshot with the merged MRs of mrs, tagged with since.
func (s *MergedJSONStore) Save(since time.Time, mrs []domain.MergeRequest) error {
	f := mergedFileFormat{Version: currentVersion, WrittenAt: time.Now(), Since: since, MRs: domain.MergedMRs(mrs)}
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("snapshotstore: marshal merged: %w", err)
	}
	if err := os.WriteFile(filepath.Clean(s.path), data, fileMode); err != nil {
		return fmt.Errorf("snapshotstore: write %q: %w", s.path, err)
	}
	return nil
}
