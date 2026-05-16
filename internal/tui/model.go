package tui

import (
	"context"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	ilog "github.com/ceffo/mrboard/internal/log"
	"github.com/ceffo/mrboard/pkg/theme"
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
	fetchTimeout       = 60 * time.Second
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

// tickMsg is sent every minute to refresh displayed durations.
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Minute, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

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

// Options are session-scoped overrides passed via CLI flags.
// They are not persisted to the state file.
type Options struct {
	ThemeOverride string // --theme flag; "" means use state
	ModeOverride  string // --mode flag; "" means use state
}

// Model is the root Bubble Tea model for mrboard.
type Model struct {
	state           appState
	header          headerWidget
	board           boardWidget
	footer          footerWidget
	sp              spinnerWidget
	detail          detailWidget
	showDetail      bool
	filterPopup     filterPopupWidget
	showFilter      bool
	themePicker     themePickerWidget
	showThemePicker bool
	keys            KeyMap
	detailKeys      DetailKeyMap
	filterKeys      FilterPopupKeyMap
	themePickerKeys ThemePickerKeyMap
	styles          Styles
	theme           theme.Theme[ColorKey]
	themeName       string // currently active theme name
	themeMode       string // "auto", "dark", "light"
	hasDarkBg       bool
	width           int
	height          int
	errors          []error
	errMsg          string
	cfg             *config.Config
	src             mrsvc.MergeRequestSource
	store           StateStore
	allMRs          []domain.MergeRequest
	userMap         map[string]string
	currentUser     string
	viewMode        ViewMode
	sortField       sortField
	sortDesc        bool
	filterPhases    map[domain.MRPhase]bool
	filterAuthors   []string
	filterReviewers []string
	fetchCancel     context.CancelFunc
	baseCtx         context.Context
	logger          *slog.Logger
	isRefreshing    bool
}

// New creates a ready-to-run mrboard model. It loads persisted UI state from
// store; on error it logs and falls back to DefaultState().
func New(
	ctx context.Context,
	cfg *config.Config,
	src mrsvc.MergeRequestSource,
	store StateStore,
	version string,
	opts Options,
) Model {
	logger := ilog.FromContext(ctx)

	st, err := store.Load()
	if err != nil {
		logger.Error("statestore: load failed, using defaults", "err", err)
		st = DefaultState()
	}

	// Resolve theme name and mode: flag overrides > state > defaults.
	themeName := st.ThemeName
	if themeName == "" {
		themeName = "default"
	}
	if opts.ThemeOverride != "" {
		themeName = opts.ThemeOverride
	}

	themeMode := st.ThemeMode
	if themeMode == "" {
		themeMode = themeModeAuto
	}
	if opts.ModeOverride != "" {
		themeMode = opts.ModeOverride
	}

	th := LoadThemeByName(themeName)

	// Default to dark; corrected by BackgroundColorMsg on first update.
	initialDark := themeMode == themeModeDark || themeMode != themeModeLight
	styles := NewStyles(th, initialDark)
	keys := DefaultKeyMap

	sf := sortFieldFromState(st.SortField)
	keys.Sort = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", sortLabel(sf, st.SortDesc)))

	viewMode := st.ViewMode
	if viewMode == ViewMine && cfg.CurrentUser == "" {
		viewMode = ViewAll
	}
	if viewMode == ViewMine {
		keys.ToggleView = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "team view"))
	}
	if cfg.CurrentUser == "" {
		keys.ToggleView.SetEnabled(false)
	}

	m := Model{
		state:           stateLoading,
		header:          newHeaderWidget(styles),
		board:           newBoardWidget(styles, defaultBoardWidth, defaultBoardHeight-chromeHeight),
		footer:          newFooterWidget(keys, styles, version),
		sp:              newSpinnerWidget(),
		detail:          newDetailWidget(styles),
		keys:            keys,
		detailKeys:      DefaultDetailKeyMap,
		filterKeys:      DefaultFilterPopupKeyMap,
		themePickerKeys: DefaultThemePickerKeyMap,
		styles:          styles,
		theme:           th,
		themeName:       themeName,
		themeMode:       themeMode,
		hasDarkBg:       initialDark,
		cfg:             cfg,
		src:             src,
		store:           store,
		currentUser:     cfg.CurrentUser,
		viewMode:        viewMode,
		sortField:       sf,
		sortDesc:        st.SortDesc,
		baseCtx:         ctx,
		logger:          logger,
	}
	if viewMode == ViewMine {
		m.header.SetTitle("mrboard — @" + cfg.CurrentUser)
	}
	logger.Info("tui: starting", "version", version, "theme", themeName, "mode", themeMode, "view", int(viewMode))
	return m
}

// Init starts the spinner, fires the first data fetch, and schedules the minute ticker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.sp.Init(), makeFetchCmd(m.baseCtx, m.src), tickCmd())
}

// makeFetchCmd returns a Cmd that fetches all MRs and a cancel func to abort it.
// The cancel is also called via defer inside the goroutine once the fetch finishes.
func makeFetchCmd(base context.Context, src mrsvc.MergeRequestSource) tea.Cmd {
	ctx, cancel := context.WithTimeout(base, fetchTimeout)
	return func() tea.Msg {
		defer cancel()
		mrs, errs := src.FetchAll(ctx)
		return FetchResultMsg{MRs: mrs, Errors: errs}
	}
}

// startFetch builds a fetch Cmd and stores its cancel func in the model so
// that a subsequent 'q' press can abort an in-flight request.
func (m *Model) startFetch() tea.Cmd {
	ctx, cancel := context.WithTimeout(m.baseCtx, fetchTimeout)
	if m.fetchCancel != nil {
		m.fetchCancel()
	}
	m.fetchCancel = cancel
	src := m.src
	return func() tea.Msg {
		defer cancel()
		mrs, errs := src.FetchAll(ctx)
		return FetchResultMsg{MRs: mrs, Errors: errs}
	}
}

// Update handles all incoming messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		if m.themeMode == themeModeAuto || m.themeMode == "" {
			m.hasDarkBg = msg.IsDark()
			m.applyTheme()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.header.SetWidth(msg.Width)
		m.footer.SetWidth(msg.Width)
		m.resizeBoard()
		return m, nil

	case FetchResultMsg:
		m.state = stateBoard
		m.isRefreshing = false
		m.allMRs = msg.MRs
		m.errors = msg.Errors
		m.logger.Info("tui: fetch result", "mrs", len(msg.MRs), "errors", len(msg.Errors))
		for _, e := range msg.Errors {
			m.logger.Warn("tui: fetch partial error", "error", e)
		}
		m.applyMRFilter()
		return m, nil

	case FetchErrMsg:
		m.isRefreshing = false
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

	case FilterAppliedMsg:
		m.filterPhases = msg.Phases
		m.filterAuthors = msg.Authors
		m.filterReviewers = msg.Reviewers
		m.applyMRFilter()
		return m, nil

	case FilterClosedMsg:
		m.showFilter = false
		return m, nil

	case ThemePickerClosedMsg:
		m.showThemePicker = false
		return m, nil

	case ThemeChangedMsg:
		m.themeName = msg.Name
		m.themeMode = msg.Mode
		switch msg.Mode {
		case themeModeDark:
			m.hasDarkBg = true
		case themeModeLight:
			m.hasDarkBg = false
			// themeModeAuto or "": keep current hasDarkBg from last BackgroundColorMsg
		}
		m.theme = LoadThemeByName(msg.Name)
		m.applyTheme()
		m.saveState()
		return m, nil

	case tickMsg:
		return m, tickCmd()

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	default:
		if m.state == stateLoading || m.isRefreshing {
			updated, cmd := m.sp.Update(msg)
			m.sp = updated.(spinnerWidget)
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		if m.fetchCancel != nil {
			m.fetchCancel()
		}
		return m, tea.Quit
	}

	if m.state != stateBoard || m.isRefreshing {
		return m, nil
	}

	if m.showFilter {
		updated, cmd := m.filterPopup.Update(msg)
		m.filterPopup = updated.(filterPopupWidget)
		return m, cmd
	}

	if m.showThemePicker {
		updated, cmd := m.themePicker.Update(msg)
		m.themePicker = updated.(themePickerWidget)
		return m, cmd
	}

	// 't' opens the theme picker from board or detail mode.
	if key.Matches(msg, m.keys.Theme) {
		m.openThemePicker()
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
		if len(m.allMRs) > 0 {
			m.isRefreshing = true
			return m, tea.Batch(m.sp.Init(), m.startFetch())
		}
		m.state = stateLoading
		return m, tea.Batch(m.sp.Init(), m.startFetch())
	case key.Matches(msg, m.keys.Sort):
		m.sortField, m.sortDesc = advanceSort(m.sortField, m.sortDesc)
		m.keys.Sort = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", sortLabel(m.sortField, m.sortDesc)))
		m.footer.SetKeyMap(m.keys)
		m.applyMRFilter()
		m.saveState()
	case key.Matches(msg, m.keys.ToggleView):
		if m.viewMode == ViewMine {
			m.viewMode = ViewAll
			m.header.SetTitle("mrboard")
			m.keys.ToggleView = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "my view"))
		} else {
			m.viewMode = ViewMine
			m.header.SetTitle("mrboard — @" + m.currentUser)
			m.keys.ToggleView = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "team view"))
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
	case key.Matches(msg, m.keys.Filter):
		authors, reviewers := uniqueAuthorsReviewers(m.allMRs)
		m.filterPopup = newFilterPopupWidget(
			m.styles, m.filterKeys, authors, reviewers, m.userMap,
			m.filterPhases, m.currentUser, m.filterAuthors, m.filterReviewers,
		)
		m.showFilter = true
		return m, nil
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

func (m *Model) openThemePicker() {
	names, err := AllThemeNames()
	if err != nil {
		m.logger.Error("theme: list theme names", "err", err)
		names = []string{m.themeName}
	}
	m.themePicker = newThemePickerWidget(names, m.themeName, m.themeMode, m.styles, m.themePickerKeys)
	m.showThemePicker = true
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
	base := m.baseCtx
	projectID := int64(mr.ProjectID)
	mrIID := int64(mr.IID)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, fetchTimeout)
		defer cancel()
		desc, threads, err := src.GetDetail(ctx, projectID, mrIID)
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
		board := m.renderBoard()
		if m.isRefreshing {
			overlay := m.styles.PopupBorder.Render(m.sp.spinner.View() + " Loading…")
			return m.renderWithOverlay(board, overlay)
		}
		if m.showFilter {
			return m.renderWithOverlay(board, m.filterPopup.render())
		}
		if m.showThemePicker {
			return m.renderWithOverlay(board, m.themePicker.render())
		}
		return board
	}
	return ""
}

func (m Model) renderBoard() string {
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

// renderWithOverlay composites popup centered over the board background.
func (m Model) renderWithOverlay(board, popup string) string {
	bg := lip.Place(m.width, m.height, lip.Left, lip.Top, board)
	popupW := lip.Width(popup)
	popupH := lip.Height(popup)
	x := (m.width - popupW) / 2  //nolint:mnd
	y := (m.height - popupH) / 2 //nolint:mnd
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	bgLayer := lip.NewLayer(bg)
	popupLayer := lip.NewLayer(popup).X(x).Y(y).Z(1)
	return lip.NewCompositor(bgLayer, popupLayer).Render()
}

// applyMRFilter applies all active filters and sort, then pushes the result into the board.
// applyTheme regenerates all styles from the current theme and dark-mode flag,
// then propagates them to all widgets.
func (m *Model) applyTheme() {
	m.styles = NewStyles(m.theme, m.hasDarkBg)
	m.header.SetStyles(m.styles)
	m.board.SetStyles(m.styles)
	m.footer.SetStyles(m.styles)
	m.detail.SetStyles(m.styles)
}

func (m *Model) applyMRFilter() {
	m.userMap = mrsvc.BuildUserMap(m.allMRs)
	mrs := mrsvc.FilterAndSort(m.allMRs, mrsvc.FilterOptions{
		MyView:      m.viewMode == ViewMine,
		CurrentUser: m.currentUser,
		SortField:   m.sortField.stateKey(),
		SortDesc:    m.sortDesc,
		Phases:      m.filterPhases,
		Authors:     m.filterAuthors,
		Reviewers:   m.filterReviewers,
	})
	displayMRs := visibleMRs(mrs, m.currentUser)
	m.board.SetMRs(displayMRs)
	m.header.SetMRs(displayMRs)
	m.header.SetFilterActive(m.isFilterActive())
}

// visibleMRs returns only the MRs that the board will display: those with at
// least one assigned reviewer, plus any MR authored by currentUser (so the
// current user's own drafts-without-reviewers are always visible).
func visibleMRs(mrs []domain.MergeRequest, currentUser string) []domain.MergeRequest {
	out := make([]domain.MergeRequest, 0, len(mrs))
	for _, mr := range mrs {
		if hasAssignedReviewer(mr) || mr.Author == currentUser {
			out = append(out, mr)
		}
	}
	return out
}

func (m *Model) isFilterActive() bool {
	return len(m.filterPhases) > 0 || len(m.filterAuthors) > 0 || len(m.filterReviewers) > 0
}

func (m *Model) saveState() {
	if err := m.store.Save(State{
		SortField: m.sortField.stateKey(),
		SortDesc:  m.sortDesc,
		ViewMode:  m.viewMode,
		ThemeName: m.themeName,
		ThemeMode: m.themeMode,
	}); err != nil {
		m.logger.Error("statestore: save failed", "err", err)
	}
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
