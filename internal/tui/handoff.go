package tui

import (
	"io"
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"
)

// enterAltScreen switches to the alternate screen and clears it. Writing it
// again while the alternate screen is already active is a no-op on
// xterm-compatible terminals, so it is safe on every path.
const enterAltScreen = "\x1b[?1049h\x1b[2J\x1b[H"

// handoffCommand runs a foreground child for tea.Exec without exposing the
// primary screen. bubbletea leaves the alternate screen before Run is called;
// Run re-enters it before the child starts, and (where a PTY is available) keeps
// the child's own alternate-screen exit from reaching the terminal, so the
// terminal stays on the alternate screen until bubbletea repaints the board.
type handoffCommand struct {
	cmd    *exec.Cmd
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// execForeground returns the tea.Cmd that suspends the board, runs cmd in the
// foreground and reports its outcome through fn.
func execForeground(cmd *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
	return tea.Exec(&handoffCommand{cmd: cmd}, fn)
}

func (h *handoffCommand) SetStdin(r io.Reader)  { h.stdin = r }
func (h *handoffCommand) SetStdout(w io.Writer) { h.stdout = w }
func (h *handoffCommand) SetStderr(w io.Writer) { h.stderr = w }

// Run enters the alternate screen, then runs the child on a PTY when stdin and
// stdout are both terminals, or directly on the terminal otherwise.
func (h *handoffCommand) Run() error {
	if h.stdout != nil {
		_, _ = io.WriteString(h.stdout, enterAltScreen)
	}
	in, inOK := h.stdin.(*os.File)
	out, outOK := h.stdout.(*os.File)
	if inOK && outOK {
		if ran, err := runOnPTY(h.cmd, in, out); ran {
			return err
		}
	}
	h.attachDirect()
	return h.cmd.Run()
}

// attachDirect wires the child to the terminal bubbletea handed over, without
// overriding any stream the caller already set.
func (h *handoffCommand) attachDirect() {
	if h.cmd.Stdin == nil {
		h.cmd.Stdin = h.stdin
	}
	if h.cmd.Stdout == nil {
		h.cmd.Stdout = h.stdout
	}
	if h.cmd.Stderr == nil {
		h.cmd.Stderr = h.stderr
	}
}
