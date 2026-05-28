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
	ToggleView  key.Binding
	Settings    key.Binding
	Approvers   key.Binding
	Diff        key.Binding
	Quit        key.Binding
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Up, k.Down, k.Left, k.Right,
		k.Refresh, k.Open, k.Detail, k.Sort, k.ToggleView, k.Settings, k.Approvers, k.Diff, k.Quit,
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
	ToggleView:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "my view")),
	Settings:    key.NewBinding(key.WithKeys(","), key.WithHelp(",", "settings")),
	Approvers:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "approvers")),
	Diff:        key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diff")),
	Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// FilterPopupKeyMap holds keybindings used inside the filter popup.
type FilterPopupKeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Toggle    key.Binding
	FocusNext key.Binding
	FocusPrev key.Binding
	Close     key.Binding
}

// ShortHelp implements help.KeyMap.
func (k FilterPopupKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Toggle, k.FocusNext, k.Close}
}

// FullHelp implements help.KeyMap.
func (k FilterPopupKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// DefaultFilterPopupKeyMap is the default keybinding set for the filter popup.
var DefaultFilterPopupKeyMap = FilterPopupKeyMap{
	Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Toggle:    key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
	FocusNext: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next section")),
	FocusPrev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev section")),
	Close:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
}

// ThemePickerKeyMap holds keybindings for the theme picker popup.
type ThemePickerKeyMap struct {
	Up        key.Binding
	Down      key.Binding
	FocusNext key.Binding
	FocusPrev key.Binding
	Confirm   key.Binding
	Close     key.Binding
}

// ShortHelp implements help.KeyMap.
func (k ThemePickerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.FocusNext, k.Confirm, k.Close}
}

// FullHelp implements help.KeyMap.
func (k ThemePickerKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// DefaultThemePickerKeyMap is the default keybinding set for the theme picker popup.
var DefaultThemePickerKeyMap = ThemePickerKeyMap{
	Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	FocusNext: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next pane")),
	FocusPrev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev pane")),
	Confirm:   key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("↵/space", "select")),
	Close:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
}

// ApproverEditorKeyMap holds keybindings for the approver editor overlay.
type ApproverEditorKeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Toggle    key.Binding
	FocusNext key.Binding
	FocusPrev key.Binding
	Confirm   key.Binding
	Close     key.Binding
}

// ShortHelp implements help.KeyMap.
func (k ApproverEditorKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Toggle, k.FocusNext, k.Confirm, k.Close}
}

// FullHelp implements help.KeyMap.
func (k ApproverEditorKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// DefaultApproverEditorKeyMap is the default keybinding set for the approver editor overlay.
var DefaultApproverEditorKeyMap = ApproverEditorKeyMap{
	Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Toggle:    key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
	FocusNext: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "all members")),
	FocusPrev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "reviewers")),
	Confirm:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "save")),
	Close:     key.NewBinding(key.WithKeys("a", "esc"), key.WithHelp("a/esc", "close")),
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
	PrevTab key.Binding
	NextTab key.Binding
	Toggle  key.Binding
	Confirm key.Binding
	Close   key.Binding
}

// ShortHelp implements help.KeyMap.
func (k SettingsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.PrevTab, k.NextTab, k.Toggle, k.Confirm, k.Close}
}

// FullHelp implements help.KeyMap.
func (k SettingsKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// DefaultSettingsKeyMap is the default keybinding set for the settings panel.
var DefaultSettingsKeyMap = SettingsKeyMap{
	Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PrevTab: key.NewBinding(key.WithKeys("shift+tab", "left", "h"), key.WithHelp("shift+tab", "prev tab")),
	NextTab: key.NewBinding(key.WithKeys("tab", "right", "l"), key.WithHelp("tab", "next tab")),
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
