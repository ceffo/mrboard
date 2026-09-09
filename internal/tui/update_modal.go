package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// UpdateConfirmedMsg is sent when the user confirms the self-update
// (docs/adr/0010-self-update-check.md).
type UpdateConfirmedMsg struct{}

// UpdateCancelledMsg is sent when the user dismisses the update-confirmation
// modal without confirming.
type UpdateCancelledMsg struct{}

// updateCommand is the fixed, non-user-templated command the update-confirm
// modal offers to run. Unlike the argv-only custom-command launcher (whose
// args come from admin config and must never pass through a shell), this
// string never varies and is not built from any external input.
const updateCommand = "brew update && brew upgrade ceffo/tap/mrboard"

// updateModalWidget is the confirmation overlay shown when a newer mrboard
// release is available. It is a real Yes/No confirmation — Enter/y runs the
// update, Esc/n dismisses — not a "show the command, press another key to
// run it" flow.
type updateModalWidget struct {
	styles  Styles
	keys    UpdateConfirmKeyMap
	current string
	latest  string
}

func newUpdateModalWidget(current, latest string, styles Styles, keys UpdateConfirmKeyMap) *updateModalWidget {
	return &updateModalWidget{styles: styles, keys: keys, current: current, latest: latest}
}

func (w *updateModalWidget) Init() tea.Cmd { return nil }

func (w *updateModalWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return w, nil
	}
	switch {
	case w.keys.Confirm.Match(kMsg):
		return w, func() tea.Msg { return UpdateConfirmedMsg{} }
	case w.keys.Cancel.Match(kMsg):
		return w, func() tea.Msg { return UpdateCancelledMsg{} }
	}
	return w, nil
}

func (w *updateModalWidget) render() string {
	body := fmt.Sprintf(
		"update available: %s → %s\n\nrun `%s`?\n\n[Enter]=yes  [Esc]=no",
		w.current, w.latest, updateCommand,
	)
	return w.styles.PopupBorder.Render(w.styles.PopupTitle.Render("Update mrboard") + "\n\n" + body)
}

func (w *updateModalWidget) View() tea.View { return tea.NewView(w.render()) }
