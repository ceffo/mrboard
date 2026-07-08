package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	mock "github.com/stretchr/testify/mock"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	dmocks "github.com/ceffo/mrboard/internal/domain/mocks"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

// runCmd recursively executes a Cmd, following any tea.BatchMsg it produces, so
// that side-effecting leaf commands (like the notify webhook call) actually run.
func runCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			runCmd(t, c)
		}
	}
}

// modelWithNotifier builds a board-state Model wired to the given notifier.
func modelWithNotifier(t *testing.T, notifier domain.Notifier) Model {
	t.Helper()
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	m := New(context.Background(), &config.Config{}, src, noopStore{}, notifier, nil, nil, "dev", Options{})
	next, _ := m.Update(FetchResultMsg{MRs: nil})
	return next.(Model)
}

func savedMR() domain.MergeRequest {
	return domain.MergeRequest{
		ID: 1, IID: 10, Author: "carol", ProjectPath: "org/gamma",
		Reviewers: []domain.ReviewerInfo{{Username: "dave", IsApprover: true}},
	}
}

func TestHandleReviewersSaved_ApproversChanged_Notifies(t *testing.T) {
	notifier := dmocks.NewMockNotifier(t)
	notifier.EXPECT().Notify(mock.Anything, mock.Anything).Return(nil).Once()

	m := modelWithNotifier(t, notifier)
	_, cmd := m.handleReviewersSaved(ReviewersSavedMsg{MR: savedMR(), ApproversChanged: true})
	runCmd(t, cmd)
	// notifier's Once() expectation is asserted via t.Cleanup.
}

func TestHandleReviewersSaved_ApproversUnchanged_DoesNotNotify(t *testing.T) {
	// No EXPECT() set: any Notify call is an unexpected-call failure.
	notifier := dmocks.NewMockNotifier(t)

	m := modelWithNotifier(t, notifier)
	_, cmd := m.handleReviewersSaved(ReviewersSavedMsg{MR: savedMR(), ApproversChanged: false})
	runCmd(t, cmd)
}
