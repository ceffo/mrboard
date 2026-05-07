package tui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
)

type footerWidget struct {
	help   help.Model
	keyMap help.KeyMap
	styles Styles
}

func newFooterWidget(keys KeyMap, styles Styles) footerWidget {
	return footerWidget{help: help.New(), keyMap: keys, styles: styles}
}

// SetKeyMap swaps the active key map; the footer renders the new bindings on the next View.
func (f *footerWidget) SetKeyMap(km help.KeyMap) { f.keyMap = km }

func (f footerWidget) Init() tea.Cmd                         { return nil }
func (f footerWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return f, nil }
func (f footerWidget) View() tea.View                        { return tea.NewView(f.render()) }

func (f footerWidget) render() string {
	return f.styles.Footer.Render(f.help.View(f.keyMap))
}
