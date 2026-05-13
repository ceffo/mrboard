// Package app is the composition root for mrboard binaries.
// It is the only place in the codebase where concrete adapter types meet
// service constructors. All other packages see interfaces only.
package app

import (
	"log/slog"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/gitlab"
	"github.com/ceffo/mrboard/internal/service"
)

// Services holds every dependency a binary needs, fully wired.
type Services struct {
	Config   *config.AppConfig
	Logger   *slog.Logger
	MRSource service.MergeRequestSource
}

// New builds all services from the config at path (empty = XDG discovery).
// logger may be nil; pass slog.New(slog.DiscardHandler) for silence.
func New(cfgPath string, logger *slog.Logger) (*Services, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}

	client, err := gitlab.NewClient(cfg, cfg.GitLab.Timeout, logger)
	if err != nil {
		return nil, err
	}

	return &Services{
		Config:   cfg,
		Logger:   logger,
		MRSource: service.NewGitLabSource(client, cfg),
	}, nil
}
