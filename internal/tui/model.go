package tui

import (
	"os/exec"
	"runtime"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"
	"github.com/mrboard/mrboard/internal/config"
	"github.com/mrboard/mrboard/internal/domain"
	"github.com/mrboard/mrboard/internal/service"
)

// sortField identifies which MR attribute to sort by.
type sortField int

const (
	sortByRepoIID sortField = iota
	sortByAuthor
	sortByAge
	numSortFields
)

// Sort field string keys used in persisted state.
const (
	sortKeyRepoIID = "repo_iid"
	sortKeyAuthor  = "author"
	sortKeyAge     = "age"
)

func (f sortField) next() sortField { return (f + 1) % numSortFields }

func (f sortField) display() string {
	switch f {
	case sortByAuthor:
		return sortKeyAuthor
	case sortByAge:
		return sortKeyAge
	default:
		return "repo·id"
	}
}

func (f sortField) stateKey() string {
	switch f {
	case sortByAuthor:
		return sortKeyAuthor
	case sortByAge:
		return sortKeyAge
	default:
		return sortKeyRepoIID
	}
}

func sortFieldFromState(s string) sortField {
	switch s {
	case sortKeyAuthor:
		return sortByAuthor
	case sortKeyAge:
		return sortByAge
	default:
		return sortByRepoIID
	}
}

// sortLabel returns the footer label for the current sort state.
func sortLabel(field sortField, desc bool) string {
	dir := "↑"
	if desc {
		dir = "↓"
	}
	return "sort:" + field.display() + dir
}

// advanceSort cycles to the next sort state.
// Pressing s once flips direction; pressing again on the new direction advances the field.
// Cycle: (field, asc) → (field, desc) → (nextField, asc) → …
func advanceSort(field sortField, desc bool) (sortField, bool) {
	if !desc {
		return field, true
	}
	return field.next(), false
}

const (
	detailWidthRatio   = 40  // percent of total width for the detail panel
	detailWidthDivisor = 100 // divisor for percentage calculation
)

type appState int

const (
	stateLoading appState = iota
	stateBoard
	stateError
)

const (
	defaultBoardWidth  = 80
	defaultBoardHeight = 24
	headerHeight       = 1
	footerHeight       = 1
	chromeHeight       = headerHeight + footerHeight
)

// FetchResultMsg carries the result of a successful (or partial) fetch.
type FetchResultMsg struct {
	MRs    []domain.MergeRequest
	Errors []error
}

// FetchErrMsg carries a fatal fetch error (e.g. network down, bad token).
type FetchErrMsg struct{ Err error }

// DetailFetchResultMsg carries the description and threads for a single MR.
type DetailFetchResultMsg struct {
	ProjectID   int
	MRIID       int
	Description string
	Threads     []domain.Thread
	Err         error
}

// Model is the root Bubble Tea model for mrboard.
type Model struct {
	state       appState
	header      headerWidget
	board       boardWidget
	footer      footerWidget
	sp          spinnerWidget
	detail      detailWidget
	showDetail  bool
	keys        KeyMap
	detailKeys  DetailKeyMap
	styles      Styles
	width       int
	height      int
	errors      []error
	errMsg      string
	cfg         *config.Config
	src         service.MergeRequestSource
	allMRs      []domain.MergeRequest
	currentUser string
	myView      bool
	sortField   sortField
	sortDesc    bool
}

// New creates a ready-to-run mrboard model, restoring sort/view state from st.
func New(cfg *config.Config, src service.MergeRequestSource, st config.State) Model {
	styles := NewStyles()
	keys := DefaultKeyMap

	sf := sortFieldFromState(st.SortField)
	keys.Sort = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", sortLabel(sf, st.SortDesc)))

	myView := st.MyView && cfg.CurrentUser != ""
	if myView {
		keys.ToggleView = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "team view"))
	}
	if cfg.CurrentUser == "" {
		keys.ToggleView.SetEnabled(false)
	}

	m := Model{
		state:       stateLoading,
		header:      newHeaderWidget(styles),
		board:       newBoardWidget(styles, defaultBoardWidth, defaultBoardHeight-chromeHeight),
		footer:      newFooterWidget(keys, styles),
		sp:          newSpinnerWidget(),
		detail:      newDetailWidget(styles),
		keys:        keys,
		detailKeys:  DefaultDetailKeyMap,
		styles:      styles,
		cfg:         cfg,
		src:         src,
		currentUser: cfg.CurrentUser,
		myView:      myView,
		sortField:   sf,
		sortDesc:    st.SortDesc,
	}
	if myView {
		m.header.SetTitle("mrboard — @" + cfg.CurrentUser)
	}
	return m
}

// Init starts the spinner and fires the first data fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.sp.Init(), m.fetchCmd())
}

func (m Model) fetchCmd() tea.Cmd {
	src := m.src
	return func() tea.Msg {
		mrs, errs := src.FetchAll()
		return FetchResultMsg{MRs: mrs, Errors: errs}
	}
}

// Update handles all incoming messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.header.SetWidth(msg.Width)
		m.resizeBoard()
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

	case DetailFetchResultMsg:
		if m.showDetail && m.detail.mr != nil &&
			m.detail.mr.ProjectID == msg.ProjectID && m.detail.mr.IID == msg.MRIID {
			if msg.Err == nil {
				m.detail.mr.Description = msg.Description
				m.detail.SetThreads(msg.Threads)
			} else {
				m.detail.loading = false
			}
		}
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

	if m.showDetail {
		return m.handleKeyDetail(msg)
	}
	return m.handleKeyBoard(msg)
}

// handleKeyDetail handles keys while the detail panel owns focus.
func (m Model) handleKeyDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.CloseDetail):
		m.closeDetail()
		return m, nil
	case key.Matches(msg, m.detailKeys.ScrollUp):
		m.detail.ScrollUp()
	case key.Matches(msg, m.detailKeys.ScrollDown):
		m.detail.ScrollDown()
	case key.Matches(msg, m.keys.Open):
		if mr := m.board.FocusedMR(); mr != nil {
			return m, openBrowser(mr.WebURL)
		}
	}
	return m, nil
}

// handleKeyBoard handles keys while the kanban board owns focus.
func (m Model) handleKeyBoard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		if m.showDetail {
			m.closeDetail()
		}
		m.state = stateLoading
		return m, tea.Batch(m.sp.Init(), m.fetchCmd())
	case key.Matches(msg, m.keys.Sort):
		m.sortField, m.sortDesc = advanceSort(m.sortField, m.sortDesc)
		m.keys.Sort = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", sortLabel(m.sortField, m.sortDesc)))
		m.footer.SetKeyMap(m.keys)
		m.applyMRFilter()
		m.saveState()
	case key.Matches(msg, m.keys.ToggleView):
		m.myView = !m.myView
		if m.myView {
			m.header.SetTitle("mrboard — @" + m.currentUser)
			m.keys.ToggleView = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "team view"))
		} else {
			m.header.SetTitle("mrboard")
			m.keys.ToggleView = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "my view"))
		}
		m.footer.SetKeyMap(m.keys)
		m.applyMRFilter()
		m.saveState()
	case key.Matches(msg, m.keys.Open):
		if mr := m.board.FocusedMR(); mr != nil {
			return m, openBrowser(mr.WebURL)
		}
	case key.Matches(msg, m.keys.Detail):
		if mr := m.board.FocusedMR(); mr != nil {
			m.openDetail(mr)
			return m, m.fetchDetailCmd(mr)
		}
	}
	return m, nil
}

func (m *Model) openDetail(mr *domain.MergeRequest) {
	m.showDetail = true
	m.board.SetActive(false)
	m.footer.SetKeyMap(m.detailKeys)
	m.detail.SetMR(mr)
	m.resizeBoard()
}

func (m *Model) closeDetail() {
	m.showDetail = false
	m.board.SetActive(true)
	m.footer.SetKeyMap(m.keys)
	m.resizeBoard()
}

func (m *Model) resizeBoard() {
	if m.showDetail {
		detailW := m.width * detailWidthRatio / detailWidthDivisor
		boardW := m.width - detailW
		m.board.SetSize(boardW, m.height-chromeHeight)
		m.detail.SetSize(detailW, m.height-chromeHeight)
	} else {
		m.board.SetSize(m.width, m.height-chromeHeight)
	}
}

func (m Model) fetchDetailCmd(mr *domain.MergeRequest) tea.Cmd {
	src := m.src
	projectID := int64(mr.ProjectID)
	mrIID := int64(mr.IID)
	return func() tea.Msg {
		desc, threads, err := src.GetDetail(projectID, mrIID)
		return DetailFetchResultMsg{
			ProjectID:   int(projectID),
			MRIID:       int(mrIID),
			Description: desc,
			Threads:     threads,
			Err:         err,
		}
	}
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
		headerStr := m.header.render()
		footerStr := m.footer.render()
		boardH := m.height - chromeHeight

		boardStr := m.board.render()
		if boardH > 0 {
			lines := strings.SplitN(boardStr, "\n", boardH+2) //nolint:mnd
			if len(lines) > boardH {
				lines = lines[:boardH]
			}
			boardStr = strings.Join(lines, "\n")
			boardStr = lip.NewStyle().Height(boardH).Render(boardStr)
		}

		var contentStr string
		if m.showDetail {
			detailStr := m.detail.render()
			if boardH > 0 {
				dLines := strings.SplitN(detailStr, "\n", boardH+2) //nolint:mnd
				if len(dLines) > boardH {
					dLines = dLines[:boardH]
				}
				detailStr = strings.Join(dLines, "\n")
				detailStr = lip.NewStyle().Height(boardH).Render(detailStr)
			}
			contentStr = joinHorizontalTop(boardStr, detailStr)
		} else {
			contentStr = boardStr
		}

		var errLines string
		for _, e := range m.errors {
			errLines += "\n" + m.styles.ErrorMsg.Render("⚠ "+e.Error())
		}

		return headerStr + "\n" + contentStr + errLines + "\n" + footerStr
	}
	return ""
}

// applyMRFilter applies the my-view filter then sorts, and pushes the result into the board.
func (m *Model) applyMRFilter() {
	mrs := service.FilterAndSort(m.allMRs, service.FilterOptions{
		MyView:      m.myView,
		CurrentUser: m.currentUser,
		SortField:   m.sortField.stateKey(),
		SortDesc:    m.sortDesc,
	})
	m.board.SetMRs(mrs)
	m.header.SetMRs(mrs)
}

func (m *Model) saveState() {
	config.SaveState(config.State{
		SortField: m.sortField.stateKey(),
		SortDesc:  m.sortDesc,
		MyView:    m.myView,
	})
}

func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("open", url)
		} else {
			cmd = exec.Command("xdg-open", url)
		}
		if err := cmd.Start(); err != nil {
			return nil
		}
		return nil
	}
}
