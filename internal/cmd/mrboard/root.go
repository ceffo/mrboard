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
	var cfgPath string

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
		RunE: func(_ *cobra.Command, _ []string) error {
			return execBoard(cfgPath, Version)
		},
	}

	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "config file path (default: XDG search)")

	root.AddCommand(buildFetchCmd())
	root.AddCommand(buildVersionCmd())

	return root
}
