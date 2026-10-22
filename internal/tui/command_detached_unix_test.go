//go:build !windows

package tui

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	helperEnv       = "MRBOARD_DETACH_HELPER_MARKER"
	detachedFixture = "../../scripts/testdata/fixture-detached-command.sh"
)

// TestDetachedHelperProcess is not a test: it is the body of the subprocess
// TestDetachedCommand_OutlivesMrboard re-executes. It launches the fixture through
// startDetached, reports readiness, then idles until the parent kills it.
func TestDetachedHelperProcess(t *testing.T) {
	marker := os.Getenv(helperEnv)
	if marker == "" {
		t.Skip("helper process only")
	}
	fixture, err := filepath.Abs(detachedFixture)
	require.NoError(t, err)
	msg := startDetached("fixture", fixture, []string{marker, "0", "1"})()
	_, ok := msg.(CommandStartedMsg)
	require.True(t, ok, "expected CommandStartedMsg, got %T", msg)

	_, _ = os.Stdout.WriteString("ready\n")
	time.Sleep(time.Minute)
}

// TestDetachedCommand_OutlivesMrboard proves a detached command survives its
// launcher being interrupted: the helper starts the fixture and is then sent
// SIGINT through its own process group; the fixture must still write its marker.
func TestDetachedCommand_OutlivesMrboard(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	helper := exec.Command(os.Args[0], "-test.run=^TestDetachedHelperProcess$")
	helper.Env = append(os.Environ(), helperEnv+"="+marker)
	helper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := helper.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, helper.Start())

	ready := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if sc.Text() == "ready" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		require.True(t, ok, "helper exited before reporting ready")
	case <-time.After(15 * time.Second):
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		require.Fail(t, "helper never reported ready")
	}

	_, statErr := os.Stat(marker)
	require.Error(t, statErr, "fixture must still be sleeping when the helper is interrupted")
	require.NoError(t, syscall.Kill(-helper.Process.Pid, syscall.SIGINT))
	_ = helper.Wait()

	assert.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil },
		10*time.Second, 50*time.Millisecond, "the detached fixture must outlive its launcher")
}
