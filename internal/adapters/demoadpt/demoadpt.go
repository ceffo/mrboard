// Package demoadpt implements every driven port against a built-in, in-memory
// dataset, so `mrboard --demo` runs the whole board with no instance, no
// credentials, and no network.
//
// It is a shipped adapter backing a user-facing feature, not a test double:
// generated mocks live in sibling mocks/ packages and never link into the
// binary, whereas this package does. The dataset is a curated narrative — a
// board that shows every column, reviewer state, and age bucket — which is not
// something a generated mock can express.
//
// Naming here follows the repo's no-vendor-bleeding rule: nothing in this
// package is named for a provider. It reads and writes the vendor-named fields
// that internal/domain already exposes, which is what every other consumer of
// the domain does.
package demoadpt

import (
	"log/slog"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/ticketsvc"
)

// defaultLatency is a deliberate delay on FetchAll so the header's spinner and
// its snapshot-age indicator are actually visible instead of flicking past in
// one frame. It is a constant, so runs stay reproducible.
const defaultLatency = 900 * time.Millisecond

// Config parameterises the demo adapter.
type Config struct {
	// Now anchors every relative age in the dataset. Ages are stored as offsets
	// from this instant, so the fixture never drifts as it ages in git.
	Now time.Time
	// BaseURL is used to build MR web URLs. Nothing dereferences them in demo
	// mode, but they must look real in the detail pane.
	BaseURL string
	// Latency delays FetchAll. Zero means defaultLatency; use a negative value
	// in tests to disable it.
	Latency time.Duration
	Logger  *slog.Logger
}

// Adapter owns the demo dataset and hands out the port implementations that
// share it.
type Adapter struct {
	ds      *dataset
	logger  *slog.Logger
	latency time.Duration
}

// New loads the embedded dataset and materialises it against cfg.Now.
func New(cfg Config) (*Adapter, error) {
	now := cfg.Now
	if now.IsZero() {
		now = time.Now()
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	latency := cfg.Latency
	switch {
	case latency == 0:
		latency = defaultLatency
	case latency < 0:
		latency = 0
	}

	ds, err := loadFixture(now, cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	ds.savedState = domain.DefaultAppState()
	logger.Info("demo: dataset loaded", "mrs", len(ds.mrs), "projects", len(ds.projectPaths))
	return &Adapter{ds: ds, logger: logger, latency: latency}, nil
}

// MRSource returns the merge-request source port.
func (a *Adapter) MRSource() mrsvc.MergeRequestSource { return &mrSource{a: a} }

// TicketPorts is the pair of issue-tracker ports, exposed together because one
// instance satisfies both — mirroring the real adapter, where the enricher and
// the linker share per-session state.
type TicketPorts interface {
	ticketsvc.TicketEnricher
	ticketsvc.TicketLinker
}

// Tickets returns the issue-tracker ports; one instance satisfies both.
func (a *Adapter) Tickets() TicketPorts { return &tickets{a: a} }

// StateStore returns the in-memory app-state port.
func (a *Adapter) StateStore() domain.StateStore { return &stateStore{a: a} }

// SnapshotStore returns the in-memory snapshot port. It never touches the
// user's real cache directory.
func (a *Adapter) SnapshotStore() domain.SnapshotStore { return &snapshotStore{a: a} }

// Notifier returns a notifier that accepts and discards.
func (a *Adapter) Notifier() domain.Notifier { return &notifier{a: a} }

var (
	_ ticketsvc.TicketEnricher = (*tickets)(nil)
	_ ticketsvc.TicketLinker   = (*tickets)(nil)
)
