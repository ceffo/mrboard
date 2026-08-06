package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/toast"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/ticketsvc"
	ilog "github.com/ceffo/mrboard/internal/log"
	"github.com/ceffo/mrboard/pkg/theme"
)

// sortField identifies which MR attribute to sort by.
type sortField int

const (
	sortByRepoIID sortField = iota
	sortByAssignee
	sortByAge
	numSortFields
)

// Sort field string keys used in persisted state.
const (
	sortKeyRepoIID  = "repo_iid"
	sortKeyAssignee = "assignee"
	sortKeyAge      = "age"
)

func (f sortField) next() sortField { return (f + 1) % numSortFields }

func (f sortField) display() string {
	switch f {
	case sortByAssignee:
		return sortKeyAssignee
	case sortByAge:
		return sortKeyAge
	default:
		return "repo·id"
	}
}

func (f sortField) stateKey() string {
	switch f {
	case sortByAssignee:
		return sortKeyAssignee
	case sortByAge:
		return sortKeyAge
	default:
		return sortKeyRepoIID
	}
}

func sortFieldFromState(s string) sortField {
	switch s {
	case sortKeyAssignee:
		return sortByAssignee
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
	return field.display() + dir
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
	ticketFetchTimeout = 30 * time.Second
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

// refreshTickMsg fires the recurring background auto-refresh (docs/adr/0005,
// "Refresh cadence") — distinct from tickMsg above, which only re-renders
// duration strings and never triggers a fetch.
type refreshTickMsg struct{ gen int }

// refreshTickCmd schedules the next auto-refresh tick. gen ties the tick to
// the schedule that produced it: resetRefreshTimer bumps Model.refreshGen on
// a manual refresh, so a tick from a schedule that predates it arrives with a
// stale gen and is dropped instead of firing early (see handleRefreshTick).
// interval <= 0 disables auto-refresh entirely.
func refreshTickCmd(interval time.Duration, gen int) tea.Cmd {
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return refreshTickMsg{gen: gen}
	})
}

// FetchResultMsg carries the result of a successful (or partial) fetch.
// FetchStartedAt is stamped when the fetch was dispatched (not when it
// returned); the dirty-set guard compares it against each entry in
// Model.dirty to tell a stale landing snapshot from a confirming one
// (docs/adr/0005, "The write race that ungating creates").
//
// Seq is the value of Model.fetchSeq at dispatch time. startFetch cancels any
// fetch already in flight, and a cancelled fetch returns almost immediately
// with a partial result plus context.Canceled errors — Cmd goroutines are not
// ordered, so that degraded result can land after the newer fetch's good one.
// Update drops any result whose Seq no longer matches Model.fetchSeq. The boot
// fetch from makeFetchCmd leaves Seq at 0, matching fetchSeq's zero value.
type FetchResultMsg struct {
	MRs            []domain.MergeRequest
	Errors         []error
	FetchStartedAt time.Time
	Seq            int
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

// NotifyResultMsg carries the result of a webhook notification attempt.
type NotifyResultMsg struct {
	Err error
}

// CommandResultMsg carries the outcome of a configured external command run via
// tea.ExecProcess (docs/adr/0004-external-command-launcher.md). Err covers both
// a missing binary (*exec.Error) and a non-zero exit (*exec.ExitError) uniformly.
type CommandResultMsg struct {
	CommandName string
	Err         error
}

// TicketIssueTypeMsg carries the result of a background issue-type fetch.
type TicketIssueTypeMsg struct {
	IssueKey  string
	IssueType string // "" on error or not found
	Err       error
}

// SprintIssueKeysMsg carries the result of a background active-sprint key fetch.
type SprintIssueKeysMsg struct {
	Keys []string // nil when no active sprint exists
	Err  error
}

// TicketLinkResultMsg carries the result of a background UpsertRemoteLink call.
type TicketLinkResultMsg struct {
	IssueKey string
	GlobalID string
	Err      error
}

// TicketDescriptionLinkResultMsg carries the result of a background MR
// description back-link write — as opposed to TicketLinkResultMsg's remote
// link on the ticket tracker's own side.
type TicketDescriptionLinkResultMsg struct {
	ProjectID int
	MRIID     int
	IssueKey  string
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
	batchPreview       *batchPreviewWidget
	diffView           diffViewWidget
	overlay            overlayRouter
	showHelp           bool // '?' help modal open
	helpModal          helpModalWidget
	keys               *BoardKeyMap // points at DefaultBoardKeyMap, shared with BoardCtx
	detailKeys         DetailKeyMap
	settingsKeys       SettingsKeyMap
	reviewerEditorKeys ReviewerEditorKeyMap
	batchPreviewKeys   BatchPreviewKeyMap
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
	customCommandsCtx  *Context // user-configured external commands, see docs/adr/0004
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
	fetchSeq           int // bumped per dispatched fetch; see FetchResultMsg.Seq
	snapshotStore      domain.SnapshotStore
	snapshotWrittenAt  time.Time    // when the displayed data was captured; zero until a snapshot exists
	selected           domain.MRKey // single source of truth for board selection, see docs/adr/0005
	notifier           domain.Notifier
	alerts             toast.Model
	ticketBaseURL      string
	keyMatcher         domain.TicketKeyMatcher          // shared ticket-key extraction, see cfg.Jira
	ticketEnricher     ticketsvc.TicketEnricher         // nil when the issue tracker is not configured
	ticketLinker       ticketsvc.TicketLinker           // nil when the issue tracker is not configured
	iconResolver       IssueTypeIconResolver            // maps issue type names to emoji
	teamRoster         []domain.User                    // resolved once at startup from type:user sources
	sprintIssueKeys    map[string]bool                  // active sprint keys; nil when no active sprint
	sprintFilterActive bool                             // true when S-key sprint filter is toggled on
	ticketIndex        map[string][]domain.MergeRequest // MRs by extracted issue key; rebuilt on every allMRs change
	ticketDescLinked   map[ticketDescLinkKey]bool       // description back-link dedup, this session only
	dirty              map[domain.MRKey]time.Time       // locally-written MRs unconfirmed by a fetch, see docs/adr/0005
	refreshInterval    time.Duration                    // auto-refresh cadence; <= 0 disables it, see docs/adr/0005
	refreshGen         int                              // bumped on manual refresh to invalidate pending ticks
}

// New creates a ready-to-run mrboard model. It loads persisted UI state from
// store; on error it logs and falls back to DefaultState().
func New(
	ctx context.Context,
	cfg *config.Config,
	src mrsvc.MergeRequestSource,
	store domain.StateStore,
	snapStore domain.SnapshotStore,
	notifier domain.Notifier,
	ticketEnricher ticketsvc.TicketEnricher,
	ticketLinker ticketsvc.TicketLinker,
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
	// The model shares the package-level board keymap with BoardCtx so that
	// enablement changes are visible to dispatch, footer, and help modal
	// alike. Enablement is set both ways to stay deterministic across tests.
	keys := &DefaultBoardKeyMap
	keys.ToggleView.SetEnabled(cfg.CurrentUser != "")
	keys.Notify.SetEnabled(notifier != nil)
	keys.Sprint.SetEnabled(cfg.Jira.BoardID != 0)
	keys.OpenTicket.SetEnabled(false) // enabled dynamically when focused MR has a ticket ID

	sf := sortFieldFromState(st.SortField)

	viewMode := st.ViewMode
	if viewMode == domain.ViewMine && cfg.CurrentUser == "" {
		viewMode = domain.ViewAll
	}

	ir := NewIssueTypeIconResolver(cfg.Jira.IssueTypeIcons)
	km := domain.NewTicketKeyMatcher(cfg.Jira.CaseInsensitiveTicketMatch)

	m := Model{
		state:              stateLoading,
		header:             newHeaderWidget(styles),
		board:              newBoardWidget(styles, defaultBoardWidth, defaultBoardHeight-chromeHeight, ir, km),
		footer:             newFooterWidget(styles, version),
		helpModal:          newHelpModalWidget(styles),
		sp:                 newSpinnerWidget(),
		detail:             newDetailWidget(styles),
		keys:               keys,
		detailKeys:         DefaultDetailKeyMap,
		settingsKeys:       DefaultSettingsKeyMap,
		reviewerEditorKeys: DefaultReviewerEditorKeyMap,
		batchPreviewKeys:   DefaultBatchPreviewKeyMap,
		diffView:           newDiffViewWidget(ctx, styles, DefaultDiffViewKeyMap, src),
		styles:             styles,
		theme:              th,
		themeName:          themeName,
		themeMode:          themeMode,
		hasDarkBg:          initialDark,
		cfg:                cfg,
		customCommandsCtx:  BuildCustomCommandsContext(cfg.Commands),
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
		snapshotStore:      snapStore,
		notifier:           notifier,
		ticketBaseURL:      cfg.Jira.InstanceURL,
		keyMatcher:         km,
		ticketEnricher:     ticketEnricher,
		ticketLinker:       ticketLinker,
		ticketDescLinked:   make(map[ticketDescLinkKey]bool),
		dirty:              make(map[domain.MRKey]time.Time),
		refreshInterval:    cfg.RefreshInterval,
		iconResolver:       ir,
		alerts: toast.New(toastWidth, toast.FontUnicode, toastDuration).
			WithPosition(toast.TopRight).
			WithMinWidth(toastMinWidth).
			WithQueueDepth(toastQueueDepth),
	}
	if viewMode == domain.ViewMine {
		m.header.SetTitle("mrboard — @" + cfg.CurrentUser)
	}
	m.header.SetSort(sortLabel(sf, st.SortDesc))

	// Boot from the cached snapshot, at any age, so the board is interactive
	// immediately instead of showing stateLoading while the first fetch runs
	// (docs/adr/0005, "Non-blocking refresh"). A cold cache (nothing to draw)
	// leaves state at stateLoading, set above.
	if cachedMRs, writtenAt, err := snapStore.Load(); err != nil {
		logger.Error("snapshotstore: load failed, booting cold", "err", err)
	} else if len(cachedMRs) > 0 {
		m.state = stateBoard
		m.allMRs = cachedMRs
		m.snapshotWrittenAt = writtenAt
		m.isRefreshing = true // Init() always starts the first fetch
		m.reviewerMRsInStore = hasReviewerSourceMR(cachedMRs)
		m.applyTheme()
		m.applyMRFilter()
		m.updateTicketKey()
	}

	logger.Info("tui: starting", "version", version, "theme", themeName, "mode", themeMode, "view", int(viewMode))
	return m
}

// Init starts the spinner, fires the first data fetch, schedules the minute ticker,
// and resolves team usernames from type:user sources.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.sp.Init(),
		makeFetchCmd(m.baseCtx, m.src, m.includeReviewerMRs, m.allMRs),
		tickCmd(),
		refreshTickCmd(m.refreshInterval, m.refreshGen),
		makeResolveTeamCmd(m.baseCtx, m.src, m.cfg),
		m.sprintFetchCmd(),
	)
}

// sprintFetchCmd returns a Cmd that asks the enricher for current active-sprint
// issue keys, or nil when no JIRA board is configured. It is called on every
// trigger that should see a sprint rollover — boot, manual refresh, and the
// periodic refresh tick — because the enricher itself (not this caller) owns
// the policy for whether that ask reaches JIRA or is served from cache.
func (m Model) sprintFetchCmd() tea.Cmd {
	if m.ticketEnricher == nil || m.cfg.Jira.BoardID == 0 {
		return nil
	}
	return makeSprintFetchCmd(m.baseCtx, m.ticketEnricher, m.cfg.Jira.BoardID)
}

// makeFetchCmd returns a Cmd that fetches all MRs and a cancel func to abort it.
// The cancel is also called via defer inside the goroutine once the fetch finishes.
// previous is the last-known snapshot (nil on a cold boot), passed through so the
// adapter can skip re-fetching unchanged MRs (docs/adr/0005, "Two-phase conditional fetch").
func makeFetchCmd(base context.Context, src mrsvc.MergeRequestSource, includeReviewerMRs bool,
	previous []domain.MergeRequest,
) tea.Cmd {
	ctx, cancel := context.WithTimeout(base, fetchTimeout)
	fetchStartedAt := time.Now()
	return func() tea.Msg {
		defer cancel()
		mrs, errs := src.FetchAll(ctx, mrsvc.FetchOptions{IncludeReviewerMRs: includeReviewerMRs, Previous: previous})
		return FetchResultMsg{MRs: mrs, Errors: errs, FetchStartedAt: fetchStartedAt}
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
// that a subsequent 'q' press can abort an in-flight request. Any currently
// dirty MR (a local write not yet confirmed by a landed fetch) is forced
// stale so this fetch's phase-2 pass re-fetches it fresh rather than trusting
// a phase-1 updatedAt match (docs/adr/0005, "The write race that ungating
// creates").
func (m *Model) startFetch() tea.Cmd {
	ctx, cancel := context.WithTimeout(m.baseCtx, fetchTimeout)
	if m.fetchCancel != nil {
		m.fetchCancel()
	}
	m.fetchCancel = cancel
	m.fetchSeq++
	seq := m.fetchSeq
	src := m.src
	includeReviewerMRs := m.includeReviewerMRs
	previous := m.allMRs
	forceStale := dirtyKeys(m.dirty)
	fetchStartedAt := time.Now()
	return func() tea.Msg {
		defer cancel()
		mrs, errs := src.FetchAll(ctx, mrsvc.FetchOptions{
			IncludeReviewerMRs: includeReviewerMRs,
			Previous:           previous,
			ForceStale:         forceStale,
		})
		return FetchResultMsg{MRs: mrs, Errors: errs, FetchStartedAt: fetchStartedAt, Seq: seq}
	}
}

// handleFetchResult lands a fetch result on the board: it clears the refreshing
// state, merges the snapshot against any local writes, and persists the cache.
func (m Model) handleFetchResult(msg FetchResultMsg) (tea.Model, tea.Cmd) {
	if msg.Seq != m.fetchSeq {
		// A newer fetch has superseded this one (see FetchResultMsg.Seq). Its
		// result is authoritative, so drop this landing — and leave
		// isRefreshing set, because that newer fetch is still in flight.
		m.logger.Debug("tui: dropping superseded fetch result",
			"seq", msg.Seq, "current", m.fetchSeq, "mrs", len(msg.MRs))
		return m, nil
	}
	m.state = stateBoard
	m.isRefreshing = false
	m.allMRs = m.applyFetchResult(msg)
	m.clearResolvedDirty(msg)
	m.errors = msg.Errors
	m.reviewerMRsInStore = hasReviewerSourceMR(m.allMRs)
	// Only a wholly clean fetch may overwrite the cache. A partial or total
	// failure yields a truncated or empty board, and persisting that would
	// destroy the last-known-good snapshot the next launch boots from
	// (docs/adr/0005, "Non-blocking refresh"). snapshotWrittenAt is left
	// untouched too, so the header keeps reporting the real cache age.
	if len(msg.Errors) == 0 {
		m.saveSnapshot()
	}
	m.logger.Info("tui: fetch result", "mrs", len(msg.MRs), "errors", len(msg.Errors))
	for _, e := range msg.Errors {
		m.logger.Warn("tui: fetch partial error", "error", e)
	}
	m.applyTheme()
	m.applyMRFilter()
	m.updateTicketKey()
	cmds := []tea.Cmd{m.makeTicketEnrichCmds(), m.makeTicketLinkCmds(), m.makeTicketDescriptionLinkCmds()}
	if len(m.dirty) > 0 {
		// Landing snapshot was stale relative to one or more local writes;
		// issue an immediate targeted refetch instead of waiting for the
		// next refresh tick (docs/adr/0005).
		m.isRefreshing = true
		cmds = append(cmds, m.startFetch())
	}
	return m, tea.Batch(cmds...)
}

// handleRefreshTick fires a background fetch on the refresh_interval cadence
// (docs/adr/0005, "Refresh cadence"). A tick whose gen no longer matches
// Model.refreshGen belongs to a schedule a manual refresh has since
// superseded; it is dropped without rescheduling, since the newer schedule
// installed by the manual refresh is already driving the cadence. Otherwise
// the next tick is always scheduled, but this tick's own MR fetch is skipped —
// not queued — when one is already in flight: fetchTimeout and the default
// interval are both 60s, so overlap is otherwise reachable. The sprint fetch
// is independent of the MR fetch's in-flight state and always fires: it is
// what lets sprintFetchCmd's own revalidation policy (not this caller) decide
// whether a given tick actually reaches JIRA.
func (m Model) handleRefreshTick(msg refreshTickMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.refreshGen {
		return m, nil
	}
	next := refreshTickCmd(m.refreshInterval, m.refreshGen)
	sprintCmd := m.sprintFetchCmd()
	if m.isRefreshing {
		return m, tea.Batch(next, sprintCmd)
	}
	m.isRefreshing = true
	return m, tea.Batch(next, sprintCmd, m.startFetch())
}

// dirtyKeys returns the keys of dirty as a slice, for passing to FetchOptions.ForceStale.
func dirtyKeys(dirty map[domain.MRKey]time.Time) []domain.MRKey {
	if len(dirty) == 0 {
		return nil
	}
	keys := make([]domain.MRKey, 0, len(dirty))
	for k := range dirty {
		keys = append(keys, k)
	}
	return keys
}

// applyFetchResult merges a landing FetchResultMsg into m.allMRs. For every
// dirty key whose write happened after this fetch started, the landing entry
// is stale relative to that write: the current local entry is kept in place
// instead (docs/adr/0005, "The write race that ungating creates"). Every
// other key — clean, or dirty but confirmed by this fetch — takes the
// landing snapshot's value, matching the pre-dirty-set behavior of a
// wholesale replace.
func (m Model) applyFetchResult(msg FetchResultMsg) []domain.MergeRequest {
	if len(m.dirty) == 0 {
		return msg.MRs
	}

	oldByKey := make(map[domain.MRKey]domain.MergeRequest, len(m.allMRs))
	for _, mr := range m.allMRs {
		oldByKey[mr.Key()] = mr
	}

	seen := make(map[domain.MRKey]bool, len(msg.MRs))
	result := make([]domain.MergeRequest, 0, len(msg.MRs))
	for _, mr := range msg.MRs {
		key := mr.Key()
		seen[key] = true
		if writeAt, dirty := m.dirty[key]; dirty && msg.FetchStartedAt.Before(writeAt) {
			if old, ok := oldByKey[key]; ok {
				result = append(result, old)
				continue
			}
		}
		result = append(result, mr)
	}
	// A dirty-and-stale key can be absent from the landing snapshot entirely
	// (e.g. a concurrent phase-1 hiccup) — keep the local entry rather than
	// silently dropping it.
	for key, writeAt := range m.dirty {
		if seen[key] || !msg.FetchStartedAt.Before(writeAt) {
			continue
		}
		if old, ok := oldByKey[key]; ok {
			result = append(result, old)
		}
	}
	return result
}

// clearResolvedDirty drops every dirty entry confirmed by msg — one whose
// fetch started at or after the write landed, so its value in msg.MRs (or
// its absence, if the MR left the listing) reflects that write.
func (m *Model) clearResolvedDirty(msg FetchResultMsg) {
	for key, writeAt := range m.dirty {
		if !msg.FetchStartedAt.Before(writeAt) {
			delete(m.dirty, key)
		}
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
		m.helpModal.SetSize(msg.Width, msg.Height)
		m.resizeBoard()
		return m, nil

	case FetchResultMsg:
		return m.handleFetchResult(msg)

	case FetchErrMsg:
		m.isRefreshing = false
		m.state = stateError
		m.errMsg = msg.Err.Error()
		return m, nil

	case DetailFetchResultMsg:
		return m.handleDetailFetchResult(msg)

	case DiffFetchResultMsg:
		return m.handleDiffFetchResult(msg)

	case FileRenderResultMsg:
		updated, cmd := m.diffView.Update(msg)
		m.diffView = updated.(diffViewWidget) //nolint:forcetypeassert // diffViewWidget.Update always returns diffViewWidget
		return m, cmd

	case DiffViewClosedMsg:
		m.closeDiffView()
		return m, nil

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

	case BatchReviewerEditorPreviewMsg:
		return m.handleBatchEditorPreview(msg)

	case BatchPreviewBackMsg:
		m.overlay.openOverlay(overlayKindReviewerEditor)
		return m, nil

	case BatchPreviewConfirmedMsg:
		return m.handleBatchPreviewConfirmed(msg)

	case ReviewersSavedMsg:
		return m.handleReviewersSaved(msg)

	case NotifyResultMsg:
		return m.handleNotifyResult(msg)

	case CommandResultMsg:
		return m.handleCommandResult(msg)

	case TicketIssueTypeMsg, SprintIssueKeysMsg, TicketDescriptionLinkResultMsg, TicketLinkResultMsg:
		return m.handleTicketResultMsg(msg)

	case TeamResolvedMsg:
		return m.handleTeamResolved(msg)

	case tickMsg:
		return m, tickCmd()

	case refreshTickMsg:
		return m.handleRefreshTick(msg)

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

// baseStack returns the derived context stack (bottom → top) excluding the
// help modal. It is computed from model state on every use, so the footer and
// help modal can never drift out of sync with the actual focus.
func (m Model) baseStack() []*Context {
	stack := []*Context{BaseCtx}
	if m.state != stateBoard {
		return stack
	}
	switch m.overlay.active() {
	case overlayKindDiffView:
		return append(stack, DiffViewCtx)
	case overlayKindSettings:
		return append(stack, SettingsCtx)
	case overlayKindReviewerEditor:
		if m.reviewerEditor != nil {
			return append(stack, m.reviewerEditor.Context())
		}
	case overlayKindBatchPreview:
		return append(stack, BatchPreviewCtx)
	case overlayKindNone:
	}
	if m.showDetail {
		return append(stack, DetailCtx)
	}
	return append(stack, BoardCtx, m.customCommandsCtx)
}

// contextStack is baseStack plus the help modal context when it is open.
func (m Model) contextStack() []*Context {
	stack := m.baseStack()
	if m.showHelp {
		stack = append(stack, HelpCtx)
	}
	return stack
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// The help modal owns all key input while open.
	if m.showHelp {
		if DefaultHelpKeyMap.Close.Match(msg) {
			m.showHelp = false
		}
		return m, nil
	}

	stack := m.baseStack()
	top := stack[len(stack)-1]
	captures := top.CapturesText()

	// Base-context keys apply unless shadowed by the top context or consumed
	// by a focused text input; ctrl+c quits unconditionally.
	if DefaultBaseKeyMap.Quit.Match(msg) &&
		(msg.String() == "ctrl+c" || (!captures && !top.binds(msg.String()))) {
		if m.fetchCancel != nil {
			m.fetchCancel()
		}
		return m, tea.Quit
	}
	if !captures && !top.binds(msg.String()) && DefaultBaseKeyMap.Help.Match(msg) {
		m.showHelp = true
		return m, nil
	}

	if m.state != stateBoard {
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
	case overlayKindBatchPreview:
		if m.batchPreview != nil {
			updated, cmd := m.batchPreview.Update(msg)
			m.batchPreview = updated.(*batchPreviewWidget)
			return m, cmd
		}
	case overlayKindDiffView:
		updated, cmd := m.diffView.Update(msg)
		m.diffView = updated.(diffViewWidget) //nolint:forcetypeassert // diffViewWidget.Update always returns diffViewWidget
		return m, cmd
	}

	if m.showDetail {
		return m.handleKeyDetail(msg)
	}
	return m.handleKeyBoard(msg)
}

// handleKeyDetail handles keys while the detail panel owns focus.
func (m Model) handleKeyDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.detailKeys.Close.Match(msg):
		m.closeDetail()
		return m, nil
	case m.detailKeys.ScrollUp.Match(msg):
		m.detail.ScrollUp()
	case m.detailKeys.ScrollDown.Match(msg):
		m.detail.ScrollDown()
	case m.detailKeys.Open.Match(msg):
		if mr := m.board.FocusedMR(); mr != nil {
			return m, openBrowser(mr.WebURL)
		}
	case m.detailKeys.Diff.Match(msg):
		if mr := m.board.FocusedMR(); mr != nil {
			return m, m.openDiffView(mr)
		}
	}
	return m, nil
}

// handleKeyBoard handles keys while the kanban board owns focus.
func (m Model) handleKeyBoard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Custom commands sit above BoardCtx in the stack (see baseStack), so a
	// match here takes priority over the static board bindings below.
	if cmd, ok := m.matchCustomCommand(msg); ok {
		mr := m.board.FocusedMR()
		if mr == nil {
			m.logger.Debug("tui: configured command matched but no MR focused", "command", cmd.Name)
			return m, nil
		}
		return m, m.execCommandCmd(*mr, cmd)
	}
	switch {
	case m.keys.Up.Match(msg):
		m.board.MoveUp()
		m.updateTicketKey()
	case m.keys.Down.Match(msg):
		m.board.MoveDown()
		m.updateTicketKey()
	case m.keys.Left.Match(msg):
		m.board.MoveLeft()
		m.updateTicketKey()
	case m.keys.Right.Match(msg):
		m.board.MoveRight()
		m.updateTicketKey()
	case m.keys.Refresh.Match(msg):
		if m.showDetail {
			m.closeDetail()
		}
		// Bump refreshGen so any auto-refresh tick already scheduled under the
		// old generation is dropped on arrival instead of firing early, then
		// install a fresh schedule — this is what "manual refresh resets the
		// timer" means (docs/adr/0005, "Refresh cadence").
		m.refreshGen++
		resetTimer := refreshTickCmd(m.refreshInterval, m.refreshGen)
		sprintCmd := m.sprintFetchCmd()
		if m.isRefreshing {
			// A fetch is already in flight. Starting a second one would cancel
			// it (startFetch cancels the previous context) and land a degraded
			// partial result, so skip — not queue — this fetch, the same rule
			// handleRefreshTick applies. The cadence is still reset above. The
			// sprint fetch is independent of the MR fetch's cancellation, so it
			// still runs.
			return m, tea.Batch(resetTimer, sprintCmd)
		}
		if len(m.allMRs) > 0 {
			m.isRefreshing = true
			return m, tea.Batch(m.sp.Init(), resetTimer, m.startFetch(), sprintCmd)
		}
		m.state = stateLoading
		return m, tea.Batch(m.sp.Init(), resetTimer, m.startFetch(), sprintCmd)
	case m.keys.Sort.Match(msg):
		m.sortField, m.sortDesc = advanceSort(m.sortField, m.sortDesc)
		m.header.SetSort(sortLabel(m.sortField, m.sortDesc))
		m.applyMRFilter()
		m.saveState()
	case m.keys.Sprint.Match(msg):
		m.sprintFilterActive = !m.sprintFilterActive
		m.applyMRFilter()
	case m.keys.ToggleView.Match(msg):
		if m.viewMode == domain.ViewMine {
			m.viewMode = domain.ViewAll
			m.header.SetTitle("mrboard")
		} else {
			m.viewMode = domain.ViewMine
			m.header.SetTitle("mrboard — @" + m.currentUser)
		}
		m.applyMRFilter()
		m.saveState()
	case m.keys.Open.Match(msg):
		if mr := m.board.FocusedMR(); mr != nil {
			return m, openBrowser(mr.WebURL)
		}
	case m.keys.Detail.Match(msg):
		if mr := m.board.FocusedMR(); mr != nil {
			m.openDetail(mr)
			return m, m.fetchDetailCmd(mr)
		}
	case m.keys.Settings.Match(msg):
		m.openSettings()
		return m, nil
	case m.keys.Reviewers.Match(msg):
		if mr := m.board.FocusedMR(); mr != nil {
			siblings := m.SiblingMRs(m.keyMatcher.ExtractFromTitle(mr.Title))
			m.reviewerEditor = newReviewerEditorWidget(
				m.baseCtx, *mr, siblings, m.styles, m.reviewerEditorKeys, m.src, m.teamRoster, m.keyMatcher,
			)
			m.overlay.openOverlay(overlayKindReviewerEditor)
			return m, nil
		}
	case m.keys.Diff.Match(msg):
		if mr := m.board.FocusedMR(); mr != nil {
			return m, m.openDiffView(mr)
		}
	case m.keys.Notify.Match(msg):
		if mr := m.board.FocusedMR(); mr != nil && m.notifier != nil {
			m.logger.Info("tui: notify key pressed", "mr_iid", mr.IID, "mr_title", mr.Title)
			return m, m.notifyCmd(mr)
		}
	case m.keys.OpenTicket.Match(msg):
		if mr := m.board.FocusedMR(); mr != nil {
			if url := domain.JiraIssueURL(m.ticketBaseURL, m.keyMatcher.ExtractFromTitle(mr.Title)); url != "" {
				return m, openBrowser(url)
			}
		}
	}
	return m, nil
}

func (m *Model) openDetail(mr *domain.MergeRequest) {
	m.showDetail = true
	m.board.SetActive(false)
	m.detail.SetMR(mr)
	m.resizeBoard()
}

func (m *Model) closeDetail() {
	m.showDetail = false
	m.board.SetActive(true)
	m.resizeBoard()
}

// openDiffView switches focus to the diff view and returns the Cmd that
// fetches the initial MRDiff.
func (m *Model) openDiffView(mr *domain.MergeRequest) tea.Cmd {
	m.overlay.openOverlay(overlayKindDiffView)
	m.diffView.SetMR(mr)
	bodyH := m.height - chromeHeight
	m.diffView.SetSize(m.width, bodyH)
	m.header.SetTitle(fmt.Sprintf("diff !%d – %s", mr.IID, mr.Title))
	m.header.SetStats("loading…")
	return m.diffView.fetchDiffCmd(mr)
}

func (m *Model) closeDiffView() {
	m.overlay.closeOverlay()
	m.header.SetTitle("mrboard")
	m.header.SetStats("")
}

func (m Model) renderDiffScreen() string {
	headerStr := m.header.render()
	footerStr := m.footer.render(m.contextStack())
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
		m.header.SetSort(sortLabel(m.sortField, m.sortDesc))
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

// matchCustomCommand returns the configured command bound to msg's key, if any.
func (m Model) matchCustomCommand(msg tea.KeyPressMsg) (config.Command, bool) {
	for i, a := range m.customCommandsCtx.actions {
		if a.Match(msg) {
			return m.cfg.Commands[i], true
		}
	}
	return config.Command{}, false
}

// execCommandCmd resolves cmd's argv against mr and returns a Cmd that
// suspends mrboard, runs cmd.Binary via tea.ExecProcess, and resumes mrboard
// on exit (docs/adr/0004-external-command-launcher.md). Argv resolution
// failure is reported through the same CommandResultMsg as a run failure,
// keeping a single outcome handler for the whole invocation.
func (m Model) execCommandCmd(mr domain.MergeRequest, cmd config.Command) tea.Cmd {
	argv, err := BuildCommandArgv(mr, cmd)
	if err != nil {
		m.logger.Debug("tui: configured command argv resolution failed",
			"command", cmd.Name, "binary", cmd.Binary, "err", err)
		return func() tea.Msg { return CommandResultMsg{CommandName: cmd.Name, Err: err} }
	}
	m.logger.Debug("tui: executing configured command",
		"command", cmd.Name, "binary", cmd.Binary, "args", argv, "mr_iid", mr.IID)
	//nolint:gosec // G204: cmd.Binary is an admin-configured mrboard.toml entry, not attacker input;
	// no shell is involved, so there is no injection surface beyond running the configured binary itself.
	execCmd := exec.Command(cmd.Binary, argv...)
	return tea.ExecProcess(execCmd, func(err error) tea.Msg {
		return CommandResultMsg{CommandName: cmd.Name, Err: err}
	})
}

// handleCommandResult reports a configured command's outcome. Success shows no
// toast — the resumed, redrawn board is itself the success signal.
func (m Model) handleCommandResult(msg CommandResultMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.logger.Error("tui: command failed", "command", msg.CommandName, "err", msg.Err)
		return m, m.toast(toast.ErrorAlert, fmt.Sprintf("%s failed: %s", msg.CommandName, msg.Err))
	}
	return m, nil
}

// updateTicketKey enables or disables the ticket key based on whether the
// focused MR has a detectable ticket ID and ticketBaseURL is configured, and
// syncs m.selected to match. Call after any navigation that may change the
// focused card.
func (m *Model) updateTicketKey() {
	mr := m.board.FocusedMR()
	if mr != nil {
		m.selected = mr.Key()
	}
	enabled := m.ticketBaseURL != "" && mr != nil && m.keyMatcher.ExtractFromTitle(mr.Title) != ""
	m.keys.OpenTicket.SetEnabled(enabled)
}

// View renders the full screen. Only the root model sets AltScreen.
func (m Model) View() tea.View {
	content := m.alerts.Render(m.renderContent())
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) renderContent() string {
	screen := m.renderScreen()
	if m.showHelp {
		screen = m.renderWithOverlay(screen, m.helpModal.render(m.baseStack()))
	}
	return screen
}

func (m Model) renderScreen() string {
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
		switch m.overlay.active() {
		case overlayKindSettings:
			return m.renderWithOverlay(board, m.settings.render())
		case overlayKindReviewerEditor:
			if m.reviewerEditor != nil {
				return m.renderWithOverlay(board, m.reviewerEditor.render())
			}
		case overlayKindBatchPreview:
			if m.batchPreview != nil {
				return m.renderWithOverlay(board, m.batchPreview.render())
			}
		}
		return board
	}
	return ""
}

func (m Model) renderBoard() string {
	m.header.SetSnapshotAge(m.snapshotWrittenAt, m.isRefreshing, m.sp.spinner.View())
	headerStr := m.header.render()
	footerStr := m.footer.render(m.contextStack())
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

// handleReviewersSaved closes the editor, updates the MR in-place, and fires a
// Teams notification automatically when a notifier is configured and the write
// changed the approver set (plain reviewer edits do not notify).
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
	m.dirty[updatedMR.Key()] = time.Now()
	m.applyMRFilter()
	m.updateTicketKey()

	cmds := []tea.Cmd{m.toast(toast.InfoAlert, "Reviewers saved")}
	// Only ping Teams when the approver set actually changed — a plain reviewer
	// reassignment is not notification-worthy.
	if m.notifier != nil && msg.ApproversChanged {
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

// handleTicketResultMsg groups the four ticket-tracker background-result
// message types under one coreUpdate case, keeping that switch's cyclomatic
// complexity down.
func (m Model) handleTicketResultMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TicketIssueTypeMsg:
		return m.handleTicketIssueType(msg)
	case SprintIssueKeysMsg:
		return m.handleSprintIssueKeys(msg)
	case TicketDescriptionLinkResultMsg:
		return m.handleTicketDescriptionLinkResult(msg)
	case TicketLinkResultMsg:
		return m.handleTicketLinkResult(msg)
	default:
		return m, nil
	}
}

// handleTicketIssueType stores a freshly fetched issue type on the matching MR(s).
func (m Model) handleTicketIssueType(msg TicketIssueTypeMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.logger.Warn("tui: ticket fetch failed", "key", msg.IssueKey, "err", msg.Err)
		return m, nil
	}
	for i := range m.allMRs {
		if m.keyMatcher.ExtractFromTitle(m.allMRs[i].Title) == msg.IssueKey {
			m.allMRs[i].JiraIssueType = msg.IssueType
		}
	}
	m.applyMRFilter()
	return m, nil
}

// handleSprintIssueKeys stores the active sprint key set and re-applies the filter.
func (m Model) handleSprintIssueKeys(msg SprintIssueKeysMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.logger.Warn("tui: sprint fetch failed", "err", msg.Err)
		return m, nil
	}
	if len(msg.Keys) == 0 {
		// No active sprint; keep sprintIssueKeys nil so the filter stays inert.
		return m, nil
	}
	m.sprintIssueKeys = make(map[string]bool, len(msg.Keys))
	for _, k := range msg.Keys {
		m.sprintIssueKeys[k] = true
	}
	m.applyMRFilter()
	return m, nil
}

func (m Model) handleDetailFetchResult(msg DetailFetchResultMsg) (tea.Model, tea.Cmd) {
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
}

func (m Model) handleBatchEditorPreview(msg BatchReviewerEditorPreviewMsg) (tea.Model, tea.Cmd) {
	m.batchPreview = newBatchPreviewWidget(
		msg.Staged, msg.Siblings, msg.FocusedMR, msg.KnownIDs, m.styles, m.batchPreviewKeys,
	)
	m.overlay.openOverlay(overlayKindBatchPreview)
	return m, nil
}

func (m Model) handleBatchPreviewConfirmed(msg BatchPreviewConfirmedMsg) (tea.Model, tea.Cmd) {
	m.overlay.closeOverlay()
	if len(msg.Targets) == 0 {
		return m, nil
	}
	cmds := make([]tea.Cmd, len(msg.Targets))
	for i, mr := range msg.Targets {
		cmds[i] = makeReviewerWriteCmd(m.baseCtx, m.src, mr, msg.Staged, msg.KnownIDs)
	}
	return m, tea.Batch(cmds...)
}

// makeReviewerWriteCmd is the single reviewer-write use case: it wraps
// mrsvc.ApplyReviewerChanges with the snapshot + ID-resolution + origApprovers
// setup shared by both the single-edit save path (reviewerEditorWidget.saveCmd)
// and each per-target write in a batch apply (handleBatchPreviewConfirmed), so
// that setup — previously duplicated and diverging between the two callers —
// can no longer drift out of sync.
//
// knownIDs seeds already-resolved usernames, e.g. from the project-members
// fetch the single-edit path ran against its own MR. GitLab user IDs are
// global to the instance, so a seed resolved against one project remains
// valid when writing to another target's project — it lets ApplyReviewerChanges
// skip a redundant GetProjectMembers call for any target sharing a resolved user.
func makeReviewerWriteCmd(
	base context.Context,
	src reviewerWriter,
	target domain.MergeRequest,
	staged []stagedReviewer,
	knownIDs map[string]int64,
) tea.Cmd {
	projectID := int64(target.ProjectID)
	mrIID := int64(target.IID)

	// Snapshot everything so the closure captures stable data.
	edits := make([]mrsvc.ReviewerEdit, len(staged))
	for i, s := range staged {
		edits[i] = mrsvc.ReviewerEdit{Username: s.Username, IsApprover: s.IsApprover, UserID: s.UserID}
	}
	ids := make(map[string]int64, len(knownIDs))
	for k, v := range knownIDs {
		ids[k] = v
	}
	origApprovers := make(map[string]bool, len(target.Reviewers))
	for _, r := range target.Reviewers {
		if r.IsApprover {
			origApprovers[r.Username] = true
		}
	}

	ctx, cancel := context.WithTimeout(base, fetchTimeout)
	return func() tea.Msg {
		defer cancel()
		mr, approversChanged, err := mrsvc.ApplyReviewerChanges(ctx, src, projectID, mrIID, edits, ids, origApprovers)
		return ReviewersSavedMsg{MR: mr, ApproversChanged: approversChanged, Err: err}
	}
}

// makeTicketEnrichCmds returns one fetch command per unique issue key found
// in allMRs. Returns nil when ticketEnricher is nil or no keys are found.
func (m *Model) makeTicketEnrichCmds() tea.Cmd {
	if m.ticketEnricher == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var cmds []tea.Cmd
	for _, mr := range m.allMRs {
		if issueKey := m.keyMatcher.ExtractFromTitle(mr.Title); issueKey != "" {
			if _, ok := seen[issueKey]; !ok {
				seen[issueKey] = struct{}{}
				cmds = append(cmds, makeTicketFetchCmd(m.baseCtx, m.ticketEnricher, issueKey))
			}
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// makeTicketFetchCmd returns a Cmd that calls GetIssueType for issueKey and
// wraps the result in a TicketIssueTypeMsg.
func makeTicketFetchCmd(base context.Context, enricher ticketsvc.TicketEnricher, issueKey string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, ticketFetchTimeout)
		defer cancel()
		issueType, err := enricher.GetIssueType(ctx, issueKey)
		return TicketIssueTypeMsg{IssueKey: issueKey, IssueType: issueType, Err: err}
	}
}

// makeSprintFetchCmd returns a Cmd that loads all issue keys for the active sprint
// of the given JIRA board and wraps the result in a SprintIssueKeysMsg.
func makeSprintFetchCmd(base context.Context, enricher ticketsvc.TicketEnricher, boardID int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, ticketFetchTimeout)
		defer cancel()
		keys, err := enricher.GetActiveSprintIssueKeys(ctx, boardID)
		return SprintIssueKeysMsg{Keys: keys, Err: err}
	}
}

// makeTicketLinkCmds returns one UpsertRemoteLink command per MR that has a
// ticket key in its title. Returns nil when ticketLinker is nil or no MRs have ticket keys.
func (m *Model) makeTicketLinkCmds() tea.Cmd {
	if m.ticketLinker == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, mr := range m.allMRs {
		issueKey := m.keyMatcher.ExtractFromTitle(mr.Title)
		if issueKey == "" || mr.WebURL == "" {
			continue
		}
		cmds = append(cmds, makeTicketLinkCmd(m.baseCtx, m.ticketLinker, mr, issueKey))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// makeTicketLinkCmd returns a Cmd that calls UpsertRemoteLink for a single MR.
// The globalId format is load-bearing per ADR-0003: changing it orphans existing links.
// The display title is "!{IID} {repoName}: {mrTitle}" — repo name is the last path
// segment of ProjectPath so it stays short and human-readable in the tracker.
func makeTicketLinkCmd(
	base context.Context, linker ticketsvc.TicketLinker, mr domain.MergeRequest, issueKey string,
) tea.Cmd {
	globalID := fmt.Sprintf("mrboard:%d:%d", mr.ProjectID, mr.IID)
	repoName := mr.ProjectPath
	if i := strings.LastIndex(repoName, "/"); i >= 0 {
		repoName = repoName[i+1:]
	}
	displayTitle := fmt.Sprintf("!%d %s: %s", mr.IID, repoName, mr.Title)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, ticketFetchTimeout)
		defer cancel()
		err := linker.UpsertRemoteLink(ctx, issueKey, globalID, displayTitle, mr.WebURL)
		return TicketLinkResultMsg{IssueKey: issueKey, GlobalID: globalID, Err: err}
	}
}

// handleTicketLinkResult surfaces write failures as toast alerts; successes are silent.
func (m Model) handleTicketLinkResult(msg TicketLinkResultMsg) (tea.Model, tea.Cmd) {
	if msg.Err == nil {
		return m, nil
	}
	m.logger.Warn("tui: ticket remote link failed", "issueKey", msg.IssueKey, "globalId", msg.GlobalID, "err", msg.Err)
	return m, m.toast(toast.ErrorAlert, "JIRA link failed: "+msg.IssueKey)
}

// ticketDescLinkKey identifies an MR for description-back-link dedup.
type ticketDescLinkKey struct {
	projectID int
	mrIID     int
}

// makeTicketDescriptionLinkCmds returns one description-back-link command per
// MR that has a ticket key in its title and hasn't already been linked this
// session. Returns nil when ticketBaseURL is unconfigured or no MRs qualify.
// gitlabadpt only ever executes the plain mrsvc.UpdateDescription write it's
// told to make (per ADR-0003); the decision of what to write, whether it's
// already present, and per-session dedup all live here in the TUI.
func (m *Model) makeTicketDescriptionLinkCmds() tea.Cmd {
	if m.ticketBaseURL == "" {
		return nil
	}
	var cmds []tea.Cmd
	for _, mr := range m.allMRs {
		issueKey := m.keyMatcher.ExtractFromTitle(mr.Title)
		if issueKey == "" {
			continue
		}
		key := ticketDescLinkKey{projectID: mr.ProjectID, mrIID: mr.IID}
		if m.ticketDescLinked[key] {
			continue
		}
		m.ticketDescLinked[key] = true // claimed now; removed on error to allow retry next refresh
		cmds = append(cmds, makeTicketDescriptionLinkCmd(m.baseCtx, m.src, m.ticketBaseURL, mr.ProjectID, mr.IID, issueKey))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// makeTicketDescriptionLinkCmd returns a Cmd that reads the MR description,
// appends a ticket back-link if domain.HasJiraLink reports it's absent, and
// writes it back via the generic mrsvc.MergeRequestSource.UpdateDescription.
func makeTicketDescriptionLinkCmd(
	base context.Context, src mrsvc.MergeRequestSource, ticketBaseURL string, projectID, mrIID int, issueKey string,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, ticketFetchTimeout)
		defer cancel()
		desc, _, err := src.GetDetail(ctx, int64(projectID), int64(mrIID))
		if err != nil {
			return TicketDescriptionLinkResultMsg{ProjectID: projectID, MRIID: mrIID, IssueKey: issueKey, Err: err}
		}
		if domain.HasJiraLink(desc) {
			return TicketDescriptionLinkResultMsg{ProjectID: projectID, MRIID: mrIID, IssueKey: issueKey}
		}
		newDesc := domain.AppendJiraLink(desc, ticketBaseURL, issueKey)
		err = src.UpdateDescription(ctx, int64(projectID), int64(mrIID), newDesc)
		return TicketDescriptionLinkResultMsg{ProjectID: projectID, MRIID: mrIID, IssueKey: issueKey, Err: err}
	}
}

// handleTicketDescriptionLinkResult surfaces write failures as toast alerts;
// successes are silent. On error the dedup claim is released so the next
// refresh retries. A successful write marks the MR dirty so a landing
// snapshot started before this write completed doesn't undo it — mostly
// moot today since the write only touches GitLab's stored description (not
// a field allMRs carries), but it keeps the guard's write-path coverage
// complete per docs/adr/0005 and forces a fresh phase-2 fetch for the MR.
func (m Model) handleTicketDescriptionLinkResult(msg TicketDescriptionLinkResultMsg) (tea.Model, tea.Cmd) {
	if msg.Err == nil {
		m.dirty[domain.MRKey{ProjectID: msg.ProjectID, IID: msg.MRIID}] = time.Now()
		return m, nil
	}
	delete(m.ticketDescLinked, ticketDescLinkKey{projectID: msg.ProjectID, mrIID: msg.MRIID})
	m.logger.Warn("tui: ticket description link failed", "issueKey", msg.IssueKey, "err", msg.Err)
	return m, m.toast(toast.ErrorAlert, "JIRA back-link failed: "+msg.IssueKey)
}

// handleDiffFetchResult delegates the fetched MRDiff to diffView and updates
// the header stats to match, once the widget confirms the result matches the
// MR it currently has open.
func (m Model) handleDiffFetchResult(msg DiffFetchResultMsg) (tea.Model, tea.Cmd) {
	updated, cmd := m.diffView.Update(msg)
	m.diffView = updated.(diffViewWidget) //nolint:forcetypeassert // diffViewWidget.Update always returns diffViewWidget
	if msg.Err == nil && m.diffView.mr != nil &&
		m.diffView.mr.ProjectID == msg.ProjectID && m.diffView.mr.IID == msg.MRIID {
		added, removed := diffStats(msg.Diff.Files)
		m.header.SetStats(fmt.Sprintf("%d files  +%d -%d", len(msg.Diff.Files), added, removed))
	}
	return m, cmd
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
	m.helpModal.SetStyles(m.styles)
	m.detail.SetStyles(m.styles)
	m.diffView.SetStyles(m.styles)
	m.settings.styles = m.styles
	if m.reviewerEditor != nil {
		m.reviewerEditor.styles = m.styles
	}
	if m.batchPreview != nil {
		m.batchPreview.styles = m.styles
	}
}

func (m *Model) applyMRFilter() {
	m.buildTicketIndex()
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
		MyView:       m.viewMode == domain.ViewMine,
		CurrentUser:  m.currentUser,
		SortField:    m.sortField.stateKey(),
		SortDesc:     m.sortDesc,
		Phases:       m.filter.Phases,
		Assignees:    m.filter.Assignees,
		Reviewers:    m.filter.Reviewers,
		SprintFilter: m.sprintFilterActive,
		SprintKeys:   m.sprintIssueKeys,
		KeyMatcher:   m.keyMatcher,
	})
	displayMRs := visibleMRs(mrs, m.currentUser)
	m.selected = m.board.SetMRs(displayMRs, m.selected)
	m.header.SetMRs(displayMRs)
	m.header.SetFilterActive(m.isFilterActive())
	m.header.SetSprintFilterActive(m.sprintFilterActive)
}

func visibleMRs(mrs []domain.MergeRequest, _ string) []domain.MergeRequest {
	return mrs
}

// buildTicketIndex rebuilds ticketIndex from allMRs. MRs without a detectable ticket
// key are omitted; empty key "" is never stored.
func (m *Model) buildTicketIndex() {
	idx := make(map[string][]domain.MergeRequest, len(m.allMRs))
	for _, mr := range m.allMRs {
		if key := m.keyMatcher.ExtractFromTitle(mr.Title); key != "" {
			idx[key] = append(idx[key], mr)
		}
	}
	m.ticketIndex = idx
}

// SiblingMRs returns all MRs in allMRs that share the given ticket issue key.
// Returns nil when issueKey is empty or no MRs match.
func (m Model) SiblingMRs(issueKey string) []domain.MergeRequest {
	if issueKey == "" {
		return nil
	}
	return m.ticketIndex[issueKey]
}

func (m *Model) isFilterActive() bool {
	return len(m.filter.Phases) > 0 || len(m.filter.Assignees) > 0 || len(m.filter.Reviewers) > 0 ||
		m.sprintFilterActive
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

// saveSnapshot persists the current board data and stamps the age shown in the
// header (docs/adr/0005, "Non-blocking refresh"). Called on every successful
// fetch swap, not just at boot.
func (m *Model) saveSnapshot() {
	m.snapshotWrittenAt = time.Now()
	if err := m.snapshotStore.Save(m.allMRs); err != nil {
		m.logger.Error("snapshotstore: save failed", "err", err)
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
