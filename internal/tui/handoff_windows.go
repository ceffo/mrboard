//go:build windows

package tui

import (
	"os"
	"os/exec"
)

// runOnPTY is unavailable on Windows: the child runs directly on the terminal.
func runOnPTY(_ *exec.Cmd, _, _ *os.File) (bool, error) { return false, nil }
