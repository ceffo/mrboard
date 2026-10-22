//go:build !windows

package tui

import (
	"os/exec"
	"syscall"
)

// detachProcess puts proc in its own session so a terminal SIGINT or hangup
// aimed at mrboard's process group never reaches it.
func detachProcess(proc *exec.Cmd) {
	proc.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
