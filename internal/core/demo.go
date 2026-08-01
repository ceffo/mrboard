package core

import (
	"context"
	"time"

	"github.com/ceffo/mrboard/internal/adapters/demoadpt"
	"github.com/ceffo/mrboard/internal/config"
	ilog "github.com/ceffo/mrboard/internal/log"
)

// NewDemo wires the application against the built-in demo dataset: no clients,
// no credentials, no network.
//
// It is a separate constructor rather than a branch inside New because New
// builds the real state and snapshot stores, and both create their directories
// under the user's XDG paths at construction time. Demo mode must not touch
// those, so the only safe thing is never to call them.
func NewDemo(_ context.Context, cfg *config.AppConfig) (*Core, error) {
	logCfg := cfg.LogConfig()
	logger, closer, err := ilog.New(ilog.Config{Path: logCfg.Path, Level: logCfg.Level})
	if err != nil {
		return nil, err
	}

	adpt, err := demoadpt.New(demoadpt.Config{
		Now:     time.Now(),
		BaseURL: cfg.GitLab.URL,
		Logger:  logger,
	})
	if err != nil {
		closer.Close()
		return nil, err
	}

	// One instance serves both ticket ports, as in New.
	ticketAdpt := adpt.Tickets()
	return &Core{
		MRSource:       adpt.MRSource(),
		StateStore:     adpt.StateStore(),
		SnapshotStore:  adpt.SnapshotStore(),
		Notifier:       adpt.Notifier(),
		TicketEnricher: ticketAdpt,
		TicketLinker:   ticketAdpt,
		Config:         cfg,
		Logger:         logger,
		logCloser:      closer,
	}, nil
}
