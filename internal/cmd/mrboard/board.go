package mrboardcmd

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/core"
	"github.com/ceffo/mrboard/internal/tui"
)

func execBoard(ctx context.Context, c *core.Core, version string, opts tui.Options) error {
	_, err := tea.NewProgram(
		tui.New(ctx, c.Config, c.MRSource, c.StateStore, version, opts),
		tea.WithContext(ctx),
	).Run()
	return err
}
