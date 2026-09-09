// Package githubadpt implements updatesvc.UpdateChecker using pkg/github.Client,
// with a disk cache (default TTL 24h) built on github.com/eko/gocache and
// pkg/diskstore.
package githubadpt

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/spf13/afero"

	"github.com/ceffo/mrboard/internal/domain/service/updatesvc"
	"github.com/ceffo/mrboard/pkg/diskstore"
	pkggithub "github.com/ceffo/mrboard/pkg/github"
)

// releaseClient is the subset of pkg/github.Client used by this adapter.
// Defined as a local interface so tests can substitute a fake.
type releaseClient interface {
	GetLatestRelease(ctx context.Context) (*pkggithub.Release, error)
}

const latestTagCacheKey = "latest_release_tag"

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
	// TTL is the cache lifetime for the latest-release lookup. Zero or
	// negative disables caching. Kept long (default 24h): a new mrboard
	// release ships far less often than JIRA issue-type or sprint changes.
	TTL time.Duration
}

// GitHubAdapter implements updatesvc.UpdateChecker backed by a live GitHub
// client and a write-through disk cache.
type GitHubAdapter struct {
	client releaseClient
	cache  *marshaler.Marshaler
	cfg    Config
	logger *slog.Logger
}

// New returns a GitHubAdapter wired to the given client, config, and logger.
func New(client releaseClient, cfg Config, logger *slog.Logger) (*GitHubAdapter, error) {
	st, err := diskstore.New(diskstore.Config{FS: cfg.FS, Dir: cfg.CacheDir})
	if err != nil {
		return nil, fmt.Errorf("githubadpt: %w", err)
	}
	return &GitHubAdapter{
		client: client,
		cache:  marshaler.New(cache.New[any](st)),
		cfg:    cfg,
		logger: logger,
	}, nil
}

// CheckForUpdate implements updatesvc.UpdateChecker. currentVersion == "dev"
// (or anything else ParseVersion rejects) never calls the client and never
// reports an update available.
func (a *GitHubAdapter) CheckForUpdate(ctx context.Context, currentVersion string) (updatesvc.Info, error) {
	current, ok := updatesvc.ParseVersion(currentVersion)
	if !ok {
		return updatesvc.Info{}, nil
	}

	var cachedTag string
	if a.getCache(ctx, &cachedTag) {
		a.logger.Debug("githubadpt: cache hit", "tag", cachedTag)
		return a.compare(current, cachedTag), nil
	}

	rel, err := a.client.GetLatestRelease(ctx)
	if err != nil {
		return updatesvc.Info{}, fmt.Errorf("githubadpt: get latest release: %w", err)
	}
	if rel == nil {
		return updatesvc.Info{}, nil
	}

	a.setCache(ctx, rel.TagName)
	return a.compare(current, rel.TagName), nil
}

func (a *GitHubAdapter) compare(current updatesvc.ParsedVersion, tag string) updatesvc.Info {
	latest, ok := updatesvc.ParseVersion(tag)
	if !ok || !updatesvc.IsNewer(current, latest) {
		return updatesvc.Info{}
	}
	return updatesvc.Info{Available: true, Latest: tag}
}

// getCache reads the cached latest-release tag. A miss, an expired or corrupt
// entry, and caching disabled via cfg.TTL are all treated the same way:
// return false so the caller falls back to a live fetch.
func (a *GitHubAdapter) getCache(ctx context.Context, dest *string) bool {
	if a.cfg.TTL <= 0 {
		return false
	}
	_, err := a.cache.Get(ctx, latestTagCacheKey, dest)
	return err == nil
}

// setCache writes the latest-release tag to the disk cache, honoring
// cfg.TTL. Write failures are logged and otherwise ignored — the caller
// already has the live value in hand regardless of whether the cache write
// succeeds.
func (a *GitHubAdapter) setCache(ctx context.Context, tag string) {
	if a.cfg.TTL <= 0 {
		return
	}
	if err := a.cache.Set(ctx, latestTagCacheKey, tag, store.WithExpiration(a.cfg.TTL)); err != nil {
		a.logger.Warn("githubadpt: cache write failed", "err", err)
	}
}
