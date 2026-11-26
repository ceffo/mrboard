package tui

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

// refreshTarget is someMRs' second MR, the one the foreground command runs on.
var refreshTarget = domain.MRKey{ProjectID: 0, IID: 20}

// newRefreshModel lands someMRs on a board whose source has no FetchAll
// expectation: any full refetch fails the test, so only the FetchMR calls a
// test expects may reach the source.
func newRefreshModel(t *testing.T) (Model, *mocks.MockMergeRequestSource) {
	t.Helper()
	src := mocks.NewMockMergeRequestSource(t)
	m := New(context.Background(), &config.Config{}, src, noopStore{}, noopSnapshotStore{},
		nil, nil, nil, nil, "dev", Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	next, _ = next.(Model).Update(FetchResultMsg{MRs: someMRs()})
	m = next.(Model)
	require.NotNil(t, m.board.FocusedMR(), "the board must focus an MR")
	return m, src
}

func refreshedTarget() domain.MergeRequest {
	mr := someMRs()[1]
	mr.Title = "retitled by the command"
	mr.Reviewers = []domain.ReviewerInfo{{Username: editorTestApprover, State: domain.ReviewerApproved}}
	return mr
}

func mrByKey(m Model, key domain.MRKey) (domain.MergeRequest, bool) {
	i := slices.IndexFunc(m.allMRs, func(mr domain.MergeRequest) bool { return mr.Key() == key })
	if i < 0 {
		return domain.MergeRequest{}, false
	}
	return m.allMRs[i], true
}

// deliverAll runs cmd, and every Cmd of a batch it yields, feeding each
// resulting message back through Update. Follow-up Cmds are returned batched,
// not run, so a toast's dismiss timer cannot stall the test.
func deliverAll(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	var follow []tea.Cmd
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- c() }()
		var msg tea.Msg
		select {
		case msg = <-done:
		case <-time.After(300 * time.Millisecond):
			return
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, child := range batch {
				run(child)
			}
			return
		}
		if msg != nil {
			var next tea.Cmd
			m, next = step(t, m, msg)
			follow = append(follow, next)
		}
	}
	run(cmd)
	return m, tea.Batch(follow...)
}

func screenText(m Model) string { return ansi.Strip(m.View().Content) }

// step feeds msg through Update and returns the next model and its Cmd.
func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func TestForegroundResult_NamesTheMRItRanOn(t *testing.T) {
	key := domain.MRKey{ProjectID: 7, IID: 42}
	cb := foregroundResult(argvTestCommandName, key)

	ok, isResult := cb(nil).(CommandResultMsg)
	require.True(t, isResult)
	assert.Equal(t, key, ok.MR)
	assert.NoError(t, ok.Err)

	exitErr := exec.Command("false").Run()
	require.Error(t, exitErr)
	failed, isResult := cb(exitErr).(CommandResultMsg)
	require.True(t, isResult)
	assert.Equal(t, key, failed.MR, "a non-zero exit refreshes too")
	assert.Equal(t, exitErr, failed.Err)
}

func TestCommandResult_ExitZero_RefreshesOnlyThatMR(t *testing.T) {
	m, src := newRefreshModel(t)
	focused := m.board.FocusedMR().Key()
	filter := m.filter
	src.EXPECT().FetchMR(mock.Anything, int64(refreshTarget.ProjectID), int64(refreshTarget.IID)).
		Return(refreshedTarget(), nil).Once()

	m, cmd := step(t, m, CommandResultMsg{CommandName: argvTestCommandName, MR: refreshTarget})
	require.NotNil(t, cmd, "a foreground command's exit must start a refresh")
	assert.False(t, m.isRefreshing, "the refresh shows no spinner")
	msg := findMsg[commandRefreshResultMsg](cmd)
	require.NotNil(t, msg)
	m, follow := step(t, m, *msg)

	assert.Nil(t, follow, "landing the refresh starts no further fetch")
	got, ok := mrByKey(m, refreshTarget)
	require.True(t, ok)
	assert.Equal(t, "retitled by the command", got.Title)
	assert.Equal(t, domain.ReviewerApproved, got.Reviewers[0].State)
	assert.Contains(t, ansi.Strip(m.renderContent()), "retitled by the command", "the card shows the new state")
	other, ok := mrByKey(m, someMRs()[0].Key())
	require.True(t, ok)
	assert.Equal(t, someMRs()[0].Title, other.Title, "the other MR is untouched")
	assert.Equal(t, focused, m.board.FocusedMR().Key(), "focus survives the refresh")
	assert.Equal(t, filter, m.filter, "filters survive the refresh")
	assert.Len(t, m.allMRs, len(someMRs()))
}

func TestCommandResult_NonZeroExit_ToastsAndRefreshes(t *testing.T) {
	m, src := newRefreshModel(t)
	src.EXPECT().FetchMR(mock.Anything, int64(refreshTarget.ProjectID), int64(refreshTarget.IID)).
		Return(refreshedTarget(), nil).Once()

	m, cmd := step(t, m, CommandResultMsg{
		CommandName: argvTestCommandName, Err: errors.New("exit status 3"), MR: refreshTarget,
	})
	m, _ = deliverAll(t, m, cmd)

	assert.Contains(t, screenText(m), "review failed: exit status 3", "the failure toast is still shown")
	got, ok := mrByKey(m, refreshTarget)
	require.True(t, ok)
	assert.Equal(t, "retitled by the command", got.Title, "a failed command refreshes its MR too")
}

func TestCommandRefresh_FetchError_ToastsAndKeepsCard(t *testing.T) {
	m, src := newRefreshModel(t)
	src.EXPECT().FetchMR(mock.Anything, int64(refreshTarget.ProjectID), int64(refreshTarget.IID)).
		Return(domain.MergeRequest{}, errors.New("gitlab down")).Once()

	m, cmd := step(t, m, CommandResultMsg{CommandName: argvTestCommandName, MR: refreshTarget})
	m, cmd = deliverAll(t, m, cmd)
	m, _ = deliverAll(t, m, cmd)

	assert.Contains(t, screenText(m), "Refresh !20 failed: gitlab down", "a refresh failure is toasted")
	got, ok := mrByKey(m, refreshTarget)
	require.True(t, ok)
	assert.Equal(t, someMRs()[1], got, "the old card stays in place")
	assert.NotContains(t, m.dirty, refreshTarget)
}

func TestCommandResult_NoMR_DoesNotRefresh(t *testing.T) {
	m, _ := newRefreshModel(t)

	m, cmd := step(t, m, CommandResultMsg{CommandName: "watcher"})
	assert.Nil(t, cmd, "a detached command's success neither toasts nor refreshes")

	m, cmd = step(t, m, CommandResultMsg{CommandName: "watcher", Err: errors.New("exit status 1")})
	assert.Nil(t, findMsg[commandRefreshResultMsg](cmd), "a detached command's failure does not refresh")
	m, _ = deliverAll(t, m, cmd)
	assert.Contains(t, screenText(m), "watcher failed")
}

func TestCommandRefresh_OlderThanLocalWrite_IsDropped(t *testing.T) {
	m, _ := newRefreshModel(t)
	started := time.Now()
	m.dirty.Mark(refreshTarget, started.Add(time.Second))

	m, _ = step(t, m, commandRefreshResultMsg{Key: refreshTarget, MR: refreshedTarget(), FetchStartedAt: started})

	got, ok := mrByKey(m, refreshTarget)
	require.True(t, ok)
	assert.Equal(t, someMRs()[1].Title, got.Title, "a refresh older than a local write must not overwrite it")
}

func TestCommandRefresh_GuardsAgainstAnOlderFullFetch(t *testing.T) {
	m, _ := newRefreshModel(t)
	fullFetchStarted := time.Now()
	refreshStarted := fullFetchStarted.Add(time.Second)

	m, _ = step(t, m, commandRefreshResultMsg{Key: refreshTarget, MR: refreshedTarget(), FetchStartedAt: refreshStarted})
	m.fetchSeq = 1
	m, _ = step(t, m, FetchResultMsg{MRs: someMRs(), FetchStartedAt: fullFetchStarted, Seq: 1})

	got, ok := mrByKey(m, refreshTarget)
	require.True(t, ok)
	assert.Equal(t, "retitled by the command", got.Title,
		"a full fetch started before the refresh must not land its older copy")
}

func TestCommandRefresh_MergedMR_LeavesTheOpenBoard(t *testing.T) {
	m, _ := newRefreshModel(t)
	merged := refreshedTarget()
	merged.MergedAt = time.Now()

	m, _ = step(t, m, commandRefreshResultMsg{Key: refreshTarget, MR: merged, FetchStartedAt: time.Now()})

	_, ok := mrByKey(m, refreshTarget)
	assert.False(t, ok, "a merged MR leaves the open board")
	assert.Len(t, m.allMRs, len(someMRs())-1)
}

// TestCommandResult_Demo_CardReflectsTheCommandsWrite runs the refresh against
// the demo adapter: a write made behind the board's back, as a foreground
// tool's would be, shows on the card once the command returns.
func TestCommandResult_Demo_CardReflectsTheCommandsWrite(t *testing.T) {
	h := newDetachedHarness(t, nil)
	focused := *h.m.board.FocusedMR()
	key := focused.Key()
	before, ok := mrByKey(h.m, key)
	require.True(t, ok)
	require.NoError(t, h.demo.MRSource().SetReviewers(context.Background(),
		int64(key.ProjectID), int64(key.IID), nil))
	require.NotEmpty(t, before.Reviewers, "the fixture's focused MR must start with reviewers")

	h.deliver(t, h.send(CommandResultMsg{CommandName: argvTestCommandName, Err: errors.New("exit status 1"), MR: key}))

	after, ok := mrByKey(h.m, key)
	require.True(t, ok)
	assert.Empty(t, after.Reviewers, "the card reflects the write the command made")
	assert.Equal(t, key, h.m.board.FocusedMR().Key(), "focus survives the refresh")
	assert.Contains(t, viewText(h), "review failed")
}
