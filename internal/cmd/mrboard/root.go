// Package mrboardcmd wires the cobra CLI and boots the application.
package mrboardcmd

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/core"
	ilog "github.com/ceffo/mrboard/internal/log"
	"github.com/ceffo/mrboard/internal/tui"
)

// Version is set at build time via -ldflags.
var Version = "dev"

type coreKey struct{}

// Execute is the entry point called by cmd/mrboard/main.go.
func Execute(ctx context.Context) error {
	return buildRootCmd().ExecuteContext(ctx)
}

func buildRootCmd() *cobra.Command {
	var cfgPath string
	var logLevel string
	var themeOverride string
	var modeOverride string
	var demoMode bool
	var c *core.Core

	// bootCore loads config and wires up the application. Only commands that
	// actually need a live GitLab/JIRA client call this from their own PreRunE —
	// it must not run as a PersistentPreRunE, since that would also apply it to
	// commands like `version` and cobra's built-in `completion` that must work
	// without any config present (e.g. Homebrew's completion-generation step
	// runs the binary in a sandbox with no config file at all).
	bootCore := func(cmd *cobra.Command) error {
		// Demo mode builds its config in memory and wires the built-in dataset,
		// so it works with no config file and no network at all.
		var cfg *config.AppConfig
		var err error
		if demoMode {
			if cfgPath != "" {
				return errors.New("--demo uses the built-in dataset and cannot be combined with --config")
			}
			cfg = config.DemoConfig()
		} else if cfg, err = config.Load(cfgPath); err != nil {
			return err
		}
		if logLevel != "" {
			cfg.Log.Level = logLevel
		}
		boot := core.New
		if demoMode {
			boot = core.NewDemo
		}
		built, err := boot(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		c = built
		ctx := ilog.WithLogger(cmd.Context(), c.Logger)
		ctx = context.WithValue(ctx, coreKey{}, c)
		cmd.SetContext(ctx)
		c.Logger.Info("mrboard startup", "version", Version, "log_level", cfg.Log.Level, "current_user", cfg.CurrentUser)
		return nil
	}

	root := &cobra.Command{
		Use:   "mrboard",
		Short: "GitLab MR review board for daily standups",
		Long: `mrboard displays GitLab merge requests in a kanban board.

Config search path (first match wins):
  --config flag
  $XDG_CONFIG_HOME/mrboard/mrboard.yaml  (default: ~/.config/mrboard/mrboard.yaml)
  ./mrboard.yaml

Environment:
  GITLAB_TOKEN     Override gitlab.token from config

Run "mrboard --demo" to explore the board against a built-in fake dataset,
with no config file, credentials, or network access required.`,
		SilenceUsage: true,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return bootCore(cmd)
		},
		PersistentPostRunE: func(_ *cobra.Command, _ []string) error {
			if c != nil {
				c.Logger.Info("mrboard shutdown")
				return c.Close(context.Background())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := tui.Options{
				ThemeOverride: themeOverride,
				ModeOverride:  modeOverride,
			}
			return execBoard(cmd.Context(), c, Version, opts)
		},
	}

	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "config file path (default: XDG search)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level override (debug|info|warn|error)")
	root.PersistentFlags().BoolVar(&demoMode, "demo", false,
		"run against the built-in demo dataset — no config file, no token, no network")
	root.Flags().StringVar(&themeOverride, "theme", "", "session theme (default, dracula, nord, tokyo-night, monokai)")
	root.Flags().StringVar(&modeOverride, "mode", "", "colour mode for this session (auto, dark, light)")

	fetchCmd := buildFetchCmd()
	fetchCmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return bootCore(cmd)
	}
	root.AddCommand(fetchCmd)

	updateCmd := buildUpdateCmd()
	updateCmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return bootCore(cmd)
	}
	root.AddCommand(updateCmd)

	root.AddCommand(buildVersionCmd())

	return root
}
