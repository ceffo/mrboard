package tui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/adapters/demoadpt"
	"github.com/ceffo/mrboard/internal/config"
)

// newDetachedHarness boots the TUI on the demo adapter with cmds configured,
// past its first board fetch.
func newDetachedHarness(t *testing.T, cmds []config.Command) *loadHarness {
	t.Helper()
	cfg := config.DemoConfig()
	cfg.Commands = cmds
	a, err := demoadpt.New(demoadpt.Config{Now: time.Now(), Fixture: "load", BaseURL: cfg.GitLab.URL, Latency: -1})
	require.NoError(t, err)
	cfg.CurrentUser = a.Settings().CurrentUser
	cfg.Sources = []config.Source{{Type: "user", IDs: a.Settings().Team}}
	h := &loadHarness{
		src: &recordingSource{MergeRequestSource: a.MRSource()}, store: &recordingStore{},
		demo: a, merged: a.MergedSnapshotStore(),
	}
	h.m = New(context.Background(), cfg, h.src, h.store, a.SnapshotStore(), nil,
		a.Tickets(), nil, nil, "dev", Options{}).WithMergedSnapshots(h.merged)
	h.send(tea.WindowSizeMsg{Width: 160, Height: 30})
	h.send(makeFetchCmd(context.Background(), h.src, false, nil)())
	require.NotNil(t, h.m.board.FocusedMR(), "demo board must focus an MR")
	return h
}

// deliver runs the Cmd that startDetached-based keypresses return and feeds
// each resulting message back through Update, returning the wait Cmd's outcome
// (if any) once delivered.
func (h *loadHarness) deliver(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	require.NotNil(t, cmd)
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			h.deliverAsync(c)
		}
		return
	}
	h.deliverMsg(t, msg)
}

func (h *loadHarness) deliverAsync(c tea.Cmd) {
	if c == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- c() }()
	select {
	case msg := <-done:
		if msg != nil {
			h.send(msg)
		}
	case <-time.After(5 * time.Second):
	}
}

// viewText is the full View as plain text, toasts included (screen() omits them).
func viewText(h *loadHarness) string { return ansi.Strip(h.m.View().Content) }

func (h *loadHarness) deliverMsg(t *testing.T, msg tea.Msg) {
	t.Helper()
	if msg != nil {
		h.send(msg)
	}
}

func TestDetachedCommand_Model_KeepsBoardAndToastsLaunched(t *testing.T) {
	dir := t.TempDir()
	fixture, err := filepath.Abs(detachedFixture)
	require.NoError(t, err)
	h := newDetachedHarness(t, []config.Command{{
		Name: "bgjob", Key: "R", Binary: fixture, Detach: true,
		Args: []string{filepath.Join(dir, "marker"), "0", "0.2"},
	}})

	cmd := h.press("R")
	require.NotNil(t, cmd)
	started, ok := cmd().(CommandStartedMsg)
	require.True(t, ok, "detached key must start in place, not via tea.ExecProcess")
	h.deliver(t, h.send(started))

	after := viewText(h)
	assert.Contains(t, after, "bgjob launched", "launched toast must be visible")
	assert.Contains(t, after, h.m.board.FocusedMR().Title[:10], "the board must still be on screen")
}

func TestDetachedCommand_Model_FailureShowsStderrTail(t *testing.T) {
	dir := t.TempDir()
	fixture, err := filepath.Abs(detachedFixture)
	require.NoError(t, err)
	h := newDetachedHarness(t, []config.Command{{
		Name: "badjob", Key: "R", Binary: fixture, Detach: true,
		Args: []string{filepath.Join(dir, "marker"), "1", "0.1"},
	}})

	started, ok := h.press("R")().(CommandStartedMsg)
	require.True(t, ok)
	h.deliver(t, h.send(started.wait()))

	assert.Contains(t, viewText(h), "badjob failed")
	assert.Contains(t, viewText(h), "failing on purpose", "the toast wraps, so match the stderr tail loosely")
}
