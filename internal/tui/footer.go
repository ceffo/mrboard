package tui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
)

type footerWidget struct {
	help help.Model
	keys KeyMap
}

func newFooterWidget(keys KeyMap) footerWidget {
	return footerWidget{help: help.New(), keys: keys}
}

func (f footerWidget) Init() tea.Cmd                         { return nil }
func (f footerWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return f, nil }
func (f footerWidget) View() tea.View {
	return tea.NewView(f.help.View(f.keys))
}
