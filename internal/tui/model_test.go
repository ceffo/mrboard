package tui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	m := New(context.Background(), cfg, src, noopStore{}, "dev", Options{})

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
	if m.State() != StateBoard {
		t.Fatalf("expected stateBoard, got %v", m.State())
	}
}

func TestModel_FetchResultMsg_PopulatesAllMRs(t *testing.T) {
	mrs := someMRs()
	m := makeModel(t, mrs, "")
	if len(m.AllMRs()) != len(mrs) {
		t.Fatalf("expected %d MRs, got %d", len(mrs), len(m.AllMRs()))
	}
}

// --- Fetch error ---

func TestModel_FetchErrMsg_TransitionsToErrorState(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	m := New(context.Background(), &config.Config{}, src, noopStore{}, "dev", Options{})
	next, _ := m.Update(FetchErrMsg{Err: errors.New("network down")})
	m2 := next.(Model)

	if m2.State() != StateError {
		t.Fatalf("expected stateError, got %v", m2.State())
	}
	if m2.ErrMsg() == "" {
		t.Fatal("expected non-empty error message")
	}
}

// --- Partial results ---

func TestModel_FetchResultMsg_PartialResults_ShowsMRsAndErrors(t *testing.T) {
	src := mocks.NewMockMergeRequestSource(t)
	src.EXPECT().FetchAll(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	m := New(context.Background(), &config.Config{}, src, noopStore{}, "dev", Options{})
	next, _ := m.Update(FetchResultMsg{
		MRs:    someMRs(),
		Errors: []error{errors.New("source A failed")},
	})
	m2 := next.(Model)

	if m2.State() != StateBoard {
		t.Fatalf("expected stateBoard, got %v", m2.State())
	}
	if len(m2.AllMRs()) != 2 {
		t.Fatalf("expected 2 MRs, got %d", len(m2.AllMRs()))
	}
	if len(m2.Errors()) != 1 {
		t.Fatalf("expected 1 error, got %d", len(m2.Errors()))
	}
}

// --- Sort cycling ---

func TestModel_SortKey_CyclesField(t *testing.T) {
	m := makeModel(t, someMRs(), "")

	// Starting state: repo_iid asc. First 's' → repo_iid desc.
	m2, _ := m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m2m := m2.(Model)
	if m2m.SortFieldKey() != "repo_iid" || !m2m.SortDesc() {
		t.Fatalf("after 1st s: want repo_iid desc, got field=%s desc=%v",
			m2m.SortFieldKey(), m2m.SortDesc())
	}

	// Second 's' → author asc.
	m3, _ := m2m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m3m := m3.(Model)
	if m3m.SortFieldKey() != sortKeyAuthor || m3m.SortDesc() {
		t.Fatalf("after 2nd s: want author asc, got field=%s desc=%v",
			m3m.SortFieldKey(), m3m.SortDesc())
	}

	// Third 's' → author desc.
	m4, _ := m3m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m4m := m4.(Model)
	if m4m.SortFieldKey() != sortKeyAuthor || !m4m.SortDesc() {
		t.Fatalf("after 3rd s: want author desc, got field=%s desc=%v",
			m4m.SortFieldKey(), m4m.SortDesc())
	}

	// Fourth 's' → age asc.
	m5, _ := m4m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m5m := m5.(Model)
	if m5m.SortFieldKey() != "age" || m5m.SortDesc() {
		t.Fatalf("after 4th s: want age asc, got field=%s desc=%v",
			m5m.SortFieldKey(), m5m.SortDesc())
	}
}

// --- My-view toggle ---

func TestModel_TabKey_TogglesMyView(t *testing.T) {
	m := makeModel(t, someMRs(), "alice")
	if m.MyView() {
		t.Fatal("expected myView=false initially")
	}

	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m2m := m2.(Model)
	if !m2m.MyView() {
		t.Fatal("expected myView=true after first tab")
	}

	m3, _ := m2m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m3m := m3.(Model)
	if m3m.MyView() {
		t.Fatal("expected myView=false after second tab")
	}
}

func TestModel_TabKey_DisabledWithoutCurrentUser(t *testing.T) {
	m := makeModel(t, someMRs(), "") // no current user
	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m2m := m2.(Model)
	if m2m.MyView() {
		t.Fatal("my-view should not activate when CurrentUser is empty")
	}
}

// --- Detail panel open / close ---

func TestModel_EnterKey_OpensDetailPanel(t *testing.T) {
	// Need at least one MR in the board so FocusedMR() is non-nil.
	m := makeModel(t, someMRs(), "")

	// Deliver a detail result so we don't spin waiting for fetch.
	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2m := m2.(Model)
	// showDetail is set immediately on enter even before detail fetch resolves.
	if !m2m.ShowDetail() {
		t.Fatal("expected showDetail=true after pressing enter")
	}
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
	if m3m.ShowDetail() {
		t.Fatal("expected showDetail=false after pressing esc")
	}
}
