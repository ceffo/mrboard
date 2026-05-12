// Package mrboardcmd wires the cobra CLI and boots the application.
package mrboardcmd

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/app"
	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/tui"
)

func execBoard(version string) error {
	svc, err := app.New(loadTimeout(), nil)
	if err != nil {
		return err
	}
	st := config.LoadState()
	m := tui.New(svc.Config, svc.MRSource, st, version)
	_, err = tea.NewProgram(m).Run()
	return err
}
