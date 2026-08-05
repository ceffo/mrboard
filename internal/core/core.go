// Package core is the composition root for mrboard binaries.
// It wires concrete adapter types to service interfaces.
// No TUI imports are allowed here.
package core

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/ceffo/mrboard/internal/adapters/gitlabadpt"
	"github.com/ceffo/mrboard/internal/adapters/jiraadpt"
	"github.com/ceffo/mrboard/internal/adapters/snapshotstore"
	"github.com/ceffo/mrboard/internal/adapters/statestore"
	"github.com/ceffo/mrboard/internal/adapters/teamsnotify"
	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/ticketsvc"
	ilog "github.com/ceffo/mrboard/internal/log"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
	pkgjira "github.com/ceffo/mrboard/pkg/jira"
)

// Core holds every dependency a binary needs, fully wired.
type Core struct {
	MRSource       mrsvc.MergeRequestSource
	StateStore     domain.StateStore
	SnapshotStore  domain.SnapshotStore
	Notifier       domain.Notifier
	TicketEnricher ticketsvc.TicketEnricher // nil when the issue tracker is not configured
	TicketLinker   ticketsvc.TicketLinker   // nil when not configured; same adapter instance as TicketEnricher
	Config         *config.AppConfig
	Logger         *slog.Logger
	logCloser      io.Closer
}

// New builds all services from the provided config.
func New(_ context.Context, cfg *config.AppConfig) (*Core, error) {
	// 1. Logger
	logCfg := cfg.LogConfig()
	logger, closer, err := ilog.New(ilog.Config{Path: logCfg.Path, Level: logCfg.Level})
	if err != nil {
		return nil, err
	}

	// 2. GitLab client
	clientCfg := cfg.GitLabClientConfig()
	client, err := pkggitlab.NewClient(pkggitlab.Config{
		URL:     clientCfg.URL,
		Token:   clientCfg.Token,
		Timeout: clientCfg.Timeout,
	}, logger)
	if err != nil {
		closer.Close()
		return nil, err
	}

	// 3. GitLab adapter
	adptCfg := cfg.GitLabAdapterConfig()
	sources := make([]mrsvc.Source, len(adptCfg.Sources))
	for i, s := range adptCfg.Sources {
		sources[i] = mrsvc.Source{
			Type: mrsvc.SourceType(s.Type),
			IDs:  s.IDs,
		}
	}
	adapter := gitlabadpt.New(client, gitlabadpt.Config{
		Sources:           sources,
		ExcludedAuthors:   adptCfg.ExcludedAuthors,
		ReviewerUsernames: deriveReviewerUsernames(sources, adptCfg.CurrentUser),
	})

	// 4. State store
	store, err := statestore.New(statestore.Config{Dir: config.XDGDataDir()})
	if err != nil {
		closer.Close()
		return nil, err
	}

	// 4b. Snapshot store (incremental-fetch cache, see docs/adr/0005)
	snapStore, err := snapshotstore.New(snapshotstore.Config{Dir: config.XDGCacheDir()})
	if err != nil {
		closer.Close()
		return nil, err
	}

	var notifier domain.Notifier
	if teamsCfg := cfg.Notifications.Teams; teamsCfg.WebhookURL != "" {
		notifier = teamsnotify.New(teamsnotify.Config{
			WebhookURL:    teamsCfg.WebhookURL,
			UserMappings:  teamsCfg.UserMappings,
			UserIDs:       teamsCfg.UserIDs,
			TicketBaseURL: cfg.Jira.InstanceURL,
		}, logger)
	}

	// 5. Issue-tracker adapter (optional — only wired when all three credentials
	// are present). The same *jiraadpt.JiraAdapter instance satisfies both
	// TicketEnricher and TicketLinker so the session sync.Map for remote-link
	// dedup is shared.
	var ticketEnricher ticketsvc.TicketEnricher
	var ticketLinker ticketsvc.TicketLinker
	if j := cfg.Jira; j.InstanceURL != "" && j.Email != "" && j.APIToken != "" {
		jiraClient := pkgjira.NewClient(pkgjira.Config{
			InstanceURL: j.InstanceURL,
			Email:       j.Email,
			APIToken:    j.APIToken,
		})
		adpt, err := jiraadpt.New(jiraClient, jiraadpt.Config{
			FS:          afero.NewOsFs(),
			CacheDir:    filepath.Join(config.XDGCacheDir(), "jira"),
			TTL:         j.CacheTTL,
			LinkIconURL: j.RemoteLinkIconURL,
		}, logger)
		if err != nil {
			closer.Close()
			return nil, err
		}
		ticketEnricher = adpt
		ticketLinker = adpt
	}

	return &Core{
		MRSource:       adapter,
		StateStore:     store,
		SnapshotStore:  snapStore,
		Notifier:       notifier,
		TicketEnricher: ticketEnricher,
		TicketLinker:   ticketLinker,
		Config:         cfg,
		Logger:         logger,
		logCloser:      closer,
	}, nil
}

// Close releases resources held by Core (e.g. the log file).
func (c *Core) Close(_ context.Context) error {
	if c.logCloser != nil {
		return c.logCloser.Close()
	}
	return nil
}

// deriveReviewerUsernames collects usernames from user-type sources and appends
// currentUser, deduplicating across both sets.
func deriveReviewerUsernames(sources []mrsvc.Source, currentUser string) []string {
	seen := map[string]struct{}{}
	var names []string
	for _, src := range sources {
		if src.Type == mrsvc.SourceTypeUser {
			for _, id := range src.IDs {
				if _, ok := seen[id]; !ok {
					seen[id] = struct{}{}
					names = append(names, id)
				}
			}
		}
	}
	if currentUser != "" {
		if _, ok := seen[currentUser]; !ok {
			names = append(names, currentUser)
		}
	}
	return names
}
