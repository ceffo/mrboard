package mrboardcmd

import (
	"context"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/core"
	"github.com/ceffo/mrboard/internal/tui"
)

// execBoard runs the TUI to completion. A successful self-update quits with a
// message to print (tui.Model.ExitMessage) rather than a toast, since the
// terminal must be restored first — the running process is still the old
// binary until mrboard is relaunched (docs/adr/0010-self-update-check.md).
func execBoard(ctx context.Context, c *core.Core, version string, opts tui.Options, out io.Writer) error {
	final, err := tea.NewProgram(
		tui.New(ctx, c.Config, c.MRSource, c.StateStore, c.SnapshotStore,
			c.Notifier, c.TicketEnricher, c.TicketLinker, c.UpdateChecker, version, opts).
			WithApproverAnnouncements(c.ApproverClaims, c.Config.Notifications.Teams.NewMRWindow),
		tea.WithContext(ctx),
	).Run()
	if err != nil {
		return err
	}
	if m, ok := final.(tui.Model); ok {
		if msg := m.ExitMessage(); msg != "" {
			fmt.Fprintln(out, msg)
		}
	}
	return nil
}
