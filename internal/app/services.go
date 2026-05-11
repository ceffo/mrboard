// Package app is the composition root for mrboard binaries.
// It is the only place in the codebase where concrete adapter types meet
// service constructors. All other packages see interfaces only.
package app

import (
	"io"
	"log/slog"
	"time"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/gitlab"
	"github.com/ceffo/mrboard/internal/service"
)

// Services holds every dependency a binary needs, fully wired.
type Services struct {
	Config   *config.Config
	Logger   *slog.Logger
	MRSource service.MergeRequestSource
}

// New builds all services from environment and config. timeout controls the
// HTTP deadline for every GitLab API call; pass 0 for no timeout.
// logger is used for structured output; pass slog.New(slog.DiscardHandler) for silence.
func New(timeout time.Duration, logger *slog.Logger) (*Services, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	client, err := gitlab.NewClient(cfg, timeout, logger)
	if err != nil {
		return nil, err
	}

	return &Services{
		Config:   cfg,
		Logger:   logger,
		MRSource: service.NewGitLabSource(client, cfg),
	}, nil
}
