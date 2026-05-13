// Package app is the composition root for mrboard binaries.
// It is the only place in the codebase where concrete adapter types meet
// service constructors. All other packages see interfaces only.
package app

import (
	"log/slog"

	"github.com/ceffo/mrboard/internal/adapters/gitlabadpt"
	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	pkggitlab "github.com/ceffo/mrboard/pkg/gitlab"
)

// Services holds every dependency a binary needs, fully wired.
type Services struct {
	Config   *config.AppConfig
	Logger   *slog.Logger
	MRSource mrsvc.MergeRequestSource
}

// New builds all services from the config at path (empty = XDG discovery).
// logger may be nil; pass slog.New(slog.DiscardHandler) for silence.
func New(cfgPath string, logger *slog.Logger) (*Services, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}

	if logger == nil {
		logger = slog.Default()
	}

	clientCfg := cfg.GitLabClientConfig()
	client, err := pkggitlab.NewClient(pkggitlab.Config{
		URL:     clientCfg.URL,
		Token:   clientCfg.Token,
		Timeout: clientCfg.Timeout,
	}, logger)
	if err != nil {
		return nil, err
	}

	adptCfg := cfg.GitLabAdapterConfig()
	sources := make([]mrsvc.Source, len(adptCfg.Sources))
	for i, s := range adptCfg.Sources {
		sources[i] = mrsvc.Source{
			Type:     s.Type,
			ID:       s.ID,
			Username: s.Username,
		}
	}

	adapter := gitlabadpt.New(client, gitlabadpt.Config{
		RequiredApprovals: adptCfg.RequiredApprovals,
		Sources:           sources,
		ExcludedAuthors:   adptCfg.ExcludedAuthors,
	})

	return &Services{
		Config:   cfg,
		Logger:   logger,
		MRSource: adapter,
	}, nil
}
