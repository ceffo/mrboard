// Package jiraadpt implements ticketsvc.TicketEnricher and ticketsvc.TicketLinker
// using pkg/jira.Client, with a disk cache (default TTL 24h) built on
// github.com/eko/gocache and pkg/diskstore.
package jiraadpt

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/spf13/afero"

	"github.com/ceffo/mrboard/pkg/diskstore"
	pkgjira "github.com/ceffo/mrboard/pkg/jira"
)

// jiraClient is the subset of pkg/jira.Client used by this adapter.
// Defined as a local interface so tests can substitute a fake.
type jiraClient interface {
	GetIssue(ctx context.Context, issueKey string) (*pkgjira.Issue, error)
	GetActiveSprint(ctx context.Context, boardID int) (*pkgjira.Sprint, error)
	GetSprintIssueKeys(ctx context.Context, sprintID int) ([]string, error)
	GetRemoteLink(ctx context.Context, issueKey, globalID string) (string, error)
	CreateOrUpdateRemoteLink(ctx context.Context, issueKey string, link pkgjira.RemoteLink) error
}

// Config holds adapter-specific settings.
type Config struct {
	// FS is the filesystem the disk cache is written through. Production
	// callers pass afero.NewOsFs(); tests pass afero.NewMemMapFs() to avoid
	// touching disk.
	FS afero.Fs
	// CacheDir is the directory cache entries are written to. Callers must
	// supply an already-resolved path (e.g. config.XDGCacheDir()); this
	// package applies no platform-specific defaulting of its own.
	CacheDir string
	// TTL is the cache lifetime. Zero or negative disables caching.
	TTL time.Duration
	// LinkIconURL is the URL of a 16×16 icon shown next to remote links in JIRA.
	// Empty string omits the icon field from the payload.
	LinkIconURL string
}

// JiraAdapter implements ticketsvc.TicketEnricher and ticketsvc.TicketLinker backed
// by a live JIRA client and a write-through disk cache.
type JiraAdapter struct {
	client     jiraClient
	cache      *marshaler.Marshaler
	cfg        Config
	logger     *slog.Logger
	sessionMap sync.Map // globalID → last-written mrTitle; resets on process restart
}

// New returns a JiraAdapter wired to the given client, config, and logger.
func New(client jiraClient, cfg Config, logger *slog.Logger) (*JiraAdapter, error) {
	st, err := diskstore.New(diskstore.Config{FS: cfg.FS, Dir: cfg.CacheDir})
	if err != nil {
		return nil, fmt.Errorf("jiraadpt: %w", err)
	}
	return &JiraAdapter{
		client: client,
		cache:  marshaler.New(cache.New[any](st)),
		cfg:    cfg,
		logger: logger,
	}, nil
}

// GetIssueType implements ticketsvc.TicketEnricher.
// Returns ("", nil) when the issue is not found.
func (a *JiraAdapter) GetIssueType(ctx context.Context, issueKey string) (string, error) {
	key := issueTypeCacheKey(issueKey)

	var cached string
	if a.getCache(ctx, key, &cached) {
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

	a.setCache(ctx, key, issue.Type)
	return issue.Type, nil
}

// GetActiveSprintIssueKeys implements ticketsvc.TicketEnricher.
// Returns (nil, nil) when no active sprint exists for boardID. forceRefresh
// skips the cache read and always fetches live, then rewrites the cache entry
// with the fresh result (and a new expiry).
func (a *JiraAdapter) GetActiveSprintIssueKeys(ctx context.Context, boardID int, forceRefresh bool) ([]string, error) {
	key := sprintCacheKey(boardID)

	var cached []string
	if !forceRefresh && a.getCache(ctx, key, &cached) {
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

	a.setCache(ctx, key, keys)
	return keys, nil
}

// UpsertRemoteLink implements ticketsvc.TicketLinker.
// It is idempotent across three layers:
//  1. Session sync.Map: skips the call entirely if this globalID+title was
//     already written in this process lifetime.
//  2. Disk cache: skips if the persisted last-written title matches mrTitle.
//  3. GET-before-write (only on disk-cache miss): fetches the current JIRA
//     state before writing, so a first-run against a JIRA instance that already
//     has the correct link does not generate a spurious change-history entry.
func (a *JiraAdapter) UpsertRemoteLink(ctx context.Context, issueKey, globalID, mrTitle, mrURL string) error {
	// Layer 1: session dedup
	if v, ok := a.sessionMap.Load(globalID); ok && v.(string) == mrTitle {
		a.logger.Debug("jiraadpt: remote link session hit", "globalId", globalID)
		return nil
	}

	key := remoteLinkCacheKey(globalID)

	// Layer 2: disk cache
	var cachedTitle string
	diskHit := a.getCache(ctx, key, &cachedTitle)
	if diskHit && cachedTitle == mrTitle {
		a.logger.Debug("jiraadpt: remote link disk cache hit", "globalId", globalID)
		a.sessionMap.Store(globalID, mrTitle)
		return nil
	}

	// Layer 3: GET-before-write (only on cold disk miss)
	action := "update" // disk cache existed but title changed
	if !diskHit {
		existing, err := a.client.GetRemoteLink(ctx, issueKey, globalID)
		if err != nil {
			return fmt.Errorf("jiraadpt: get remote link %q on %q: %w", globalID, issueKey, err)
		}
		if existing == mrTitle {
			a.logger.Debug("jiraadpt: remote link JIRA already current", "globalId", globalID)
			a.setCache(ctx, key, mrTitle)
			a.sessionMap.Store(globalID, mrTitle)
			return nil
		}
		if existing == "" {
			action = "create"
		}
	}

	// Write the remote link (create or update)
	a.logger.Info("jiraadpt: writing remote link",
		"action", action,
		"issueKey", issueKey,
		"globalId", globalID,
		"title", mrTitle,
		"url", mrURL,
	)
	link := pkgjira.RemoteLink{
		GlobalID:     globalID,
		Relationship: "mentioned in",
		Object: pkgjira.RemoteLinkObject{
			Title: mrTitle,
			URL:   mrURL,
			Icon:  a.linkIcon(),
		},
	}
	if err := a.client.CreateOrUpdateRemoteLink(ctx, issueKey, link); err != nil {
		return fmt.Errorf("jiraadpt: upsert remote link %q on %q: %w", globalID, issueKey, err)
	}

	a.logger.Info("jiraadpt: remote link written", "action", action, "issueKey", issueKey, "globalId", globalID)
	a.setCache(ctx, key, mrTitle)
	a.sessionMap.Store(globalID, mrTitle)
	return nil
}

// linkIcon returns the configured remote link icon, or nil when no icon URL
// was configured. The adapter has no knowledge of what the icon represents.
func (a *JiraAdapter) linkIcon() *pkgjira.RemoteLinkIcon {
	if a.cfg.LinkIconURL == "" {
		return nil
	}
	return &pkgjira.RemoteLinkIcon{URL16x16: a.cfg.LinkIconURL}
}

// getCache reads dest from the disk cache under key. A miss, an expired or
// corrupt entry, and caching disabled via cfg.TTL are all treated the same
// way: return false so the caller falls back to a live fetch.
func (a *JiraAdapter) getCache(ctx context.Context, key string, dest any) bool {
	if a.cfg.TTL <= 0 {
		return false
	}
	_, err := a.cache.Get(ctx, key, dest)
	return err == nil
}

// setCache writes value to the disk cache under key, honoring cfg.TTL. Write
// failures are logged and otherwise ignored — callers already have the live
// value in hand regardless of whether the cache write succeeds.
func (a *JiraAdapter) setCache(ctx context.Context, key string, value any) {
	if a.cfg.TTL <= 0 {
		return
	}
	if err := a.cache.Set(ctx, key, value, store.WithExpiration(a.cfg.TTL)); err != nil {
		a.logger.Warn("jiraadpt: cache write failed", "key", key, "err", err)
	}
}

func issueTypeCacheKey(issueKey string) string { return "issue_" + issueKey }

func sprintCacheKey(boardID int) string { return fmt.Sprintf("sprint_board_%d", boardID) }

func remoteLinkCacheKey(globalID string) string { return "remotelink_" + globalID }
