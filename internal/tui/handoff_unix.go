//go:build !windows

package tui

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
	"github.com/muesli/cancelreader"
)

// copyBufSize is the read size for the child output copy.
const copyBufSize = 32 * 1024

// ptyDrainGrace bounds how long Run waits for the child's last output once the
// child has exited. A descendant that keeps the PTY open must not hold the
// board hostage.
const ptyDrainGrace = 250 * time.Millisecond

// runOnPTY runs cmd on a PTY sized like out, forwarding keystrokes from in and
// the child's output to out with its alternate-screen exits removed. ran is
// false when no PTY could be set up and nothing was started, so the caller can
// fall back to running the child directly.
func runOnPTY(cmd *exec.Cmd, in, out *os.File) (ran bool, err error) {
	if !term.IsTerminal(in.Fd()) || !term.IsTerminal(out.Fd()) {
		return false, nil
	}
	size, err := pty.GetsizeFull(out)
	if err != nil {
		return false, nil //nolint:nilerr // no usable terminal size: run the child directly
	}
	ptmx, tty, err := pty.Open()
	if err != nil {
		return false, nil //nolint:nilerr // no PTY available: run the child directly
	}
	if err := pty.Setsize(ptmx, size); err != nil {
		_ = ptmx.Close()
		_ = tty.Close()
		return false, nil //nolint:nilerr // as above
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	startErr := cmd.Start()
	_ = tty.Close()
	if startErr != nil {
		_ = ptmx.Close()
		return true, startErr
	}
	defer ptmx.Close()

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	go func() {
		for range winch {
			_ = pty.InheritSize(out, ptmx) //nolint:errcheck // a failed resize leaves the old size
		}
	}()

	if state, rawErr := term.MakeRaw(in.Fd()); rawErr == nil {
		defer term.Restore(in.Fd(), state) //nolint:errcheck
	}

	var reader cancelreader.CancelReader
	inputDone := make(chan struct{})
	if r, rErr := cancelreader.NewReader(in); rErr == nil {
		reader = r
		go func() {
			defer close(inputDone)
			_, _ = io.Copy(ptmx, reader) //nolint:errcheck // ends when the child exits or input is cancelled
		}()
	} else {
		close(inputDone)
	}

	tail := &tailBuffer{}
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		copyFiltered(io.MultiWriter(out, tail), ptmx)
	}()

	waitErr := cmd.Wait()

	select {
	case <-outputDone:
	case <-time.After(ptyDrainGrace):
	}
	if reader != nil {
		reader.Cancel()
		<-inputDone
		_ = reader.Close()
	}
	if waitErr != nil {
		waitErr = &outputError{err: waitErr, tail: lastLines(stripEscapes(tail.bytes()), stderrTailMaxLines)}
	}
	return true, waitErr
}

// copyFiltered streams src to dst through the alternate-screen filter until src
// ends (the child closed the PTY).
func copyFiltered(dst io.Writer, src io.Reader) {
	var f altScreenFilter
	buf := make([]byte, copyBufSize)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if b := f.filter(buf[:n]); len(b) > 0 {
				_, _ = dst.Write(b) //nolint:errcheck // a dead terminal ends the child's output read too
			}
		}
		if err != nil {
			if b := f.flush(); len(b) > 0 {
				_, _ = dst.Write(b) //nolint:errcheck // as above
			}
			return
		}
	}
}
