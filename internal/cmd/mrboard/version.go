package mrboardcmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func buildVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print mrboard version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(Version)
		},
	}
}
