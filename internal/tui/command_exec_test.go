package tui

import (
	"context"
	"os/exec"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

// makeModelWithCommands builds a board-state Model configured with cmds, per
// docs/adr/0004-external-command-launcher.md.
func makeModelWithCommands(t *testing.T, cmds []config.Command) Model {
	t.Helper()
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(someMRs(), nil).Maybe()
	cfg := &config.Config{Commands: cmds}
	m := New(context.Background(), cfg, src, noopStore{}, noopSnapshotStore{}, nil, nil, nil, "dev", Options{})
	next, _ := m.Update(FetchResultMsg{MRs: someMRs()})
	return next.(Model)
}

func TestMatchCustomCommand(t *testing.T) {
	cmds := []config.Command{
		{Name: "code review", Key: "R", Binary: "tuicr"},
		{Name: "hunk view", Key: "H", Binary: "hunk-view-bin"},
	}
	m := makeModelWithCommands(t, cmds)

	got, ok := m.matchCustomCommand(tea.KeyPressMsg{Text: "H", Code: 'H'})
	require.True(t, ok, "expected the configured 'H' key to match")
	assert.Equal(t, "hunk view", got.Name)

	_, ok = m.matchCustomCommand(tea.KeyPressMsg{Text: "z", Code: 'z'})
	assert.False(t, ok, "an unconfigured key must not match")
}

// TestHandleKeyBoard_CustomCommand_ShadowsBoardDefault confirms the ADR's
// stacking rule end-to-end: a configured command bound to "r" (Board's
// Refresh key) takes priority, so pressing "r" dispatches the exec Cmd
// instead of triggering a refresh.
func TestHandleKeyBoard_CustomCommand_ShadowsBoardDefault(t *testing.T) {
	cmds := []config.Command{
		{Name: "refresh via tool", Key: "r", Binary: "true", Args: []string{"{{.ProjectPath}}"}},
	}
	m := makeModelWithCommands(t, cmds)
	require.NotNil(t, m.board.FocusedMR())

	result, cmd := m.handleKeyBoard(tea.KeyPressMsg{Text: "r", Code: 'r'})

	assert.NotNil(t, cmd, "expected the custom command's exec Cmd")
	assert.False(t, result.(Model).isRefreshing, "board's default refresh must be shadowed")
}

func TestExecCommandCmd_ArgvResolutionFailure(t *testing.T) {
	m := makeModelWithCommands(t, nil)
	mr := someMRs()[0]
	cmd := config.Command{Name: argvTestCommandName, Binary: "true", Args: []string{"{{.NotAField}}"}}

	execCmd := m.execCommandCmd(mr, cmd)
	require.NotNil(t, execCmd)

	msg := execCmd()
	result, ok := msg.(CommandResultMsg)
	require.True(t, ok, "expected CommandResultMsg, got %T", msg)
	assert.Equal(t, argvTestCommandName, result.CommandName)
	assert.Error(t, result.Err)
}

// TestHandleCommandResult covers the three outcomes tea.ExecProcess's callback
// uniformly reports (docs/adr/0004-external-command-launcher.md, "UX for a
// failing or missing configured command"): success shows no toast; a missing
// binary (*exec.Error) and a non-zero exit (*exec.ExitError) both surface the
// same error toast.
func TestHandleCommandResult(t *testing.T) {
	missingBinaryErr := exec.Command("mrboard-definitely-does-not-exist-xyz").Run()
	require.Error(t, missingBinaryErr)
	var execErr *exec.Error
	require.ErrorAs(t, missingBinaryErr, &execErr)

	nonZeroExitErr := exec.Command("sh", "-c", "exit 1").Run()
	require.Error(t, nonZeroExitErr)
	var exitErr *exec.ExitError
	require.ErrorAs(t, nonZeroExitErr, &exitErr)

	tests := []struct {
		name      string
		err       error
		wantToast bool
	}{
		{name: "success", err: nil, wantToast: false},
		{name: "missing binary", err: missingBinaryErr, wantToast: true},
		{name: "non-zero exit", err: nonZeroExitErr, wantToast: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := makeModelWithCommands(t, nil)
			_, cmd := m.handleCommandResult(CommandResultMsg{CommandName: argvTestCommandName, Err: tt.err})
			if tt.wantToast {
				assert.NotNil(t, cmd, "expected an error toast Cmd")
			} else {
				assert.Nil(t, cmd, "success must show no toast")
			}
		})
	}
}
