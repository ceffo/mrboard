//go:build windows

package tui

import (
	"os/exec"
	"syscall"
)

// detachProcess gives proc its own process group so a console Ctrl-C aimed at
// mrboard does not reach it.
func detachProcess(proc *exec.Cmd) {
	proc.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
