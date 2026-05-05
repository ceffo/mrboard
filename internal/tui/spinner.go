package tui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

type spinnerWidget struct {
	spinner spinner.Model
}

func newSpinnerWidget() spinnerWidget {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return spinnerWidget{spinner: s}
}

func (s spinnerWidget) Init() tea.Cmd {
	return s.spinner.Tick
}

func (s spinnerWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	s.spinner, cmd = s.spinner.Update(msg)
	return s, cmd
}

func (s spinnerWidget) View() tea.View {
	return tea.NewView(s.spinner.View() + " Loading…")
}
