package core

import (
	"context"
	"time"

	"github.com/ceffo/mrboard/internal/adapters/demoadpt"
	"github.com/ceffo/mrboard/internal/config"
	ilog "github.com/ceffo/mrboard/internal/log"
)

// applyDemoSettings overlays a fixture's settings onto the demo config. An unset
// setting leaves the config.DemoConfig default in place.
func applyDemoSettings(cfg *config.AppConfig, s demoadpt.Settings) {
	if s.CurrentUser != "" {
		cfg.CurrentUser = s.CurrentUser
	}
	if len(s.Team) > 0 {
		cfg.Sources = []config.Source{{Type: "user", IDs: s.Team}}
	}
	cfg.AutoAssignReviewers.Enabled = s.AutoAssignReviewers
}

// NewDemo wires the application against the built-in demo dataset: no clients,
// no credentials, no network.
//
// It is a separate constructor rather than a branch inside New because New
// builds the real state and snapshot stores, and both create their directories
// under the user's XDG paths at construction time. Demo mode must not touch
// those, so the only safe thing is never to call them.
//
// fixture names the embedded dataset ("" for the default); the dataset's own
// settings are applied onto cfg, which is the demo's in-memory config.
func NewDemo(_ context.Context, cfg *config.AppConfig, fixture string) (*Core, error) {
	logCfg := cfg.LogConfig()
	logger, closer, err := ilog.New(ilog.Config{Path: logCfg.Path, Level: logCfg.Level})
	if err != nil {
		return nil, err
	}

	adpt, err := demoadpt.New(demoadpt.Config{
		Now:     time.Now(),
		Fixture: fixture,
		BaseURL: cfg.GitLab.URL,
		Logger:  logger,
	})
	if err != nil {
		closer.Close()
		return nil, err
	}
	applyDemoSettings(cfg, adpt.Settings())

	// One instance serves both ticket ports, as in New.
	ticketAdpt := adpt.Tickets()
	return &Core{
		MRSource:       adpt.MRSource(),
		StateStore:     adpt.StateStore(),
		SnapshotStore:  adpt.SnapshotStore(),
		Notifier:       adpt.Notifier(),
		TicketEnricher: ticketAdpt,
		TicketLinker:   ticketAdpt,
		UpdateChecker:  adpt.UpdateChecker(),
		Config:         cfg,
		Logger:         logger,
		logCloser:      closer,
	}, nil
}
