package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDemoConfigIsValid pins the contract that demo mode bypasses Load but still
// produces a legal config — it is a real config, just never read from disk.
func TestDemoConfigIsValid(t *testing.T) {
	cfg := DemoConfig()

	warnings, err := validate(cfg)
	require.NoError(t, err)
	assert.Empty(t, warnings, "the demo config should not trip any startup warning")
}

// TestDemoConfigSetsEveryViperDefault is the guard against the trap in
// DemoConfig: anything Load would have defaulted is zero unless set by hand, and
// a zero lifetime threshold silently disables the board's age colouring.
func TestDemoConfigSetsEveryViperDefault(t *testing.T) {
	cfg := DemoConfig()

	assert.Equal(t, 30*time.Second, cfg.GitLab.Timeout)
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, 72*time.Hour, cfg.LifetimeWarnAfter)
	assert.Equal(t, 120*time.Hour, cfg.LifetimeErrorAfter)
	assert.Equal(t, 24*time.Hour, cfg.Jira.CacheTTL)
	assert.Equal(t, 60*time.Second, cfg.RefreshInterval)
}

// TestDemoConfigEnablesOptionalFeatures keeps the demo a full showcase: each of
// these fields is what gates a feature the recording is meant to show.
func TestDemoConfigEnablesOptionalFeatures(t *testing.T) {
	cfg := DemoConfig()

	assert.NotEmpty(t, cfg.CurrentUser, `"my view" toggle is gated on current_user`)
	assert.NotEmpty(t, cfg.Jira.InstanceURL, "the ticket line is gated on the tracker URL")
	assert.NotZero(t, cfg.Jira.BoardID, "the sprint filter is gated on a board ID")
	require.Len(t, cfg.Sources, 1)
	assert.Equal(t, "user", cfg.Sources[0].Type, "a user source is what populates the team roster")
	assert.Nil(t, cfg.Commands, "demo mode must not be able to launch an external process")
}

// TestDemoConfigWritesOutsideUserDirs keeps demo mode from appending to the
// user's real log.
func TestDemoConfigWritesOutsideUserDirs(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := DemoConfig()

	assert.NotEmpty(t, cfg.Log.Path)
	assert.False(t, strings.HasPrefix(cfg.Log.Path, XDGDataDir()),
		"the demo log must not live in the user's data dir")
	assert.False(t, strings.HasPrefix(cfg.Log.Path, XDGCacheDir()),
		"the demo log must not live in the user's cache dir")
}

// TestDemoConfigUsesUnresolvableHosts is defence in depth: even if a request
// escaped the fake adapters it could not reach a real service.
func TestDemoConfigUsesUnresolvableHosts(t *testing.T) {
	cfg := DemoConfig()

	assert.Contains(t, cfg.GitLab.URL, ".invalid")
	assert.Contains(t, cfg.Jira.InstanceURL, ".invalid")
}
