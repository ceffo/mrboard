package tui

import "github.com/ceffo/mrboard/internal/config"

// This file is the single source of truth for every keybinding in mrboard:
// each action is defined exactly once as an Action and registered into a
// Context. Widgets dispatch against these same values; the footer and the
// '?' help modal render them. key.NewBinding must not appear anywhere else
// (enforced by TestBindingsDefinedOnlyInKeys).
//
// Actions shared between contexts (e.g. "open MR" on board, detail and diff
// view) are declared once below and assigned into each keymap.

// Shared actions — defined once, referenced by several contexts.
var (
	actOpenMR = Act("o", "open MR", PriorityCommon, CategoryAct)
	actDiff   = Act("d", "diff", PriorityCommon, CategoryAct)
)

// BaseKeyMap is always active at the bottom of the context stack. Its keys
// are shadowed by any context that binds them and suppressed entirely while
// a text input captures keys (except ctrl+c, which always quits).
type BaseKeyMap struct {
	Help Action
	Quit Action
}

// DefaultBaseKeyMap is the default always-on binding set.
var DefaultBaseKeyMap = BaseKeyMap{
	Help: Act("?", "help", PriorityPinned, CategoryGeneral),
	Quit: Act("q", "quit", PriorityPinned, CategoryGeneral, "q", "ctrl+c"),
}

// BaseCtx is the always-on bottom of the context stack.
var BaseCtx = NewContext("base", "mrboard", &DefaultBaseKeyMap)

// HelpKeyMap holds the keybindings active while the help modal is open.
type HelpKeyMap struct {
	Close Action
}

// DefaultHelpKeyMap is the default binding set for the help modal.
var DefaultHelpKeyMap = HelpKeyMap{
	Close: Act("? · esc", "close", PriorityPinned, CategoryGeneral, "?", "esc"),
}

// HelpCtx sits on top of the stack while the help modal is open; it owns all
// key input so only its close binding is reachable.
var HelpCtx = NewContext("help", "Help", &DefaultHelpKeyMap)

// BoardKeyMap contains all keybindings for board mode.
type BoardKeyMap struct {
	Up         Action
	Down       Action
	Left       Action
	Right      Action
	Detail     Action
	Refresh    Action
	Open       Action
	Reviewers  Action
	Diff       Action
	Sort       Action
	ToggleView Action
	Sprint     Action
	Notify     Action
	OpenTicket Action
	Settings   Action
}

// DefaultBoardKeyMap is the default keybinding set for board mode.
var DefaultBoardKeyMap = BoardKeyMap{
	Up:         Act("↑/k", "up", PriorityCore, CategoryNavigate, "up", "k"),
	Down:       Act("↓/j", "down", PriorityCore, CategoryNavigate, "down", "j"),
	Left:       Act("←/h", "left", PriorityCore, CategoryNavigate, "left", "h"),
	Right:      Act("→/l", "right", PriorityCore, CategoryNavigate, "right", "l"),
	Detail:     Act("↵", "details", PriorityCore, CategoryNavigate, "enter"),
	Refresh:    Act("r", "refresh", PriorityCommon, CategoryAct),
	Open:       actOpenMR,
	Reviewers:  Act("v", "reviewers", PriorityCommon, CategoryAct),
	Diff:       actDiff,
	Sort:       Act("s", "sort", PriorityCommon, CategoryView),
	ToggleView: Act("tab", "toggle view", PriorityCommon, CategoryView),
	Sprint:     Act("S", "sprint filter", PriorityModal, CategoryView),
	Notify:     Act("n", "notify", PriorityModal, CategoryAct),
	OpenTicket: Act("J", "open jira", PriorityModal, CategoryAct),
	Settings:   Act(",", "settings", PriorityModal, CategoryGeneral),
}

// BoardCtx is the board-mode context.
var BoardCtx = NewContext("board", "Board", &DefaultBoardKeyMap,
	WithFooterGroup("↑↓←→", "move",
		&DefaultBoardKeyMap.Up, &DefaultBoardKeyMap.Down,
		&DefaultBoardKeyMap.Left, &DefaultBoardKeyMap.Right),
)

// DetailKeyMap contains keybindings while the detail panel owns focus.
// Left/right are intentionally absent — reserved for future section navigation.
type DetailKeyMap struct {
	ScrollUp   Action
	ScrollDown Action
	Close      Action
	Open       Action
	Diff       Action
}

// DefaultDetailKeyMap is the default keybinding set for the detail panel.
var DefaultDetailKeyMap = DetailKeyMap{
	ScrollUp:   Act("↑/k", "scroll up", PriorityCore, CategoryNavigate, "up", "k"),
	ScrollDown: Act("↓/j", "scroll down", PriorityCore, CategoryNavigate, "down", "j"),
	Close:      Act("esc/↵", "close", PriorityCore, CategoryGeneral, "esc", "enter"),
	Open:       actOpenMR,
	Diff:       actDiff,
}

// DetailCtx is the detail-panel context.
var DetailCtx = NewContext("detail", "Details", &DefaultDetailKeyMap,
	WithFooterGroup("↑↓", "scroll", &DefaultDetailKeyMap.ScrollUp, &DefaultDetailKeyMap.ScrollDown),
)

// DiffViewKeyMap holds keybindings for the diff view.
type DiffViewKeyMap struct {
	PrevFile     Action
	NextFile     Action
	ScrollUp     Action
	ScrollDown   Action
	HalfPageUp   Action
	HalfPageDown Action
	Top          Action
	Bottom       Action
	Open         Action
	Close        Action
}

// DefaultDiffViewKeyMap is the default keybinding set for the diff view.
var DefaultDiffViewKeyMap = DiffViewKeyMap{
	PrevFile:     Act("p", "prev file", PriorityCore, CategoryNavigate),
	NextFile:     Act("n", "next file", PriorityCore, CategoryNavigate),
	ScrollUp:     Act("↑/k", "scroll up", PriorityCore, CategoryNavigate, "up", "k"),
	ScrollDown:   Act("↓/j", "scroll down", PriorityCore, CategoryNavigate, "down", "j"),
	HalfPageUp:   Act("^u", "½ page up", PriorityModal, CategoryNavigate, "ctrl+u"),
	HalfPageDown: Act("^d", "½ page down", PriorityModal, CategoryNavigate, "ctrl+d"),
	Top:          Act("g", "top", PriorityModal, CategoryNavigate),
	Bottom:       Act("G", "bottom", PriorityModal, CategoryNavigate),
	Open:         actOpenMR,
	Close:        Act("d/esc", "close", PriorityCore, CategoryGeneral, "d", "esc"),
}

// DiffViewCtx is the full-screen diff view context.
var DiffViewCtx = NewContext("diff", "Diff", &DefaultDiffViewKeyMap,
	WithFooterGroup("p/n", "file", &DefaultDiffViewKeyMap.PrevFile, &DefaultDiffViewKeyMap.NextFile),
	WithFooterGroup("↑↓", "scroll", &DefaultDiffViewKeyMap.ScrollUp, &DefaultDiffViewKeyMap.ScrollDown),
)

// SettingsKeyMap holds keybindings for the settings panel.
type SettingsKeyMap struct {
	Up      Action
	Down    Action
	Left    Action
	Right   Action
	PrevTab Action
	NextTab Action
	Toggle  Action
	Confirm Action
	Close   Action
}

// DefaultSettingsKeyMap is the default keybinding set for the settings panel.
var DefaultSettingsKeyMap = SettingsKeyMap{
	Up:      Act("↑/k", "up", PriorityCore, CategoryNavigate, "up", "k"),
	Down:    Act("↓/j", "down", PriorityCore, CategoryNavigate, "down", "j"),
	Left:    Act("←/h", "prev section", PriorityCore, CategoryNavigate, "left", "h"),
	Right:   Act("→/l", "next section", PriorityCore, CategoryNavigate, "right", "l"),
	PrevTab: Act("shift+tab", "prev tab", PriorityModal, CategoryView),
	NextTab: Act("tab", "next tab", PriorityCore, CategoryView),
	Toggle:  Act("space", "toggle", PriorityCore, CategoryAct),
	Confirm: Act("↵", "apply", PriorityCore, CategoryGeneral, "enter"),
	Close:   Act(",/esc", "close", PriorityCore, CategoryGeneral, ",", "esc"),
}

// SettingsCtx is the settings-panel context.
var SettingsCtx = NewContext("settings", "Settings", &DefaultSettingsKeyMap,
	WithFooterGroup("↑↓", "move", &DefaultSettingsKeyMap.Up, &DefaultSettingsKeyMap.Down),
	WithFooterGroup("←→", "section", &DefaultSettingsKeyMap.Left, &DefaultSettingsKeyMap.Right),
)

// ReviewerEditorKeyMap holds keybindings for the reviewer editor overlay
// (list mode; the search sub-mode uses ReviewerSearchKeyMap). Tab switches to
// the sibling-MR panel — present whenever the focused MR shares a JIRA key
// with other open MRs, empty otherwise.
type ReviewerEditorKeyMap struct {
	Up             Action
	Down           Action
	Tab            Action
	ToggleApprover Action
	Remove         Action
	Search         Action
	SetTeam        Action
	Confirm        Action
	Close          Action
}

// DefaultReviewerEditorKeyMap is the default keybinding set for the reviewer editor.
var DefaultReviewerEditorKeyMap = ReviewerEditorKeyMap{
	Up:             Act("↑/k", "up", PriorityCore, CategoryNavigate, "up", "k"),
	Down:           Act("↓/j", "down", PriorityCore, CategoryNavigate, "down", "j"),
	Tab:            Act("tab", "siblings", PriorityCommon, CategoryNavigate),
	ToggleApprover: Act("space", "approver", PriorityCore, CategoryAct),
	Remove:         Act("d", "remove", PriorityCommon, CategoryAct, "d", "delete"),
	Search:         Act("/", "search", PriorityCommon, CategoryAct),
	SetTeam:        Act("T", "set team", PriorityCommon, CategoryAct),
	Confirm:        Act("↵", "save", PriorityCore, CategoryGeneral, "enter"),
	Close:          Act("v/esc", "cancel", PriorityCore, CategoryGeneral, "v", "esc"),
}

// ReviewerEditorCtx is the reviewer editor (list mode) context.
var ReviewerEditorCtx = NewContext("reviewer-editor", "Reviewers", &DefaultReviewerEditorKeyMap,
	WithFooterGroup("↑↓", "move", &DefaultReviewerEditorKeyMap.Up, &DefaultReviewerEditorKeyMap.Down),
)

// ReviewerSearchKeyMap holds keybindings for the reviewer editor's search
// sub-mode. Navigation is arrow-only: j/k/v/q must insert characters into the
// query, not move the cursor or close anything.
type ReviewerSearchKeyMap struct {
	Up      Action
	Down    Action
	Select  Action
	Confirm Action
	Cancel  Action
}

// DefaultReviewerSearchKeyMap is the default keybinding set for reviewer search.
var DefaultReviewerSearchKeyMap = ReviewerSearchKeyMap{
	Up:      Act("↑", "up", PriorityCore, CategoryNavigate, "up"),
	Down:    Act("↓", "down", PriorityCore, CategoryNavigate, "down"),
	Select:  Act("space", "select", PriorityCore, CategoryAct),
	Confirm: Act("↵", "add", PriorityCore, CategoryGeneral, "enter"),
	Cancel:  Act("esc", "cancel", PriorityCore, CategoryGeneral),
}

// ReviewerSearchCtx is the reviewer-search context; it captures text.
var ReviewerSearchCtx = NewContext("reviewer-search", "Reviewer search", &DefaultReviewerSearchKeyMap,
	WithCapturesText(),
	WithFooterGroup("↑↓", "move", &DefaultReviewerSearchKeyMap.Up, &DefaultReviewerSearchKeyMap.Down),
)

// BatchPreviewKeyMap holds keybindings for the batch preview screen.
type BatchPreviewKeyMap struct {
	Up      Action
	Down    Action
	Toggle  Action
	Confirm Action
	Back    Action
}

// DefaultBatchPreviewKeyMap is the default keybinding set for the batch preview screen.
var DefaultBatchPreviewKeyMap = BatchPreviewKeyMap{
	Up:      Act("↑/k", "up", PriorityCore, CategoryNavigate, "up", "k"),
	Down:    Act("↓/j", "down", PriorityCore, CategoryNavigate, "down", "j"),
	Toggle:  Act("space", "include", PriorityCore, CategoryAct),
	Confirm: Act("↵", "apply", PriorityCore, CategoryGeneral, "enter"),
	Back:    Act("esc", "back", PriorityCore, CategoryGeneral),
}

// BatchPreviewCtx is the batch preview context.
var BatchPreviewCtx = NewContext("batch-preview", "Batch preview", &DefaultBatchPreviewKeyMap,
	WithFooterGroup("↑↓", "move", &DefaultBatchPreviewKeyMap.Up, &DefaultBatchPreviewKeyMap.Down),
)

// BuildCustomCommandsContext builds the context for user-configured external
// commands (docs/adr/0004-external-command-launcher.md). Every configured
// command is PriorityModal (never competes for footer space) and CategoryAct
// (grouped with refresh/open MR/reviewers/diff in the '?' help modal). Callers
// push the returned context above BoardCtx so a configured key colliding with
// a board default shadows it via the existing stacking rule.
func BuildCustomCommandsContext(cmds []config.Command) *Context {
	actions := make([]*Action, 0, len(cmds))
	for _, cmd := range cmds {
		a := Act(cmd.Key, cmd.Name, PriorityModal, CategoryAct)
		actions = append(actions, &a)
	}
	return NewDynamicContext("custom-commands", "Commands", actions)
}
