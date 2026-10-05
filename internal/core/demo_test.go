package core

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/adapters/demoadpt"
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

	c, err := NewDemo(context.Background(), config.DemoConfig(), "")
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

	c, err := NewDemo(context.Background(), config.DemoConfig(), "")
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

	c, err := NewDemo(context.Background(), config.DemoConfig(), "")
	require.NoError(t, err)
	t.Cleanup(func() { c.Close(context.Background()) })

	cached, writtenAt, err := c.SnapshotStore.Load()
	require.NoError(t, err)
	assert.NotEmpty(t, cached, "the demo snapshot store must serve a warm cache")
	assert.False(t, writtenAt.IsZero(), "the header needs a snapshot age to display")
}

const (
	demoTestUser  = "grace"
	demoTestOther = "linus"
)

func TestApplyDemoSettings_ZeroValueLeavesTheDefaultConfigAlone(t *testing.T) {
	cfg := config.DemoConfig()
	want := *config.DemoConfig()

	applyDemoSettings(cfg, demoadpt.Settings{})

	assert.Equal(t, want, *cfg)
}

func TestApplyDemoSettings_OverridesWhatTheFixtureSets(t *testing.T) {
	cfg := config.DemoConfig()

	applyDemoSettings(cfg, demoadpt.Settings{
		CurrentUser:         demoTestUser,
		Team:                []string{demoTestUser, demoTestOther},
		AutoAssignReviewers: true,
	})

	assert.Equal(t, demoTestUser, cfg.CurrentUser)
	assert.Equal(t, []string{demoTestUser, demoTestOther}, config.TeamUsernames(cfg.Sources))
	assert.True(t, cfg.AutoAssignReviewers.Enabled)
}

// TestNewDemoBootsEveryFixtureAndAppliesItsSettings guards the wiring between a
// fixture's settings block and the config the board runs with.
func TestNewDemoBootsEveryFixtureAndAppliesItsSettings(t *testing.T) {
	for _, name := range demoadpt.Fixtures() {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			t.Setenv("XDG_CACHE_HOME", t.TempDir())

			c, err := NewDemo(context.Background(), config.DemoConfig(), name)
			require.NoError(t, err)
			t.Cleanup(func() { c.Close(context.Background()) })

			assert.Equal(t, name == "auto-assign", c.Config.AutoAssignReviewers.Enabled,
				"automatic assignment is on for the auto-assign fixture and no other")
		})
	}
}

func TestNewDemoRejectsAnUnknownFixture(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	_, err := NewDemo(context.Background(), config.DemoConfig(), "nope")

	require.Error(t, err)
}
