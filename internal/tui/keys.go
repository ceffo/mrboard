package tui

import "charm.land/bubbles/v2/key"

// KeyMap contains all keybindings for mrboard in board mode.
type KeyMap struct {
	Up          key.Binding
	Down        key.Binding
	Left        key.Binding
	Right       key.Binding
	Refresh     key.Binding
	Open        key.Binding
	Detail      key.Binding
	CloseDetail key.Binding
	Sort        key.Binding
	Sprint      key.Binding
	ToggleView  key.Binding
	Settings    key.Binding
	Reviewers   key.Binding
	Diff        key.Binding
	Notify      key.Binding
	Jira        key.Binding
	Quit        key.Binding
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Up, k.Down, k.Left, k.Right,
		k.Refresh, k.Open, k.Detail, k.Sort, k.Sprint,
		k.ToggleView, k.Settings, k.Reviewers, k.Diff, k.Notify, k.Jira, k.Quit,
	}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// DetailKeyMap contains keybindings shown in the footer when the detail panel owns focus.
// Left/right are intentionally absent — they are reserved for future section navigation.
type DetailKeyMap struct {
	ScrollUp   key.Binding
	ScrollDown key.Binding
	Close      key.Binding
	Open       key.Binding
	Quit       key.Binding
}

// ShortHelp implements help.KeyMap.
func (d DetailKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{d.ScrollUp, d.ScrollDown, d.Close, d.Open, d.Quit}
}

// FullHelp implements help.KeyMap.
func (d DetailKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{d.ShortHelp()} }

// DefaultKeyMap is the default keybinding set for board mode.
var DefaultKeyMap = KeyMap{
	Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Left:        key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
	Right:       key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
	Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Open:        key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open")),
	Detail:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "details")),
	CloseDetail: key.NewBinding(key.WithKeys("esc", "enter"), key.WithHelp("esc/↵", "close")),
	Sort:        key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort:repo·id↑")),
	Sprint:      key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "sprint filter")),
	ToggleView:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "my view")),
	Settings:    key.NewBinding(key.WithKeys(","), key.WithHelp(",", "settings")),
	Reviewers:   key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "reviewers")),
	Diff:        key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diff")),
	Notify:      key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "notify")),
	Jira:        key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "jira")),
	Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ReviewerEditorKeyMap holds keybindings for the reviewer editor overlay.
type ReviewerEditorKeyMap struct {
	Up             key.Binding
	Down           key.Binding
	ToggleApprover key.Binding
	Remove         key.Binding
	Search         key.Binding
	SetTeam        key.Binding
	Confirm        key.Binding
	Close          key.Binding
}

// ShortHelp implements help.KeyMap.
func (k ReviewerEditorKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.ToggleApprover, k.Remove, k.Search, k.SetTeam, k.Confirm, k.Close}
}

// FullHelp implements help.KeyMap.
func (k ReviewerEditorKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// DefaultReviewerEditorKeyMap is the default keybinding set for the reviewer editor overlay.
var DefaultReviewerEditorKeyMap = ReviewerEditorKeyMap{
	Up:             key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:           key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	ToggleApprover: key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "approver")),
	Remove:         key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "remove")),
	Search:         key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	SetTeam:        key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "set team")),
	Confirm:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "save")),
	Close:          key.NewBinding(key.WithKeys("v", "esc"), key.WithHelp("v/esc", "cancel")),
}

// DefaultDetailKeyMap is the key map shown in the footer when the detail panel is open.
var DefaultDetailKeyMap = DetailKeyMap{
	ScrollUp:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	ScrollDown: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	Close:      key.NewBinding(key.WithKeys("esc", "enter"), key.WithHelp("esc/↵", "close")),
	Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// DiffViewKeyMap holds keybindings for the diff view.
type DiffViewKeyMap struct {
	PrevFile     key.Binding
	NextFile     key.Binding
	ScrollUp     key.Binding
	ScrollDown   key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	Top          key.Binding
	Bottom       key.Binding
	Open         key.Binding
	Close        key.Binding
	Quit         key.Binding
}

// ShortHelp implements help.KeyMap.
func (k DiffViewKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.PrevFile, k.NextFile, k.ScrollUp, k.ScrollDown,
		k.HalfPageUp, k.HalfPageDown, k.Top, k.Bottom,
		k.Open, k.Close, k.Quit,
	}
}

// FullHelp implements help.KeyMap.
func (k DiffViewKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// SettingsKeyMap holds keybindings for the settings panel.
type SettingsKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Left    key.Binding
	Right   key.Binding
	PrevTab key.Binding
	NextTab key.Binding
	Toggle  key.Binding
	Confirm key.Binding
	Close   key.Binding
}

// ShortHelp implements help.KeyMap.
func (k SettingsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Left, k.Right, k.PrevTab, k.NextTab, k.Toggle, k.Confirm, k.Close}
}

// FullHelp implements help.KeyMap.
func (k SettingsKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// DefaultSettingsKeyMap is the default keybinding set for the settings panel.
var DefaultSettingsKeyMap = SettingsKeyMap{
	Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Left:    key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev section")),
	Right:   key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next section")),
	PrevTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
	NextTab: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	Toggle:  key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
	Confirm: key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "apply")),
	Close:   key.NewBinding(key.WithKeys(",", "esc"), key.WithHelp(",/esc", "close")),
}

// DefaultDiffViewKeyMap is the default keybinding set for the diff view.
var DefaultDiffViewKeyMap = DiffViewKeyMap{
	PrevFile:     key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev file")),
	NextFile:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next file")),
	ScrollUp:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	ScrollDown:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "½ page up")),
	HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "½ page down")),
	Top:          key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
	Bottom:       key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
	Open:         key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open")),
	Close:        key.NewBinding(key.WithKeys("d", "esc"), key.WithHelp("d/esc", "close")),
	Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
