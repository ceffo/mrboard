package tui

import (
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"
	"github.com/mrboard/mrboard/internal/config"
	"github.com/mrboard/mrboard/internal/domain"
	"github.com/mrboard/mrboard/internal/gitlab"
)

type appState int

const (
	stateLoading appState = iota
	stateBoard
	stateError
)

// FetchResultMsg carries the result of a successful (or partial) fetch.
type FetchResultMsg struct {
	MRs    []domain.MergeRequest
	Errors []error
}

// FetchErrMsg carries a fatal fetch error (e.g. network down, bad token).
type FetchErrMsg struct{ Err error }

// Model is the root Bubble Tea model for mrboard.
type Model struct {
	state     appState
	board     boardWidget
	footer    footerWidget
	sp        spinnerWidget
	keys      KeyMap
	styles    Styles
	width     int
	height    int
	errors    []error
	errMsg    string
	cfg       *config.Config
	client    *gitlab.Client
	allMRs    []domain.MergeRequest
	hideStale bool
}

// New creates a ready-to-run mrboard model.
func New(cfg *config.Config, client *gitlab.Client) Model {
	styles := NewStyles()
	keys := DefaultKeyMap
	return Model{
		state:  stateLoading,
		board:  newBoardWidget(styles, 80, 24),
		footer: newFooterWidget(keys),
		sp:     newSpinnerWidget(),
		keys:   keys,
		styles: styles,
		cfg:    cfg,
		client: client,
	}
}

// Init starts the spinner and fires the first data fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.sp.Init(), m.fetchCmd())
}

func (m Model) fetchCmd() tea.Cmd {
	cfg := m.cfg
	client := m.client
	return func() tea.Msg {
		mrs, errs := gitlab.FetchAll(client, cfg)
		return FetchResultMsg{MRs: mrs, Errors: errs}
	}
}

// Update handles all incoming messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.board.SetSize(msg.Width, msg.Height-2)
		return m, nil

	case FetchResultMsg:
		m.state = stateBoard
		m.allMRs = msg.MRs
		m.errors = msg.Errors
		m.applyMRFilter()
		return m, nil

	case FetchErrMsg:
		m.state = stateError
		m.errMsg = msg.Err.Error()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	default:
		if m.state == stateLoading {
			updated, cmd := m.sp.Update(msg)
			m.sp = updated.(spinnerWidget)
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}

	if m.state != stateBoard {
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Up):
		m.board.MoveUp()
	case key.Matches(msg, m.keys.Down):
		m.board.MoveDown()
	case key.Matches(msg, m.keys.Left):
		m.board.MoveLeft()
	case key.Matches(msg, m.keys.Right):
		m.board.MoveRight()
	case key.Matches(msg, m.keys.Refresh):
		m.state = stateLoading
		return m, tea.Batch(m.sp.Init(), m.fetchCmd())
	case key.Matches(msg, m.keys.HideStale):
		if m.cfg.StaleThresholdDays > 0 {
			m.hideStale = !m.hideStale
			m.applyMRFilter()
		}
	case key.Matches(msg, m.keys.Open):
		if mr := m.board.FocusedMR(); mr != nil {
			return m, openBrowser(mr.WebURL)
		}
	}
	return m, nil
}

// View renders the full screen. Only the root model sets AltScreen.
func (m Model) View() tea.View {
	content := m.renderContent()
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) renderContent() string {
	switch m.state {
	case stateLoading:
		msg := m.sp.spinner.View() + " Loading…"
		return lip.Place(m.width, m.height, lip.Center, lip.Center, msg)

	case stateError:
		body := m.styles.ErrorMsg.Render("Error: "+m.errMsg) + "\n\nPress q to quit."
		return lip.Place(m.width, m.height, lip.Center, lip.Center, body)

	case stateBoard:
		var sb strings.Builder
		sb.WriteString(m.board.render())
		for _, e := range m.errors {
			sb.WriteString("\n" + m.styles.ErrorMsg.Render("⚠ "+e.Error()))
		}
		sb.WriteString("\n" + m.footer.help.View(m.keys))
		return sb.String()
	}
	return ""
}

// applyMRFilter pushes allMRs (optionally filtered for staleness) into the board.
func (m *Model) applyMRFilter() {
	mrs := m.allMRs
	if m.hideStale && m.cfg.StaleThresholdDays > 0 {
		threshold := time.Duration(m.cfg.StaleThresholdDays) * 24 * time.Hour
		now := time.Now()
		filtered := make([]domain.MergeRequest, 0, len(mrs))
		for _, mr := range mrs {
			base := mr.NonDraftSince
			if base.IsZero() {
				base = mr.CreatedAt
			}
			if base.IsZero() || now.Sub(base) <= threshold {
				filtered = append(filtered, mr)
			}
		}
		mrs = filtered
	}
	m.board.SetMRs(mrs)
}

func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("open", url)
		} else {
			cmd = exec.Command("xdg-open", url)
		}
		_ = cmd.Start()
		return nil
	}
}
