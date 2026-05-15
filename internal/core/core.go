// Package core is the composition root for mrboard binaries.
// It wires concrete adapter types to service interfaces.
// No TUI imports are allowed here.
package core

import (
	"context"
	"io"
	"log/slog"

	"github.com/ceffo/mrboard/internal/adapters/gitlabadpt"
	"github.com/ceffo/mrboard/internal/adapters/statestore"
	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	ilog "github.com/ceffo/mrboard/internal/log"
	"github.com/ceffo/mrboard/internal/tui"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// Core holds every dependency a binary needs, fully wired.
type Core struct {
	MRSource   mrsvc.MergeRequestSource
	StateStore tui.StateStore
	Config     *config.AppConfig
	Logger     *slog.Logger
	logCloser  io.Closer
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
		RequiredApprovals: adptCfg.RequiredApprovals,
		Sources:           sources,
		ExcludedAuthors:   adptCfg.ExcludedAuthors,
	})

	// 4. State store
	store, err := statestore.New(statestore.Config{Dir: config.XDGDataDir()})
	if err != nil {
		closer.Close()
		return nil, err
	}

	return &Core{
		MRSource:   adapter,
		StateStore: store,
		Config:     cfg,
		Logger:     logger,
		logCloser:  closer,
	}, nil
}

// Close releases resources held by Core (e.g. the log file).
func (c *Core) Close(_ context.Context) error {
	if c.logCloser != nil {
		return c.logCloser.Close()
	}
	return nil
}
