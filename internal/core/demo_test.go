package core

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// TestNewDemoTouchesNoUserDirectory is the load-bearing test for demo mode's
// central promise. The real state and snapshot stores create their directories
// at construction time, so avoiding Save is not enough — the constructors must
// never run. This asserts nothing appears under either XDG root, even after
// both stores have been written to.
func TestNewDemoTouchesNoUserDirectory(t *testing.T) {
	dataDir, cacheDir := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	c, err := NewDemo(context.Background(), config.DemoConfig())
	require.NoError(t, err)
	t.Cleanup(func() { c.Close(context.Background()) })

	require.NoError(t, c.StateStore.Save(domain.DefaultAppState()))
	mrs, errs := c.MRSource.FetchAll(context.Background(), mrsvc.FetchOptions{})
	require.Empty(t, errs)
	require.NotEmpty(t, mrs)
	require.NoError(t, c.SnapshotStore.Save(mrs))

	for label, dir := range map[string]string{"XDG_DATA_HOME": dataDir, "XDG_CACHE_HOME": cacheDir} {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, entries, "demo mode wrote into %s (%s)", label, dir)
	}
}

// TestNewDemoWiresEveryPort guards against a half-wired Core, which would show up
// as a nil-dereference panic or a silently missing feature at runtime.
func TestNewDemoWiresEveryPort(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	c, err := NewDemo(context.Background(), config.DemoConfig())
	require.NoError(t, err)
	t.Cleanup(func() { c.Close(context.Background()) })

	assert.NotNil(t, c.MRSource)
	assert.NotNil(t, c.StateStore)
	assert.NotNil(t, c.SnapshotStore)
	assert.NotNil(t, c.Notifier, "the notification key is gated on a non-nil notifier")
	assert.NotNil(t, c.TicketEnricher, "ticket icons and the sprint filter need the enricher")
	assert.NotNil(t, c.TicketLinker)
	assert.NotNil(t, c.Logger)
	assert.NotNil(t, c.Config)
}

// TestNewDemoBootsWarm asserts the board has data before the first fetch, which
// is what lets the demo open on an interactive board instead of a spinner.
func TestNewDemoBootsWarm(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	c, err := NewDemo(context.Background(), config.DemoConfig())
	require.NoError(t, err)
	t.Cleanup(func() { c.Close(context.Background()) })

	cached, writtenAt, err := c.SnapshotStore.Load()
	require.NoError(t, err)
	assert.NotEmpty(t, cached, "the demo snapshot store must serve a warm cache")
	assert.False(t, writtenAt.IsZero(), "the header needs a snapshot age to display")
}
