package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewSelfUpdateExecCmd_NeverInvoked asserts the exact command that would
// run without ever invoking it — pressing confirm in a real run shells out to
// brew against the developer's actual Homebrew install, so this must stay a
// pure construction check (docs/adr/0010-self-update-check.md).
func TestNewSelfUpdateExecCmd_NeverInvoked(t *testing.T) {
	execCmd := newSelfUpdateExecCmd()

	require.NotNil(t, execCmd)
	assert.Equal(t, []string{"sh", "-c", "brew update && brew upgrade ceffo/tap/mrboard"}, execCmd.Args)
}

func TestUpdateModalWidget_Update_ConfirmAndCancel(t *testing.T) {
	styles := NewStyles(LoadThemeByName("default"), true)
	w := newUpdateModalWidget("0.11.0", "v0.12.0", styles, DefaultUpdateConfirmKeyMap)

	_, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	_, ok := cmd().(UpdateConfirmedMsg)
	assert.True(t, ok, "expected UpdateConfirmedMsg on confirm")

	_, cmd = w.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	_, ok = cmd().(UpdateCancelledMsg)
	assert.True(t, ok, "expected UpdateCancelledMsg on cancel")
}

func TestUpdateModalWidget_Render_ShowsCurrentAndLatest(t *testing.T) {
	styles := NewStyles(LoadThemeByName("default"), true)
	w := newUpdateModalWidget("0.11.0", "v0.12.0", styles, DefaultUpdateConfirmKeyMap)

	out := w.render()

	assert.Contains(t, out, "0.11.0")
	assert.Contains(t, out, "v0.12.0")
	assert.Contains(t, out, updateCommand)
}

func TestHandleSelfUpdateResult(t *testing.T) {
	m := makeModelWithCommands(t, nil)

	_, successCmd := m.handleSelfUpdateResult(SelfUpdateResultMsg{})
	assert.NotNil(t, successCmd, "success must still toast — the running process is still the old binary")

	_, failCmd := m.handleSelfUpdateResult(SelfUpdateResultMsg{Err: assert.AnError})
	assert.NotNil(t, failCmd, "expected an error toast Cmd")
}
