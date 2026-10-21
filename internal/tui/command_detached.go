package tui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
)

const (
	// stderrTailMaxBytes bounds how much of a detached command's stderr is read back.
	stderrTailMaxBytes = 2048
	// stderrTailMaxLines is how many trailing stderr lines reach the error toast.
	stderrTailMaxLines = 3
	// stderrTailMaxLineRunes caps each tail line so one long line cannot flood a toast.
	stderrTailMaxLineRunes = 160
)

// CommandStartedMsg reports that a detached command's process started
// (docs/adr/0004-external-command-launcher.md, "Detached commands"). It carries
// the wait Cmd so the root handler can run it off the UI goroutine.
type CommandStartedMsg struct {
	CommandName string
	wait        tea.Cmd
}

// detachedCommandCmd resolves cmd's argv against mr and returns a Cmd that
// starts the process in its own session without suspending the board. A
// resolution or start failure is reported as a CommandResultMsg; a successful
// start yields a CommandStartedMsg whose wait Cmd later yields the exit outcome
// as a CommandResultMsg.
func (m Model) detachedCommandCmd(mr domain.MergeRequest, cmd config.Command) tea.Cmd {
	argv, err := BuildCommandArgv(mr, cmd)
	if err != nil {
		m.logger.Debug("tui: detached command argv resolution failed",
			"command", cmd.Name, "binary", cmd.Binary, "err", err)
		return func() tea.Msg { return CommandResultMsg{CommandName: cmd.Name, Err: err} }
	}
	m.logger.Debug("tui: starting detached command",
		"command", cmd.Name, "binary", cmd.Binary, "args", argv, "mr_iid", mr.IID)
	return startDetached(cmd.Name, cmd.Binary, argv)
}

// startDetached starts binary with argv: stdin and stdout are the null device,
// stderr goes to a temp file (not a pipe, so the child survives mrboard exiting
// instead of taking SIGPIPE) and is read back only on a failing exit. The
// process is never tied to mrboard's lifetime or terminal.
func startDetached(name, binary string, argv []string) tea.Cmd {
	return func() tea.Msg {
		//nolint:gosec // G204/G702: binary is an admin-configured mrboard.yaml entry; no shell is involved.
		proc := exec.Command(binary, argv...)
		detachProcess(proc)

		errFile, err := os.CreateTemp("", "mrboard-detached-*.stderr")
		if err != nil {
			return CommandResultMsg{CommandName: name, Err: fmt.Errorf("capture stderr: %w", err)}
		}
		proc.Stderr = errFile

		if err := proc.Start(); err != nil {
			_ = errFile.Close()
			_ = os.Remove(errFile.Name())
			return CommandResultMsg{CommandName: name, Err: err}
		}
		// The child holds its own descriptor; ours is only needed to read the tail later.
		_ = errFile.Close()

		return CommandStartedMsg{CommandName: name, wait: func() tea.Msg {
			waitErr := proc.Wait()
			tail := ""
			if waitErr != nil {
				tail = stderrTail(errFile.Name())
			}
			_ = os.Remove(errFile.Name())
			return CommandResultMsg{CommandName: name, Err: waitErr, Stderr: tail}
		}}
	}
}

// stderrTail returns the last few non-empty lines of the file, reading at most
// stderrTailMaxBytes from its end. Read problems yield "" — the tail is a hint.
func stderrTail(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	if info.Size() > stderrTailMaxBytes {
		if _, err := f.Seek(-stderrTailMaxBytes, io.SeekEnd); err != nil {
			return ""
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, stderrTailMaxBytes))
	if err != nil {
		return ""
	}
	return lastLines(data, stderrTailMaxLines)
}

// lastLines joins the final n non-empty lines of data with " | ".
func lastLines(data []byte, n int) string {
	var lines []string
	for _, l := range bytes.Split(data, []byte("\n")) {
		if s := strings.TrimSpace(string(l)); s != "" {
			if r := []rune(s); len(r) > stderrTailMaxLineRunes {
				s = string(r[:stderrTailMaxLineRunes]) + "…"
			}
			lines = append(lines, s)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
