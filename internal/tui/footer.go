package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/help"
)

type footerWidget struct {
	help help.Model
	keys KeyMap
}

func newFooterWidget(keys KeyMap) footerWidget {
	return footerWidget{help: help.New(), keys: keys}
}

func (f footerWidget) Init() tea.Cmd                           { return nil }
func (f footerWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return f, nil }
func (f footerWidget) View() tea.View {
	return tea.NewView(f.help.View(f.keys))
}
