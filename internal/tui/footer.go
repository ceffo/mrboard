package tui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
)

type footerWidget struct {
	help   help.Model
	keys   KeyMap
	styles Styles
}

func newFooterWidget(keys KeyMap, styles Styles) footerWidget {
	return footerWidget{help: help.New(), keys: keys, styles: styles}
}

func (f footerWidget) Init() tea.Cmd                         { return nil }
func (f footerWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return f, nil }
func (f footerWidget) View() tea.View                        { return tea.NewView(f.render()) }

func (f footerWidget) render() string {
	return f.styles.Footer.Render(f.help.View(f.keys))
}
