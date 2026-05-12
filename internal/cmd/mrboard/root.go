package mrboardcmd

import (
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Execute is the entry point called by cmd/mrboard/main.go.
func Execute() {
	if err := buildRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func buildRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "mrboard",
		Short: "GitLab MR review board for daily standups",
		Long: `mrboard displays GitLab merge requests in a kanban board.

Config search path (first match wins):
  $MRBOARD_CONFIG
  $XDG_CONFIG_HOME/mrboard/mrboard.yaml  (default: ~/.config/mrboard/mrboard.yaml)
  ./mrboard.yaml

Environment:
  MRBOARD_CONFIG   Explicit config file path
  GITLAB_TOKEN     Override gitlab.token from config
  MRBOARD_TIMEOUT  HTTP timeout (default: 30s, e.g. "60s")
  MRBOARD_DEBUG    Write debug logs to this file path`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return execBoard(Version)
		},
	}

	root.AddCommand(buildFetchCmd())
	root.AddCommand(buildVersionCmd())

	return root
}
