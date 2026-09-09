package tui

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain/service/updatesvc"
	"github.com/ceffo/mrboard/internal/domain/service/updatesvc/mocks"
)

// testLatestTag is the newer release the fake checks report.
const testLatestTag = "v1.3.0"

// newTestVersionWidget builds a widget with no checker wired — enough for
// render and keybinding assertions. The update action is a copy of the board
// binding so tests never mutate the package-level keymap.
func newTestVersionWidget(version string) *versionWidget {
	action := DefaultBoardKeyMap.Update
	return newVersionWidget(
		context.Background(), NewStyles(LoadThemeByName("default"), true),
		version, nil, 0, &action, slog.Default(),
	)
}

// batchCmds unpacks a tea.Batch into its constituent commands without running
// them: one of them is a tea.Tick that would otherwise block for its full
// interval.
func batchCmds(t *testing.T, cmd tea.Cmd) []tea.Cmd {
	t.Helper()
	require.NotNil(t, cmd)
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok, "want a tea.Batch")
	return batch
}

// runCmds executes each command and returns the messages they produce.
func runCmds(t *testing.T, cmds []tea.Cmd) []tea.Msg {
	t.Helper()
	msgs := make([]tea.Msg, 0, len(cmds))
	for _, c := range cmds {
		msgs = append(msgs, c())
	}
	return msgs
}

func TestVersionWidget_Render_NoUpdate_OmitsBadge(t *testing.T) {
	v := newTestVersionWidget("1.2.3")

	out := v.render()

	assert.Contains(t, out, "1.2.3")
	assert.False(t, strings.Contains(out, "↑"), "no badge expected when no update is available")
}

// TestVersionWidget_Render_UpdateAvailable_ShowsKeyHint pins that the badge
// carries its own key hint: the update action is PriorityModal and so never
// reaches the footer's binding list, leaving this the only place the user can
// discover the shortcut.
func TestVersionWidget_Render_UpdateAvailable_ShowsKeyHint(t *testing.T) {
	v := newTestVersionWidget("1.2.3")
	v.applyCheckResult(updateCheckResultMsg{info: updatesvc.Info{Available: true, Latest: testLatestTag}})

	out := v.render()

	assert.Contains(t, out, "1.2.3")
	assert.Contains(t, out, "↑")
	assert.Contains(t, out, v.action.Help().Key)
	assert.Contains(t, out, v.action.Help().Desc)
}

func TestVersionWidget_ApplyCheckResult_TogglesUpdateKey(t *testing.T) {
	v := newTestVersionWidget("1.2.3")
	require.False(t, v.action.Enabled(), "want the update key disabled before any check")

	v.applyCheckResult(updateCheckResultMsg{info: updatesvc.Info{Available: true, Latest: testLatestTag}})
	assert.True(t, v.action.Enabled())
	assert.Equal(t, testLatestTag, v.latest)

	v.applyCheckResult(updateCheckResultMsg{info: updatesvc.Info{}})
	assert.False(t, v.action.Enabled())
}

// TestVersionWidget_ApplyCheckResult_ErrorKeepsPreviousState covers a failing
// re-check: a network blip must not retract a badge the user is already
// looking at.
func TestVersionWidget_ApplyCheckResult_ErrorKeepsPreviousState(t *testing.T) {
	v := newTestVersionWidget("1.2.3")
	v.applyCheckResult(updateCheckResultMsg{info: updatesvc.Info{Available: true, Latest: testLatestTag}})

	v.applyCheckResult(updateCheckResultMsg{err: errors.New("boom")})

	assert.True(t, v.available)
	assert.True(t, v.action.Enabled())
}

func TestVersionWidget_Init_ForcesLiveCheckAndSchedulesNext(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	checker.EXPECT().
		CheckForUpdate(mock.Anything, "1.2.3", updatesvc.CheckOptions{Force: true}).
		Return(updatesvc.Info{Available: true, Latest: testLatestTag}, nil).
		Once()

	action := DefaultBoardKeyMap.Update
	v := newVersionWidget(
		context.Background(), NewStyles(LoadThemeByName("default"), true),
		"1.2.3", checker, time.Hour, &action, slog.Default(),
	)

	cmds := batchCmds(t, v.Init())

	require.Len(t, cmds, 2, "want a check and a scheduled re-check")
	assert.Equal(t,
		updateCheckResultMsg{info: updatesvc.Info{Available: true, Latest: testLatestTag}},
		cmds[0](),
	)
}

// TestVersionWidget_Tick_ReChecksAndReschedules pins the recurring cadence:
// every tick both performs a check and schedules the following one, so the
// badge keeps up with releases published mid-session.
func TestVersionWidget_Tick_ReChecksAndReschedules(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	checker.EXPECT().
		CheckForUpdate(mock.Anything, "1.2.3", updatesvc.CheckOptions{}).
		Return(updatesvc.Info{}, nil).
		Once()

	action := DefaultBoardKeyMap.Update
	v := newVersionWidget(
		context.Background(), NewStyles(LoadThemeByName("default"), true),
		"1.2.3", checker, time.Hour, &action, slog.Default(),
	)

	_, cmd := v.Update(updateCheckTickMsg{})
	cmds := batchCmds(t, cmd)

	require.Len(t, cmds, 2, "want a check and the next tick")
	assert.Equal(t, updateCheckResultMsg{}, cmds[0]())
}

func TestVersionWidget_DevBuild_NeverChecks(t *testing.T) {
	checker := mocks.NewMockUpdateChecker(t)
	action := DefaultBoardKeyMap.Update
	v := newVersionWidget(
		context.Background(), NewStyles(LoadThemeByName("default"), true),
		devVersion, checker, time.Hour, &action, slog.Default(),
	)

	assert.Nil(t, v.Init(), "a dev build has no release to compare against")
}

func TestVersionWidget_ConfirmDialog_RunsUpdateOnYes(t *testing.T) {
	v := newTestVersionWidget("1.2.3")
	v.applyCheckResult(updateCheckResultMsg{info: updatesvc.Info{Available: true, Latest: testLatestTag}})

	dlg := v.confirmDialog(DefaultConfirmKeyMap)
	body := dlg.render()
	assert.Contains(t, body, "1.2.3")
	assert.Contains(t, body, testLatestTag)
	assert.Contains(t, body, updateCommand)

	_, cmd := dlg.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	msgs := runCmds(t, batchCmds(t, cmd))

	assert.Contains(t, msgs, tea.Msg(dismissOverlayMsg{}))
	assert.Contains(t, msgs, tea.Msg(selfUpdateRequestedMsg{}))
}

func TestConfirmWidget_Cancel_DismissesWithoutRunning(t *testing.T) {
	dlg := newConfirmWidget("t", "b", NewStyles(LoadThemeByName("default"), true), DefaultConfirmKeyMap,
		func() tea.Msg { return selfUpdateRequestedMsg{} })

	_, cmd := dlg.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	require.NotNil(t, cmd)
	assert.Equal(t, dismissOverlayMsg{}, cmd())
}

func TestNewSelfUpdateExecCmd_ShellsOutToBrew(t *testing.T) {
	cmd := newSelfUpdateExecCmd()

	assert.Equal(t, []string{"sh", "-c", updateCommand}, cmd.Args)
}
