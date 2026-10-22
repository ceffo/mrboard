package tui

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"

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
		_, _ = io.WriteString(h.stdout, enterAltScreen) //nolint:errcheck // best effort: a dead terminal fails the run anyway
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

// outputError is a foreground child's failure plus the tail of what it printed.
// On a PTY the child's stderr is mixed into the terminal stream and the board
// redraw wipes it, so the tail is the only record of why the child failed.
type outputError struct {
	err  error
	tail string
}

func (e *outputError) Error() string { return e.err.Error() }
func (e *outputError) Unwrap() error { return e.err }

// outputTail returns the output tail carried by err, or "".
func outputTail(err error) string {
	var oe *outputError
	if errors.As(err, &oe) {
		return oe.tail
	}
	return ""
}

// escapeRe matches CSI and OSC terminal escape sequences.
var escapeRe = regexp.MustCompile(
	`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][A-Za-z0-9]|\x1b[=>78]`,
)

// tailKeepFactor is how many times stderrTailMaxBytes a tailBuffer retains, so
// escape-heavy output still leaves a few readable lines.
const tailKeepFactor = 4

// stripEscapes removes terminal escape sequences and carriage returns.
func stripEscapes(b []byte) []byte {
	return bytes.ReplaceAll(escapeRe.ReplaceAll(b, nil), []byte("\r"), nil)
}

// tailBuffer keeps the last stderrTailMaxBytes*4 bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if limit := stderrTailMaxBytes * tailKeepFactor; len(t.buf) > limit {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-limit:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.buf...)
}
