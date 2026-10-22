//go:build !windows

package tui

import (
	"bytes"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runHandoff runs script under handoffCommand on a throwaway terminal and
// returns everything written to that terminal plus the run error.
func runHandoff(t *testing.T, script string) (string, error) {
	t.Helper()
	ptmx, tty, err := pty.Open()
	require.NoError(t, err)
	require.NoError(t, pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80}))
	var captured bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&captured, ptmx)
	}()

	h := &handoffCommand{cmd: exec.Command("sh", "-c", script)}
	h.SetStdin(tty)
	h.SetStdout(tty)
	h.SetStderr(tty)
	runErr := h.Run()

	require.NoError(t, tty.Close())
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	_ = ptmx.Close()
	return captured.String(), runErr
}

func TestHandoffCommand_KeepsAltScreenAcrossChild(t *testing.T) {
	out, err := runHandoff(t, `printf '\033[?1049hCHILD\033[?1049l'`)
	require.NoError(t, err)

	assert.Contains(t, out, "\x1b[?1049h\x1b[2J\x1b[H", "alt screen is re-entered before the child runs")
	assert.Contains(t, out, "CHILD")
	assert.NotContains(t, out, "\x1b[?1049l", "the child's alt-screen exit must not reach the terminal")
}

func TestHandoffCommand_PlainChildOutputShown(t *testing.T) {
	out, err := runHandoff(t, `echo plain-output`)
	require.NoError(t, err)

	assert.Contains(t, out, "plain-output")
}

func TestHandoffCommand_ExitStatus(t *testing.T) {
	_, err := runHandoff(t, `exit 3`)

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 3, exitErr.ExitCode())
}

func TestHandoffCommand_MissingBinary(t *testing.T) {
	ptmx, tty, err := pty.Open()
	require.NoError(t, err)
	defer ptmx.Close()
	h := &handoffCommand{cmd: exec.Command("mrboard-definitely-does-not-exist-xyz")}
	h.SetStdin(tty)
	h.SetStdout(tty)
	h.SetStderr(tty)

	runErr := h.Run()

	var execErr *exec.Error
	assert.ErrorAs(t, runErr, &execErr)
	_ = tty.Close()
}

func TestHandoffCommand_ReceivesKeysAndSize(t *testing.T) {
	ptmx, tty, err := pty.Open()
	require.NoError(t, err)
	require.NoError(t, pty.Setsize(ptmx, &pty.Winsize{Rows: 30, Cols: 100}))
	var captured bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&captured, ptmx)
	}()
	go func() {
		time.Sleep(300 * time.Millisecond)
		_, _ = ptmx.Write([]byte("k\n"))
	}()

	h := &handoffCommand{cmd: exec.Command("sh", "-c", `stty size; read line; echo got:$line`)}
	h.SetStdin(tty)
	h.SetStdout(tty)
	h.SetStderr(tty)
	require.NoError(t, h.Run())
	require.NoError(t, tty.Close())
	<-done

	assert.Contains(t, captured.String(), "30 100")
	assert.Contains(t, captured.String(), "got:k")
}

func TestHandoffCommand_FailureCarriesOutputTail(t *testing.T) {
	_, err := runHandoff(t, `printf '\033[?1049h\033[31m'; echo 'fatal: could not read from remote' >&2; exit 128`)

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 128, exitErr.ExitCode())
	assert.Equal(t, "fatal: could not read from remote", outputTail(err))
}

func TestHandoffCommand_SuccessHasNoTail(t *testing.T) {
	_, err := runHandoff(t, `echo fine`)

	require.NoError(t, err)
	assert.Empty(t, outputTail(err))
}

// The launcher-chain fixture clones, fetches and adds a worktree from a local
// origin behind a /dev/tty prompt, then execs a full-screen child.
func TestHandoffCommand_LauncherChainFixture(t *testing.T) {
	root := t.TempDir()
	ptmx, tty, err := pty.Open()
	require.NoError(t, err)
	require.NoError(t, pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80}))
	var captured bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&captured, ptmx)
	}()
	go func() {
		time.Sleep(1500 * time.Millisecond)
		_, _ = ptmx.Write([]byte("y\n"))
		time.Sleep(1500 * time.Millisecond)
		_, _ = ptmx.Write([]byte("x"))
	}()

	h := &handoffCommand{cmd: exec.Command("../../scripts/testdata/fixture-handoff-chain.sh", root)}
	h.SetStdin(tty)
	h.SetStdout(tty)
	h.SetStderr(tty)
	runErr := h.Run()
	require.NoError(t, tty.Close())
	select {
	case <-done:
	case <-time.After(time.Second):
	}
	_ = ptmx.Close()

	require.NoError(t, runErr)
	assert.Contains(t, captured.String(), "clone it?")
	assert.Contains(t, captured.String(), "mrboard alt-screen fixture")
}
