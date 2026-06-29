package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/toast"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/jirasvc"
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
	jiraFetchTimeout   = 30 * time.Second
	toastWidth         = 50
	toastMinWidth      = 30
	toastQueueDepth    = 16
	toastDuration      = 4 * time.Second
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

// DiffFetchResultMsg carries the MRDiff (refs + file list) for a single MR.
type DiffFetchResultMsg struct {
	ProjectID int
	MRIID     int
	Diff      domain.MRDiff
	Err       error
}

// FileRenderResultMsg carries the pre-rendered lines for a single diff file.
type FileRenderResultMsg struct {
	ProjectID int
	MRIID     int
	FileIdx   int
	Lines     []string
	Err       error
}

// NotifyResultMsg carries the result of a webhook notification attempt.
type NotifyResultMsg struct {
	Err error
}

// JiraIssueTypeMsg carries the result of a background JIRA issue type fetch.
type JiraIssueTypeMsg struct {
	IssueKey  string
	IssueType string // "" on error or not found
	Err       error
}

// TeamResolvedMsg carries the result of resolving team usernames to domain.Users at startup.
type TeamResolvedMsg struct {
	Roster           []domain.User
	InvalidUsernames []string // usernames that could not be resolved
	Err              error
}

// Options are session-scoped overrides passed via CLI flags.
// They are not persisted to the state file.
type Options struct {
	ThemeOverride string // --theme flag; "" means use state
	ModeOverride  string // --mode flag; "" means use state
}

// Model is the root Bubble Tea model for mrboard.
type Model struct {
	state              appState
	header             headerWidget
	board              boardWidget
	footer             footerWidget
	sp                 spinnerWidget
	detail             detailWidget
	showDetail         bool
	settings           settingsWidget
	reviewerEditor     *reviewerEditorWidget
	diffView           diffViewWidget
	diffViewKeys       DiffViewKeyMap
	overlay            overlayRouter
	keys               KeyMap
	detailKeys         DetailKeyMap
	settingsKeys       SettingsKeyMap
	reviewerEditorKeys ReviewerEditorKeyMap
	styles             Styles
	theme              theme.Theme[ColorKey]
	themeName          string // currently active theme name
	themeMode          string // "auto", "dark", "light"
	hasDarkBg          bool
	width              int
	height             int
	errors             []error
	errMsg             string
	cfg                *config.Config
	src                mrsvc.MergeRequestSource
	store              domain.StateStore
	allMRs             []domain.MergeRequest
	userMap            map[string]string
	currentUser        string
	viewMode           domain.ViewMode
	sortField          sortField
	sortDesc           bool
	filter             domain.FilterCriteria
	includeReviewerMRs bool
	reviewerMRsInStore bool // true once allMRs contains reviewer-source MRs
	fetchCancel        context.CancelFunc
	baseCtx            context.Context
	logger             *slog.Logger
	isRefreshing       bool
	prevFocusMR        *domain.MergeRequest // saved before refresh for focus restoration
	notifier           domain.Notifier
	alerts             toast.Model
	jiraBaseURL        string
	jiraEnricher       jirasvc.JiraEnricher  // nil when JIRA is not configured
	iconResolver       IssueTypeIconResolver // maps JIRA issue type names to emoji
	teamRoster         []domain.User         // resolved once at startup from type:user sources
}

// New creates a ready-to-run mrboard model. It loads persisted UI state from
// store; on error it logs and falls back to DefaultState().
func New(
	ctx context.Context,
	cfg *config.Config,
	src mrsvc.MergeRequestSource,
	store domain.StateStore,
	notifier domain.Notifier,
	jiraEnricher jirasvc.JiraEnricher,
	version string,
	opts Options,
) Model {
	logger := ilog.FromContext(ctx)

	st, err := store.Load()
	if err != nil {
		logger.Error("statestore: load failed, using defaults", "err", err)
		st = domain.DefaultAppState()
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
	if cfg.LifetimeWarnAfter > 0 {
		styles.LifetimeWarn = cfg.LifetimeWarnAfter
	}
	if cfg.LifetimeErrorAfter > 0 {
		styles.LifetimeError = cfg.LifetimeErrorAfter
	}
	keys := DefaultKeyMap

	sf := sortFieldFromState(st.SortField)
	keys.Sort = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", sortLabel(sf, st.SortDesc)))

	viewMode := st.ViewMode
	if viewMode == domain.ViewMine && cfg.CurrentUser == "" {
		viewMode = domain.ViewAll
	}
	if viewMode == domain.ViewMine {
		keys.ToggleView = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "team view"))
	}
	if cfg.CurrentUser == "" {
		keys.ToggleView.SetEnabled(false)
	}
	if notifier == nil {
		keys.Notify.SetEnabled(false)
	}
	keys.Jira.SetEnabled(false) // enabled dynamically when focused MR has a JIRA ID

	m := Model{
		state:              stateLoading,
		header:             newHeaderWidget(styles),
		board:              newBoardWidget(styles, defaultBoardWidth, defaultBoardHeight-chromeHeight),
		footer:             newFooterWidget(keys, styles, version),
		sp:                 newSpinnerWidget(),
		detail:             newDetailWidget(styles),
		keys:               keys,
		detailKeys:         DefaultDetailKeyMap,
		settingsKeys:       DefaultSettingsKeyMap,
		reviewerEditorKeys: DefaultReviewerEditorKeyMap,
		diffViewKeys:       DefaultDiffViewKeyMap,
		diffView:           newDiffViewWidget(styles),
		styles:             styles,
		theme:              th,
		themeName:          themeName,
		themeMode:          themeMode,
		hasDarkBg:          initialDark,
		cfg:                cfg,
		src:                src,
		store:              store,
		currentUser:        cfg.CurrentUser,
		viewMode:           viewMode,
		sortField:          sf,
		sortDesc:           st.SortDesc,
		filter:             st.Filter,
		includeReviewerMRs: st.IncludeReviewerMRs,
		baseCtx:            ctx,
		logger:             logger,
		notifier:           notifier,
		jiraBaseURL:        cfg.Jira.InstanceURL,
		jiraEnricher:       jiraEnricher,
		iconResolver:       NewIssueTypeIconResolver(cfg.Jira.IssueTypeIcons),
		alerts: toast.New(toastWidth, toast.FontUnicode, toastDuration).
			WithPosition(toast.TopRight).
			WithMinWidth(toastMinWidth).
			WithQueueDepth(toastQueueDepth),
	}
	if viewMode == domain.ViewMine {
		m.header.SetTitle("mrboard — @" + cfg.CurrentUser)
	}
	logger.Info("tui: starting", "version", version, "theme", themeName, "mode", themeMode, "view", int(viewMode))
	return m
}

// Init starts the spinner, fires the first data fetch, schedules the minute ticker,
// and resolves team usernames from type:user sources.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.sp.Init(),
		makeFetchCmd(m.baseCtx, m.src, m.includeReviewerMRs),
		tickCmd(),
		makeResolveTeamCmd(m.baseCtx, m.src, m.cfg),
	)
}

// makeFetchCmd returns a Cmd that fetches all MRs and a cancel func to abort it.
// The cancel is also called via defer inside the goroutine once the fetch finishes.
func makeFetchCmd(base context.Context, src mrsvc.MergeRequestSource, includeReviewerMRs bool) tea.Cmd {
	ctx, cancel := context.WithTimeout(base, fetchTimeout)
	return func() tea.Msg {
		defer cancel()
		mrs, errs := src.FetchAll(ctx, mrsvc.FetchOptions{IncludeReviewerMRs: includeReviewerMRs})
		return FetchResultMsg{MRs: mrs, Errors: errs}
	}
}

// makeResolveTeamCmd resolves team usernames (from type:user sources) to GitLab user IDs.
// Returns nil if there are no user-type sources (empty team is valid).
func makeResolveTeamCmd(base context.Context, src mrsvc.MergeRequestSource, cfg *config.Config) tea.Cmd {
	var usernames []string
	for _, s := range cfg.Sources {
		if s.Type == "user" {
			usernames = append(usernames, s.IDs...)
		}
	}
	if len(usernames) == 0 {
		return nil
	}
	return func() tea.Msg {
		roster, err := src.ResolveUsers(base, usernames)
		if err != nil {
			return TeamResolvedMsg{Err: err}
		}
		resolvedByUsername := make(map[string]bool, len(roster))
		for _, u := range roster {
			resolvedByUsername[u.Username] = true
		}
		var invalid []string
		for _, name := range usernames {
			if !resolvedByUsername[name] {
				invalid = append(invalid, name)
			}
		}
		return TeamResolvedMsg{Roster: roster, InvalidUsernames: invalid}
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
	includeReviewerMRs := m.includeReviewerMRs
	return func() tea.Msg {
		defer cancel()
		mrs, errs := src.FetchAll(ctx, mrsvc.FetchOptions{IncludeReviewerMRs: includeReviewerMRs})
		return FetchResultMsg{MRs: mrs, Errors: errs}
	}
}

// Update handles all incoming messages, driving toast alert animation for every tick.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	result, cmd := m.coreUpdate(msg)
	rm := result.(Model)

	var alertCmd tea.Cmd
	rm.alerts, alertCmd = rm.alerts.Update(msg)

	return rm, tea.Batch(cmd, alertCmd)
}

// toast returns a Cmd that triggers a toast notification popup.
func (m Model) toast(def toast.AlertSpec, text string) tea.Cmd {
	return m.alerts.NewAlertCmd(def, text)
}

// coreUpdate is the main message dispatch logic.
func (m Model) coreUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.reviewerMRsInStore = hasReviewerSourceMR(msg.MRs)
		m.logger.Info("tui: fetch result", "mrs", len(msg.MRs), "errors", len(msg.Errors))
		for _, e := range msg.Errors {
			m.logger.Warn("tui: fetch partial error", "error", e)
		}
		m.applyTheme()
		m.applyMRFilter()
		if m.prevFocusMR != nil {
			m.board.TryRestoreFocus(int(m.prevFocusMR.Phase), m.prevFocusMR.IID)
			m.prevFocusMR = nil
		}
		m.updateJiraKey()
		return m, m.makeJiraEnrichCmds()

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

	case DiffFetchResultMsg:
		return m.handleDiffFetchResult(msg)

	case FileRenderResultMsg:
		return m.handleFileRenderResult(msg)

	case SettingsAppliedMsg:
		return m.handleSettingsApplied(msg)

	case SettingsClosedMsg:
		m.overlay.closeOverlay()
		return m, nil

	case MembersLoadedMsg:
		return m.handleMembersLoaded(msg)

	case ReviewerEditorClosedMsg:
		m.overlay.closeOverlay()
		return m, nil

	case ReviewersSavedMsg:
		return m.handleReviewersSaved(msg)

	case NotifyResultMsg:
		return m.handleNotifyResult(msg)

	case JiraIssueTypeMsg:
		return m.handleJiraIssueType(msg)

	case TeamResolvedMsg:
		return m.handleTeamResolved(msg)

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

	switch m.overlay.active() {
	case overlayKindSettings:
		updated, cmd := m.settings.Update(msg)
		m.settings = updated.(settingsWidget)
		return m, cmd
	case overlayKindReviewerEditor:
		if m.reviewerEditor != nil {
			updated, cmd := m.reviewerEditor.Update(msg)
			m.reviewerEditor = updated.(*reviewerEditorWidget)
			return m, cmd
		}
	case overlayKindDiffView:
		return m.handleKeyDiff(msg)
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
	case key.Matches(msg, m.keys.Diff):
		if mr := m.board.FocusedMR(); mr != nil {
			m.openDiffView(mr)
			return m, m.fetchDiffCmd(mr)
		}
	}
	return m, nil
}

// handleKeyDiff handles keys while the diff view owns focus.
func (m Model) handleKeyDiff(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.diffViewKeys.Close):
		m.closeDiffView()
	case key.Matches(msg, m.diffViewKeys.PrevFile):
		m.diffView.PrevFile()
		return m, (&m).maybeRenderCurrentFile()
	case key.Matches(msg, m.diffViewKeys.NextFile):
		m.diffView.NextFile()
		return m, (&m).maybeRenderCurrentFile()
	case key.Matches(msg, m.diffViewKeys.ScrollUp):
		m.diffView.ScrollUp()
	case key.Matches(msg, m.diffViewKeys.ScrollDown):
		m.diffView.ScrollDown()
	case key.Matches(msg, m.diffViewKeys.HalfPageUp):
		m.diffView.HalfPageUp()
	case key.Matches(msg, m.diffViewKeys.HalfPageDown):
		m.diffView.HalfPageDown()
	case key.Matches(msg, m.diffViewKeys.Top):
		m.diffView.ScrollToTop()
	case key.Matches(msg, m.diffViewKeys.Bottom):
		m.diffView.ScrollToBottom()
	case key.Matches(msg, m.diffViewKeys.Open):
		if m.diffView.mr != nil {
			return m, openBrowser(m.diffView.mr.WebURL)
		}
	}
	return m, nil
}

// handleKeyBoard handles keys while the kanban board owns focus.
func (m Model) handleKeyBoard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.board.MoveUp()
		m.updateJiraKey()
	case key.Matches(msg, m.keys.Down):
		m.board.MoveDown()
		m.updateJiraKey()
	case key.Matches(msg, m.keys.Left):
		m.board.MoveLeft()
		m.updateJiraKey()
	case key.Matches(msg, m.keys.Right):
		m.board.MoveRight()
		m.updateJiraKey()
	case key.Matches(msg, m.keys.Refresh):
		if m.showDetail {
			m.closeDetail()
		}
		m.prevFocusMR = m.board.FocusedMR()
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
		if m.viewMode == domain.ViewMine {
			m.viewMode = domain.ViewAll
			m.header.SetTitle("mrboard")
			m.keys.ToggleView = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "my view"))
		} else {
			m.viewMode = domain.ViewMine
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
	case key.Matches(msg, m.keys.Settings):
		m.openSettings()
		return m, nil
	case key.Matches(msg, m.keys.Reviewers):
		if mr := m.board.FocusedMR(); mr != nil {
			m.reviewerEditor = newReviewerEditorWidget(
				m.baseCtx, *mr, m.styles, m.reviewerEditorKeys, m.src, m.teamRoster,
			)
			m.overlay.openOverlay(overlayKindReviewerEditor)
			return m, nil
		}
	case key.Matches(msg, m.keys.Diff):
		if mr := m.board.FocusedMR(); mr != nil {
			m.openDiffView(mr)
			return m, m.fetchDiffCmd(mr)
		}
	case key.Matches(msg, m.keys.Notify):
		if mr := m.board.FocusedMR(); mr != nil && m.notifier != nil {
			m.logger.Info("tui: notify key pressed", "mr_iid", mr.IID, "mr_title", mr.Title)
			return m, m.notifyCmd(mr)
		}
	case key.Matches(msg, m.keys.Jira):
		if mr := m.board.FocusedMR(); mr != nil {
			if url := domain.JiraIssueURL(m.jiraBaseURL, domain.ExtractJiraID(mr.Title)); url != "" {
				return m, openBrowser(url)
			}
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

func (m *Model) openDiffView(mr *domain.MergeRequest) {
	m.overlay.openOverlay(overlayKindDiffView)
	m.diffView.SetMR(mr)
	bodyH := m.height - chromeHeight
	m.diffView.SetSize(m.width, bodyH)
	m.header.SetTitle(fmt.Sprintf("diff !%d – %s", mr.IID, mr.Title))
	m.header.SetStats("loading…")
	m.footer.SetKeyMap(m.diffViewKeys)
}

func (m *Model) closeDiffView() {
	m.overlay.closeOverlay()
	m.header.SetTitle("mrboard")
	m.header.SetStats("")
	if m.showDetail {
		m.footer.SetKeyMap(m.detailKeys)
	} else {
		m.footer.SetKeyMap(m.keys)
	}
}

func (m Model) fetchDiffCmd(mr *domain.MergeRequest) tea.Cmd {
	src := m.src
	base := m.baseCtx
	projectID := int64(mr.ProjectID)
	mrIID := int64(mr.IID)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, fetchTimeout)
		defer cancel()
		diff, err := src.GetDiff(ctx, projectID, mrIID)
		return DiffFetchResultMsg{
			ProjectID: int(projectID),
			MRIID:     int(mrIID),
			Diff:      diff,
			Err:       err,
		}
	}
}

// fetchFileRenderCmd fetches old+new file content and runs difft (or fallback) asynchronously.
func (m Model) fetchFileRenderCmd(fileIdx int) tea.Cmd {
	src := m.src
	base := m.baseCtx
	mr := m.diffView.mr
	if mr == nil || fileIdx >= len(m.diffView.files) {
		return nil
	}
	projectID := int64(mr.ProjectID)
	mrIID := int64(mr.IID)
	f := m.diffView.files[fileIdx]
	baseSHA := m.diffView.baseSHA
	headSHA := m.diffView.headSHA
	width := m.diffView.diffPaneWidth()
	styles := m.diffView.styles

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, fetchTimeout)
		defer cancel()

		var oldContent, newContent []byte
		if !f.NewFile && baseSHA != "" {
			//nolint:errcheck // failure → nil content → difft fallback path handles it
			oldContent, _ = src.GetFileContent(ctx, projectID, f.OldPath, baseSHA)
		}
		if !f.DeletedFile && headSHA != "" {
			//nolint:errcheck // failure → nil content → difft fallback path handles it
			newContent, _ = src.GetFileContent(ctx, projectID, f.NewPath, headSHA)
		}

		var lines []string
		if difftBin != "" {
			var err error
			lines, err = runDifft(oldContent, newContent, f.OldPath, f.NewPath, width)
			if err != nil {
				lines = nil
			}
		}
		// Fallback: colorize the unified diff string from GitLab.
		if lines == nil {
			w := newDiffViewWidget(styles)
			lines = w.colorizedLines(f.Diff, width)
		}
		return FileRenderResultMsg{
			ProjectID: int(projectID),
			MRIID:     int(mrIID),
			FileIdx:   fileIdx,
			Lines:     lines,
		}
	}
}

// maybeRenderCurrentFile dispatches a render cmd for the current file if it hasn't
// been rendered yet. For the fallback path (no difft), rendering is synchronous.
func (m *Model) maybeRenderCurrentFile() tea.Cmd {
	if !m.overlay.isDiffView() || len(m.diffView.files) == 0 {
		return nil
	}
	idx := m.diffView.fileIdx
	if m.diffView.HasRendered(idx) || m.diffView.IsRendering(idx) {
		return nil
	}
	if m.diffView.files[idx].TooLarge {
		return nil
	}
	if difftBin == "" {
		// Synchronous fallback — no async cmd needed.
		m.diffView.RenderFallback(idx)
		return nil
	}
	m.diffView.SetRendering(idx)
	return m.fetchFileRenderCmd(idx)
}

func (m Model) renderDiffScreen() string {
	headerStr := m.header.render()
	footerStr := m.footer.render()
	bodyH := m.height - chromeHeight
	m.diffView.SetSize(m.width, bodyH)
	body := m.diffView.render()
	return headerStr + "\n" + body + "\n" + footerStr
}

func (m *Model) openSettings() {
	themes, err := AllThemeNames()
	if err != nil {
		m.logger.Error("theme: list theme names", "err", err)
		themes = []string{m.themeName}
	}
	authors, reviewers := BuildAuthorsReviewers(m.allMRs)
	m.settings = newSettingsWidget(
		themes,
		authors, reviewers,
		m.userMap,
		m.filter,
		m.includeReviewerMRs,
		m.sortField,
		m.sortDesc,
		m.themeName, m.themeMode,
		m.styles,
		m.settingsKeys,
	)
	m.overlay.openOverlay(overlayKindSettings)
}

// handleSettingsApplied applies all live changes from the settings panel.
func (m Model) handleSettingsApplied(msg SettingsAppliedMsg) (tea.Model, tea.Cmd) {
	m.filter = msg.Filter
	reviewerFetchNeeded := msg.IncludeReviewerMRs && !m.includeReviewerMRs && !m.reviewerMRsInStore
	m.includeReviewerMRs = msg.IncludeReviewerMRs

	sortChanged := m.sortField.stateKey() != msg.SortField || m.sortDesc != msg.SortDesc
	m.sortField = sortFieldFromState(msg.SortField)
	m.sortDesc = msg.SortDesc
	if sortChanged {
		m.keys.Sort = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", sortLabel(m.sortField, m.sortDesc)))
		m.footer.SetKeyMap(m.keys)
	}

	themeChanged := m.themeName != msg.ThemeName || m.themeMode != msg.ThemeMode
	if themeChanged {
		m.themeName = msg.ThemeName
		m.themeMode = msg.ThemeMode
		switch msg.ThemeMode {
		case themeModeDark:
			m.hasDarkBg = true
		case themeModeLight:
			m.hasDarkBg = false
		}
		m.theme = LoadThemeByName(msg.ThemeName)
		m.applyTheme()
	}

	m.applyMRFilter()
	m.saveState()

	if reviewerFetchNeeded {
		m.isRefreshing = true
		return m, tea.Batch(m.sp.Init(), m.startFetch())
	}
	return m, nil
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
	if m.overlay.isDiffView() {
		m.diffView.SetSize(m.width, m.height-chromeHeight)
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

func (m Model) notifyCmd(mr *domain.MergeRequest) tea.Cmd {
	notifier := m.notifier
	base := m.baseCtx
	snapshot := *mr
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, fetchTimeout)
		defer cancel()
		return NotifyResultMsg{Err: notifier.Notify(ctx, snapshot)}
	}
}

func (m Model) handleNotifyResult(msg NotifyResultMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.logger.Error("tui: notification failed", "err", msg.Err)
		m.errors = append(m.errors, fmt.Errorf("notify: %w", msg.Err))
		return m, m.toast(toast.ErrorAlert, "Notify failed")
	}
	m.logger.Info("tui: notification delivered")
	return m, m.toast(toast.InfoAlert, "Teams notified ✓")
}

// updateJiraKey enables or disables the Jira key based on whether the focused
// MR has a detectable JIRA ID and jiraBaseURL is configured. Call after any
// navigation that may change the focused card.
func (m *Model) updateJiraKey() {
	mr := m.board.FocusedMR()
	enabled := m.jiraBaseURL != "" && mr != nil && domain.ExtractJiraID(mr.Title) != ""
	m.keys.Jira.SetEnabled(enabled)
	m.footer.SetKeyMap(m.keys)
}

// View renders the full screen. Only the root model sets AltScreen.
func (m Model) View() tea.View {
	content := m.alerts.Render(m.renderContent())
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
		if m.overlay.isDiffView() {
			return m.renderDiffScreen()
		}
		board := m.renderBoard()
		if m.isRefreshing {
			spinner := m.styles.PopupBorder.Render(m.sp.spinner.View() + " Loading…")
			return m.renderWithOverlay(board, spinner)
		}
		switch m.overlay.active() {
		case overlayKindSettings:
			return m.renderWithOverlay(board, m.settings.render())
		case overlayKindReviewerEditor:
			if m.reviewerEditor != nil {
				return m.renderWithOverlay(board, m.reviewerEditor.render())
			}
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

// handleMembersLoaded forwards the members list to the reviewer editor if it is open.
func (m Model) handleMembersLoaded(msg MembersLoadedMsg) (tea.Model, tea.Cmd) {
	if m.reviewerEditor != nil {
		m.reviewerEditor.SetMembers(msg.Members, msg.Err)
	}
	return m, nil
}

// handleReviewersSaved closes the editor, updates the MR in-place, and fires
// a Teams notification automatically if a notifier is configured.
func (m Model) handleReviewersSaved(msg ReviewersSavedMsg) (tea.Model, tea.Cmd) {
	m.overlay.closeOverlay()
	if msg.Err != nil {
		m.logger.Error("tui: reviewers save failed", "err", msg.Err)
		m.errors = append(m.errors, msg.Err)
		return m, m.toast(toast.ErrorAlert, "Save failed")
	}
	updatedMR := msg.MR
	for i, mr := range m.allMRs {
		if mr.ProjectID == updatedMR.ProjectID && mr.IID == updatedMR.IID {
			m.allMRs[i] = updatedMR
			break
		}
	}
	m.applyMRFilter()
	m.updateJiraKey()

	cmds := []tea.Cmd{m.toast(toast.InfoAlert, "Reviewers saved")}
	if m.notifier != nil {
		cmds = append(cmds, m.notifyCmd(&updatedMR))
	}
	return m, tea.Batch(cmds...)
}

// handleTeamResolved caches the resolved team roster and surfaces any feedback.
func (m Model) handleTeamResolved(msg TeamResolvedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.logger.Error("tui: team resolution failed", "err", msg.Err)
		return m, m.toast(toast.ErrorAlert, "Team resolution failed: "+msg.Err.Error())
	}
	m.teamRoster = msg.Roster
	m.logger.Info("tui: team resolved", "count", len(msg.Roster), "invalid", len(msg.InvalidUsernames))
	if len(msg.InvalidUsernames) > 0 {
		m.logger.Warn("tui: team: unknown usernames", "usernames", msg.InvalidUsernames)
		return m, m.toast(toast.WarnAlert, "Unknown team members: "+strings.Join(msg.InvalidUsernames, ", "))
	}
	return m, nil
}

// handleJiraIssueType stores a freshly fetched JIRA issue type on the matching MR(s).
func (m Model) handleJiraIssueType(msg JiraIssueTypeMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.logger.Warn("tui: jira fetch failed", "key", msg.IssueKey, "err", msg.Err)
		return m, nil
	}
	for i := range m.allMRs {
		if domain.ExtractJiraID(m.allMRs[i].Title) == msg.IssueKey {
			m.allMRs[i].JiraIssueType = msg.IssueType
		}
	}
	m.applyMRFilter()
	return m, nil
}

// makeJiraEnrichCmds returns one fetch command per unique JIRA issue key found
// in allMRs. Returns nil when jiraEnricher is nil or no keys are found.
func (m *Model) makeJiraEnrichCmds() tea.Cmd {
	if m.jiraEnricher == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var cmds []tea.Cmd
	for _, mr := range m.allMRs {
		if issueKey := domain.ExtractJiraID(mr.Title); issueKey != "" {
			if _, ok := seen[issueKey]; !ok {
				seen[issueKey] = struct{}{}
				cmds = append(cmds, makeJiraFetchCmd(m.baseCtx, m.jiraEnricher, issueKey))
			}
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// makeJiraFetchCmd returns a Cmd that calls GetIssueType for issueKey and
// wraps the result in a JiraIssueTypeMsg.
func makeJiraFetchCmd(base context.Context, enricher jirasvc.JiraEnricher, issueKey string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, jiraFetchTimeout)
		defer cancel()
		issueType, err := enricher.GetIssueType(ctx, issueKey)
		return JiraIssueTypeMsg{IssueKey: issueKey, IssueType: issueType, Err: err}
	}
}

// handleDiffFetchResult handles DiffFetchResultMsg: stores the MRDiff and triggers file 0 render.
func (m Model) handleDiffFetchResult(msg DiffFetchResultMsg) (tea.Model, tea.Cmd) {
	if !m.overlay.isDiffView() || m.diffView.mr == nil ||
		m.diffView.mr.ProjectID != msg.ProjectID || m.diffView.mr.IID != msg.MRIID {
		return m, nil
	}
	if msg.Err != nil {
		m.diffView.loading = false
		return m, nil
	}
	m.diffView.SetDiff(msg.Diff)
	added, removed := diffStats(msg.Diff.Files)
	m.header.SetStats(fmt.Sprintf("%d files  +%d -%d", len(msg.Diff.Files), added, removed))
	cmd := (&m).maybeRenderCurrentFile()
	return m, cmd
}

// handleFileRenderResult stores rendered lines in the diff view cache.
func (m Model) handleFileRenderResult(msg FileRenderResultMsg) (tea.Model, tea.Cmd) {
	if !m.overlay.isDiffView() || m.diffView.mr == nil ||
		m.diffView.mr.ProjectID != msg.ProjectID || m.diffView.mr.IID != msg.MRIID {
		return m, nil
	}
	m.diffView.SetRendered(msg.FileIdx, msg.Lines)
	return m, nil
}

// applyTheme regenerates all styles from the current theme and dark-mode flag,
// then propagates them to all widgets including open overlays.
func (m *Model) applyTheme() {
	m.styles = NewStyles(m.theme, m.hasDarkBg)
	if m.cfg.LifetimeWarnAfter > 0 {
		m.styles.LifetimeWarn = m.cfg.LifetimeWarnAfter
	}
	if m.cfg.LifetimeErrorAfter > 0 {
		m.styles.LifetimeError = m.cfg.LifetimeErrorAfter
	}
	m.header.SetStyles(m.styles)
	m.board.SetStyles(m.styles)
	m.footer.SetStyles(m.styles)
	m.detail.SetStyles(m.styles)
	m.diffView.SetStyles(m.styles)
	m.settings.styles = m.styles
	if m.reviewerEditor != nil {
		m.reviewerEditor.styles = m.styles
	}
}

func (m *Model) applyMRFilter() {
	m.userMap = mrsvc.BuildUserMap(m.allMRs)
	src := m.allMRs
	if !m.includeReviewerMRs {
		filtered := make([]domain.MergeRequest, 0, len(src))
		for _, mr := range src {
			if !mr.ReviewerSource {
				filtered = append(filtered, mr)
			}
		}
		src = filtered
	}
	mrs := mrsvc.FilterAndSort(src, mrsvc.FilterOptions{
		MyView:      m.viewMode == domain.ViewMine,
		CurrentUser: m.currentUser,
		SortField:   m.sortField.stateKey(),
		SortDesc:    m.sortDesc,
		Phases:      m.filter.Phases,
		Authors:     m.filter.Authors,
		Reviewers:   m.filter.Reviewers,
	})
	displayMRs := visibleMRs(mrs, m.currentUser)
	m.board.SetMRs(displayMRs)
	m.header.SetMRs(displayMRs)
	m.header.SetFilterActive(m.isFilterActive())
}

func visibleMRs(mrs []domain.MergeRequest, _ string) []domain.MergeRequest {
	return mrs
}

func (m *Model) isFilterActive() bool {
	return len(m.filter.Phases) > 0 || len(m.filter.Authors) > 0 || len(m.filter.Reviewers) > 0
}

func (m *Model) saveState() {
	if err := m.store.Save(domain.AppState{
		SortField:          m.sortField.stateKey(),
		SortDesc:           m.sortDesc,
		ViewMode:           m.viewMode,
		ThemeName:          m.themeName,
		ThemeMode:          m.themeMode,
		Filter:             m.filter,
		IncludeReviewerMRs: m.includeReviewerMRs,
	}); err != nil {
		m.logger.Error("statestore: save failed", "err", err)
	}
}

func hasReviewerSourceMR(mrs []domain.MergeRequest) bool {
	for _, mr := range mrs {
		if mr.ReviewerSource {
			return true
		}
	}
	return false
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
