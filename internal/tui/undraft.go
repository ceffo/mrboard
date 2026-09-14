package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// undraftRequestedMsg is emitted when the user confirms undrafting an MR.
type undraftRequestedMsg struct{ MR domain.MergeRequest }

// UndraftResultMsg carries the outcome of stripping mr's draft marker.
type UndraftResultMsg struct {
	MR  domain.MergeRequest
	Err error
}

// newUndraftConfirmDialog builds the yes/no dialog offered when the user
// presses the undraft key on a focused draft MR.
func newUndraftConfirmDialog(mr domain.MergeRequest, styles Styles, keys ConfirmKeyMap) *confirmWidget {
	body := fmt.Sprintf("undraft !%d — %s?", mr.IID, mr.Title)
	return newConfirmWidget("Undraft MR", body, styles, keys, func() tea.Msg {
		return undraftRequestedMsg{MR: mr}
	})
}

// makeUndraftCmd asks mrsvc to undraft mr. The returned MR's Phase is
// reclassified locally so the caller can move it to the right column
// immediately, without waiting for the next fetch to confirm it — Title is
// left untouched since how a source achieves "no longer draft" is its own
// concern (docs/adr/0002).
func makeUndraftCmd(base context.Context, src mrsvc.MergeRequestSource, mr domain.MergeRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, ticketFetchTimeout)
		defer cancel()
		if err := src.Undraft(ctx, int64(mr.ProjectID), int64(mr.IID)); err != nil {
			return UndraftResultMsg{MR: mr, Err: err}
		}
		updated := mr
		updated.Phase = domain.ClassifyPhase(false, true, mr.Reviewers)
		return UndraftResultMsg{MR: updated}
	}
}
