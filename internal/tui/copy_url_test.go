package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectMsgs runs cmd and every command nested in the batches it returns.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, collectMsgs(c)...)
	}
	return out
}

func TestModel_CopyKey_ConfirmsWithToast(t *testing.T) {
	m := makeModel(t, someMRs(), "alice")

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})

	var confirmed bool
	for _, msg := range collectMsgs(cmd) {
		if tm, ok := msg.(toastMsg); ok {
			confirmed = true
			assert.Contains(t, tm.text, "copied")
		}
	}
	require.True(t, confirmed, "copying must confirm with a toast")
}
