package tui

import (
	tea "charm.land/bubbletea/v2"
)

// dismissOverlayMsg asks the root model to close the active exclusive
// overlay. An overlay widget emits it instead of reaching into the router,
// so it never needs a reference back to the model that opened it.
type dismissOverlayMsg struct{}

func dismissOverlayCmd() tea.Msg { return dismissOverlayMsg{} }

// confirmWidget is a yes/no dialog: one keypress decides the outcome, with no
// intermediate state. Confirming emits the caller's message alongside the
// dismissal, so the caller decides what confirmation means without this
// widget knowing anything about it.
type confirmWidget struct {
	styles    Styles
	keys      ConfirmKeyMap
	title     string
	body      string
	onConfirm func() tea.Msg
}

func newConfirmWidget(title, body string, styles Styles, keys ConfirmKeyMap, onConfirm func() tea.Msg) *confirmWidget {
	return &confirmWidget{styles: styles, keys: keys, title: title, body: body, onConfirm: onConfirm}
}

func (w *confirmWidget) Init() tea.Cmd { return nil }

func (w *confirmWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return w, nil
	}
	switch {
	case w.keys.Confirm.Match(kMsg):
		return w, tea.Batch(dismissOverlayCmd, w.onConfirm)
	case w.keys.Cancel.Match(kMsg):
		return w, dismissOverlayCmd
	}
	return w, nil
}

func (w *confirmWidget) render() string {
	hint := w.styles.PopupHint.Render(
		w.keys.Confirm.Help().Key + " " + w.keys.Confirm.Help().Desc + "   " +
			w.keys.Cancel.Help().Key + " " + w.keys.Cancel.Help().Desc,
	)
	return w.styles.PopupBorder.Render(
		w.styles.PopupTitle.Render(w.title) + "\n\n" + w.body + "\n\n" + hint,
	)
}

func (w *confirmWidget) View() tea.View { return tea.NewView(w.render()) }
