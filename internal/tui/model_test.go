package tui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc/mocks"
)

// noopStore is a StateStore that always returns DefaultAppState and discards saves.
type noopStore struct{}

func (noopStore) Load() (domain.AppState, error) { return domain.DefaultAppState(), nil }
func (noopStore) Save(domain.AppState) error     { return nil }

// makeModel creates a Model wired to a mock source and transitions it to
// stateBoard by delivering initialMRs via FetchResultMsg.
func makeModel(t *testing.T, initialMRs []domain.MergeRequest, currentUser string) Model {
	t.Helper()
	src := mocks.NewMockMergeRequestSource(t)
	// fetchCmd will call FetchAll once on Init; allow but don't require it.
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(initialMRs, nil).Maybe()

	cfg := &config.Config{CurrentUser: currentUser}
	m := New(context.Background(), cfg, src, noopStore{}, nil, nil, nil, "dev", Options{})

	// Deliver results directly without running the real fetch.
	next, _ := m.Update(FetchResultMsg{MRs: initialMRs})
	return next.(Model)
}

func someMRs() []domain.MergeRequest {
	return []domain.MergeRequest{
		{
			ID: 1, IID: 10, Author: "alice", ProjectPath: "org/alpha",
			Reviewers: []domain.ReviewerInfo{{Username: "bob", State: domain.ReviewerNotStarted}},
		},
		{
			ID: 2, IID: 20, Author: "bob", ProjectPath: "org/beta",
			Reviewers: []domain.ReviewerInfo{{Username: "alice", State: domain.ReviewerNotStarted}},
		},
	}
}

// --- Fetch success ---

func TestModel_FetchResultMsg_TransitionsToBoardState(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	assert.Equal(t, StateBoard, m.State())
}

func TestModel_FetchResultMsg_PopulatesAllMRs(t *testing.T) {
	mrs := someMRs()
	m := makeModel(t, mrs, "")
	assert.Len(t, m.AllMRs(), len(mrs))
}

// --- Fetch error ---

func TestModel_FetchErrMsg_TransitionsToErrorState(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	m := New(context.Background(), &config.Config{}, src, noopStore{}, nil, nil, nil, "dev", Options{})
	next, _ := m.Update(FetchErrMsg{Err: errors.New("network down")})
	m2 := next.(Model)

	assert.Equal(t, StateError, m2.State())
	assert.NotEmpty(t, m2.ErrMsg(), "expected non-empty error message")
}

// --- Partial results ---

func TestModel_FetchResultMsg_PartialResults_ShowsMRsAndErrors(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	m := New(context.Background(), &config.Config{}, src, noopStore{}, nil, nil, nil, "dev", Options{})
	next, _ := m.Update(FetchResultMsg{
		MRs:    someMRs(),
		Errors: []error{errors.New("source A failed")},
	})
	m2 := next.(Model)

	assert.Equal(t, StateBoard, m2.State())
	assert.Len(t, m2.AllMRs(), 2)
	assert.Len(t, m2.Errors(), 1)
}

// --- Sort cycling ---

func TestModel_SortKey_CyclesField(t *testing.T) {
	m := makeModel(t, someMRs(), "")

	// Starting state: repo_iid asc. First 's' → repo_iid desc.
	m2, _ := m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m2m := m2.(Model)
	assert.Equal(t, "repo_iid", m2m.SortFieldKey(), "after 1st s: want repo_iid desc")
	assert.True(t, m2m.SortDesc(), "after 1st s: want repo_iid desc")

	// Second 's' → assignee asc.
	m3, _ := m2m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m3m := m3.(Model)
	assert.Equal(t, sortKeyAssignee, m3m.SortFieldKey(), "after 2nd s: want assignee asc")
	assert.False(t, m3m.SortDesc(), "after 2nd s: want assignee asc")

	// Third 's' → assignee desc.
	m4, _ := m3m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m4m := m4.(Model)
	assert.Equal(t, sortKeyAssignee, m4m.SortFieldKey(), "after 3rd s: want assignee desc")
	assert.True(t, m4m.SortDesc(), "after 3rd s: want assignee desc")

	// Fourth 's' → age asc.
	m5, _ := m4m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m5m := m5.(Model)
	assert.Equal(t, "age", m5m.SortFieldKey(), "after 4th s: want age asc")
	assert.False(t, m5m.SortDesc(), "after 4th s: want age asc")
}

// --- My-view toggle ---

func TestModel_TabKey_TogglesMyView(t *testing.T) {
	m := makeModel(t, someMRs(), "alice")
	assert.False(t, m.MyView(), "expected myView=false initially")

	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m2m := m2.(Model)
	assert.True(t, m2m.MyView(), "expected myView=true after first tab")

	m3, _ := m2m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m3m := m3.(Model)
	assert.False(t, m3m.MyView(), "expected myView=false after second tab")
}

func TestModel_TabKey_DisabledWithoutCurrentUser(t *testing.T) {
	m := makeModel(t, someMRs(), "") // no current user
	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m2m := m2.(Model)
	assert.False(t, m2m.MyView(), "my-view should not activate when CurrentUser is empty")
}

// --- Detail panel open / close ---

func TestModel_EnterKey_OpensDetailPanel(t *testing.T) {
	// Need at least one MR in the board so FocusedMR() is non-nil.
	m := makeModel(t, someMRs(), "")

	// Deliver a detail result so we don't spin waiting for fetch.
	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2m := m2.(Model)
	// showDetail is set immediately on enter even before detail fetch resolves.
	assert.True(t, m2m.ShowDetail(), "expected showDetail=true after pressing enter")
}

func TestModel_EscKey_ClosesDetailPanel(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	// Open detail first.
	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2m := m2.(Model)
	if !m2m.ShowDetail() {
		t.Skip("detail did not open — no focused MR, skipping close test")
	}

	// Close with esc.
	m3, _ := m2m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m3m := m3.(Model)
	assert.False(t, m3m.ShowDetail(), "expected showDetail=false after pressing esc")
}

// --- ticket index ---

const (
	ticketKeyAlpha = "OD-100"
	ticketKeyBeta  = "OD-200"
)

func mrWithTicketKey(id, iid int, ticketKey string) domain.MergeRequest {
	title := "no ticket key"
	if ticketKey != "" {
		title = "feat(" + ticketKey + "): change"
	}
	return domain.MergeRequest{ID: id, IID: iid, Title: title}
}

func TestModel_TicketIndex_BuildsOnFetch(t *testing.T) {
	mrs := []domain.MergeRequest{
		mrWithTicketKey(1, 10, ticketKeyAlpha),
		mrWithTicketKey(2, 20, ticketKeyAlpha), // sibling
		mrWithTicketKey(3, 30, ticketKeyBeta),
		{ID: 4, IID: 40, Title: "no ticket key"},
	}
	m := makeModel(t, mrs, "")

	got := m.SiblingMRs(ticketKeyAlpha)
	assert.Len(t, got, 2, "expected 2 siblings for %s", ticketKeyAlpha)
	assert.Len(t, m.SiblingMRs(ticketKeyBeta), 1, "expected 1 MR for %s", ticketKeyBeta)
}

func TestModel_TicketIndex_EmptyKeyReturnsNil(t *testing.T) {
	m := makeModel(t, someMRs(), "")
	assert.Nil(t, m.SiblingMRs(""), "expected nil for empty ticket key")
}

func TestModel_TicketIndex_NoTicketKeysProducesEmptyIndex(t *testing.T) {
	m := makeModel(t, someMRs(), "") // someMRs have no ticket keys in titles
	assert.Nil(t, m.SiblingMRs(ticketKeyAlpha), "expected nil when no MR has ticket key %s", ticketKeyAlpha)
}
