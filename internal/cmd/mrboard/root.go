// Package mrboardcmd wires the cobra CLI and boots the application.
package mrboardcmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/core"
	ilog "github.com/ceffo/mrboard/internal/log"
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
	var c *core.Core

	root := &cobra.Command{
		Use:   "mrboard",
		Short: "GitLab MR review board for daily standups",
		Long: `mrboard displays GitLab merge requests in a kanban board.

Config search path (first match wins):
  --config flag
  $XDG_CONFIG_HOME/mrboard/mrboard.yaml  (default: ~/.config/mrboard/mrboard.yaml)
  ./mrboard.yaml

Environment:
  GITLAB_TOKEN     Override gitlab.token from config`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			if logLevel != "" {
				cfg.Log.Level = logLevel
			}
			built, err := core.New(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			c = built
			ctx := ilog.WithLogger(cmd.Context(), c.Logger)
			ctx = context.WithValue(ctx, coreKey{}, c)
			cmd.SetContext(ctx)
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return execBoard(cmd.Context(), c, Version)
		},
	}

	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "config file path (default: XDG search)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level override (debug|info|warn|error)")

	root.AddCommand(buildFetchCmd())
	root.AddCommand(buildVersionCmd())

	return root
}
