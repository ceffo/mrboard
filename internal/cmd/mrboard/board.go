// Package mrboardcmd wires the cobra CLI and boots the application.
package mrboardcmd

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/adapters/statestore"
	"github.com/ceffo/mrboard/internal/app"
	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/tui"
)

func execBoard(cfgPath, version string) error {
	svc, err := app.New(cfgPath, nil)
	if err != nil {
		return err
	}
	store, err := statestore.New(statestore.Config{Dir: config.XDGDataDir()})
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(tui.New(svc.Config, svc.MRSource, store, version)).Run()
	return err
}
