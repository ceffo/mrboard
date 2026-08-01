package config

import (
	"os"
	"path/filepath"
	"time"
)

// Demo-mode equivalents of the defaults Load would otherwise supply via viper.
// They are named rather than inlined so it is obvious which viper default each
// one stands in for.
const (
	demoTimeout         = 30 * time.Second
	demoWarnAfter       = 72 * time.Hour
	demoErrorAfter      = 120 * time.Hour
	demoRefreshInterval = 60 * time.Second
	demoTicketCacheTTL  = 24 * time.Hour
)

// DemoConfig returns a fully-populated AppConfig for demo mode (--demo), built
// entirely in memory: no file is read, no user directory is consulted.
//
// Every value Load would have supplied through a viper default must be set here
// by hand, since DemoConfig bypasses Load entirely. LifetimeWarnAfter and
// LifetimeErrorAfter matter most: left at zero the board loses its age
// colouring, which is one of the things the demo exists to show.
//
// Hostnames use the reserved .invalid TLD (RFC 2606) so nothing here can
// resolve, even if a request somehow escaped the fake adapters. The log goes to
// the OS temp dir rather than the user's data dir, because demo mode must not
// append to the real log.
//
// Values are chosen to light up the optional features: CurrentUser enables the
// "my view" toggle, Jira.BoardID the sprint filter, Jira.InstanceURL the ticket
// line and its open-in-browser action, and the user source populates the roster
// the reviewer editor's "set team" action reads. Commands is nil so no external
// process is ever launched.
func DemoConfig() *AppConfig {
	return &AppConfig{
		GitLab: GitLab{
			URL:     "https://gitlab.demo.invalid",
			Token:   "demo-token-not-a-real-credential",
			Timeout: demoTimeout,
		},
		Sources: []Source{{
			Type: "user",
			IDs:  []string{"ada", "grace", "linus", "margaret", "katherine"},
		}},
		CurrentUser:        "ada",
		LifetimeWarnAfter:  demoWarnAfter,
		LifetimeErrorAfter: demoErrorAfter,
		RefreshInterval:    demoRefreshInterval,
		Jira: Jira{
			InstanceURL: "https://tickets.demo.invalid",
			Email:       "demo@demo.invalid",
			APIToken:    "demo-token-not-a-real-credential",
			BoardID:     1,
			CacheTTL:    demoTicketCacheTTL,
		},
		Log: logSection{
			Path:  filepath.Join(os.TempDir(), "mrboard-demo.log"),
			Level: "info",
		},
	}
}
