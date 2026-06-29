// Package jiraadpt implements jirasvc.JiraEnricher using pkg/jira.Client
// with a JSON disk cache (default TTL 24h).
package jiraadpt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgjira "github.com/ceffo/mrboard/pkg/jira"
)

// jiraClient is the subset of pkg/jira.Client used by this adapter.
// Defined as a local interface so tests can substitute a fake.
type jiraClient interface {
	GetIssue(ctx context.Context, issueKey string) (*pkgjira.Issue, error)
	GetActiveSprint(ctx context.Context, boardID int) (*pkgjira.Sprint, error)
	GetSprintIssueKeys(ctx context.Context, sprintID int) ([]string, error)
}

// Config holds adapter-specific settings.
type Config struct {
	// CacheDir is the directory where cache files are written.
	// Defaults to os.UserCacheDir()/mrboard/jira when empty.
	CacheDir string
	// TTL is the cache lifetime. Zero disables caching.
	TTL time.Duration
}

const (
	cacheDirPerm  = 0o700
	cacheFilePerm = 0o600
)

// cacheEntry wraps a cached JSON value with its expiry timestamp.
type cacheEntry struct {
	Value     json.RawMessage `json:"v"`
	ExpiresAt time.Time       `json:"e"`
}

// JiraAdapter implements jirasvc.JiraEnricher backed by a live JIRA client
// and a write-through JSON disk cache.
type JiraAdapter struct {
	client jiraClient
	cfg    Config
	logger *slog.Logger
}

// New returns a JiraAdapter wired to the given client, config, and logger.
// If cfg.CacheDir is empty, the OS user cache directory is used.
func New(client jiraClient, cfg Config, logger *slog.Logger) *JiraAdapter {
	if cfg.CacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		cfg.CacheDir = filepath.Join(base, "mrboard", "jira")
	}
	return &JiraAdapter{client: client, cfg: cfg, logger: logger}
}

// GetIssueType implements jirasvc.JiraEnricher.
// Returns ("", nil) when the issue is not found.
func (a *JiraAdapter) GetIssueType(ctx context.Context, issueKey string) (string, error) {
	filename := a.cacheFile("issue_" + sanitizeKey(issueKey) + ".json")

	var cached string
	if a.readCache(filename, &cached) {
		a.logger.Debug("jiraadpt: cache hit", "key", issueKey)
		return cached, nil
	}

	issue, err := a.client.GetIssue(ctx, issueKey)
	if err != nil {
		return "", fmt.Errorf("jiraadpt: get issue type %q: %w", issueKey, err)
	}
	if issue == nil {
		return "", nil
	}

	a.writeCache(filename, issue.Type)
	return issue.Type, nil
}

// GetActiveSprintIssueKeys implements jirasvc.JiraEnricher.
// Returns (nil, nil) when no active sprint exists for boardID.
func (a *JiraAdapter) GetActiveSprintIssueKeys(ctx context.Context, boardID int) ([]string, error) {
	filename := a.cacheFile(fmt.Sprintf("sprint_board_%d.json", boardID))

	var cached []string
	if a.readCache(filename, &cached) {
		a.logger.Debug("jiraadpt: cache hit", "board_id", boardID, "count", len(cached))
		return cached, nil
	}

	sprint, err := a.client.GetActiveSprint(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("jiraadpt: get active sprint for board %d: %w", boardID, err)
	}
	if sprint == nil {
		return nil, nil
	}

	keys, err := a.client.GetSprintIssueKeys(ctx, sprint.ID)
	if err != nil {
		return nil, fmt.Errorf("jiraadpt: get sprint %d issue keys: %w", sprint.ID, err)
	}

	a.writeCache(filename, keys)
	return keys, nil
}

// readCache reads a cache entry from filename and JSON-unmarshals its value
// into dest. Returns false on any error (miss, expiry, or decode failure).
func (a *JiraAdapter) readCache(filename string, dest any) bool {
	if a.cfg.TTL <= 0 {
		return false
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return false
	}
	if time.Now().After(entry.ExpiresAt) {
		return false
	}
	return json.Unmarshal(entry.Value, dest) == nil
}

// writeCache serializes value to a cache entry file with expiry = now + TTL.
// Errors are logged as warnings — callers still receive live data.
func (a *JiraAdapter) writeCache(filename string, value any) {
	if a.cfg.TTL <= 0 {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		a.logger.Warn("jiraadpt: cache marshal failed", "filename", filename, "err", err)
		return
	}
	entry := cacheEntry{Value: raw, ExpiresAt: time.Now().Add(a.cfg.TTL)}
	data, err := json.Marshal(entry)
	if err != nil {
		a.logger.Warn("jiraadpt: cache entry marshal failed", "filename", filename, "err", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(filename), cacheDirPerm); err != nil {
		a.logger.Warn("jiraadpt: cache dir create failed", "dir", filepath.Dir(filename), "err", err)
		return
	}
	if err := os.WriteFile(filename, data, cacheFilePerm); err != nil {
		a.logger.Warn("jiraadpt: cache write failed", "filename", filename, "err", err)
	}
}

func (a *JiraAdapter) cacheFile(name string) string {
	return filepath.Join(a.cfg.CacheDir, name)
}

// sanitizeKey replaces characters unsafe for filenames with underscores.
func sanitizeKey(s string) string {
	return strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(s)
}
