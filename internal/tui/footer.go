package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"
)

type footerWidget struct {
	help    help.Model
	keyMap  help.KeyMap
	styles  Styles
	version string
	width   int
}

func newFooterWidget(keys KeyMap, styles Styles, version string) footerWidget {
	return footerWidget{help: help.New(), keyMap: keys, styles: styles, version: version}
}

// SetStyles updates the footer's style set.
func (f *footerWidget) SetStyles(s Styles) { f.styles = s }

// SetKeyMap swaps the active key map; the footer renders the new bindings on the next View.
func (f *footerWidget) SetKeyMap(km help.KeyMap) { f.keyMap = km }

// SetWidth updates available width so the version is pinned to the right edge.
func (f *footerWidget) SetWidth(w int) { f.width = w }

func (f footerWidget) Init() tea.Cmd                         { return nil }
func (f footerWidget) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return f, nil }
func (f footerWidget) View() tea.View                        { return tea.NewView(f.render()) }

func (f footerWidget) render() string {
	helpStr := f.help.View(f.keyMap)
	ver := f.styles.FooterVersion.Render(f.version)

	if f.width > 0 {
		pad := f.width - lip.Width(helpStr) - lip.Width(ver)
		if pad < 1 {
			pad = 1
		}
		combined := helpStr + strings.Repeat(" ", pad) + ver
		return f.styles.Footer.Render(combined)
	}
	return f.styles.Footer.Render(helpStr + " " + ver)
}
