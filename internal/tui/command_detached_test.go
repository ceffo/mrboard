package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
)

func TestStartDetached(t *testing.T) {
	tests := []struct {
		name       string
		binary     string
		argv       []string
		wantStart  bool
		wantErr    bool
		wantStderr string
	}{
		{name: "exit 0", binary: "sh", argv: []string{"-c", "echo out; echo err >&2"}, wantStart: true},
		{
			name: "non-zero exit keeps stderr tail", binary: "sh",
			argv:      []string{"-c", "echo one >&2; echo two >&2; echo three >&2; echo four >&2; exit 3"},
			wantStart: true, wantErr: true, wantStderr: "two | three | four",
		},
		{name: "missing binary", binary: "mrboard-definitely-does-not-exist-xyz", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := startDetached("job", tt.binary, tt.argv)()

			result := msg
			if tt.wantStart {
				started, ok := msg.(CommandStartedMsg)
				require.True(t, ok, "expected CommandStartedMsg, got %T", msg)
				assert.Equal(t, "job", started.CommandName)
				result = started.wait()
			}
			res, ok := result.(CommandResultMsg)
			require.True(t, ok, "expected CommandResultMsg, got %T", result)
			assert.Equal(t, "job", res.CommandName)
			assert.Equal(t, tt.wantErr, res.Err != nil)
			assert.Equal(t, tt.wantStderr, res.Stderr)
		})
	}
}

func TestStderrTailBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "err")
	big := make([]byte, 10*stderrTailMaxBytes)
	for i := range big {
		big[i] = 'x'
	}
	require.NoError(t, os.WriteFile(path, append(big, []byte("\nlast\n")...), 0o600))

	tail := stderrTail(path)

	assert.LessOrEqual(t, len([]rune(tail)), stderrTailMaxLines*(stderrTailMaxLineRunes+4))
	assert.Contains(t, tail, "last")
}

func TestDetachedCommandCmd_SurvivesAndWritesMarker(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	m := makeModelWithCommands(t, nil)
	cmd := config.Command{
		Name: "marker", Binary: "sh", Detach: true,
		Args: []string{"-c", "sleep 0.2; echo {{.IID}} > " + marker},
	}

	msg := m.detachedCommandCmd(someMRs()[0], cmd)()
	started, ok := msg.(CommandStartedMsg)
	require.True(t, ok, "expected CommandStartedMsg, got %T", msg)
	_, statErr := os.Stat(marker)
	assert.Error(t, statErr, "the command must not have finished at launch time")

	res := started.wait().(CommandResultMsg)
	assert.NoError(t, res.Err)
	assert.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil },
		2*time.Second, 20*time.Millisecond)
}

func TestHandleCommandStarted_ToastsAndWaits(t *testing.T) {
	m := makeModelWithCommands(t, nil)

	_, cmd := m.handleCommandStarted(CommandStartedMsg{CommandName: "job", wait: func() tea.Msg { return nil }})

	assert.NotNil(t, cmd, "expected a batch of the launched toast and the wait Cmd")
}

func TestHandleKeyBoard_DetachedCommand_DoesNotSuspend(t *testing.T) {
	cmds := []config.Command{{Name: "bg", Key: "R", Binary: "sh", Args: []string{"-c", "exit 0"}, Detach: true}}
	m := makeModelWithCommands(t, cmds)

	_, cmd := m.handleKeyBoard(tea.KeyPressMsg{Text: "R", Code: 'R'})

	require.NotNil(t, cmd)
	msg := cmd()
	_, started := msg.(CommandStartedMsg)
	assert.True(t, started, "a detached command yields CommandStartedMsg, not an ExecProcess message; got %T", msg)
}
