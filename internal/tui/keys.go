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
	Filter      key.Binding
	Quit        key.Binding
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Up, k.Down, k.Left, k.Right,
		k.Refresh, k.Open, k.Detail, k.Sort, k.ToggleView, k.Filter, k.Quit,
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
	Filter:      key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter")),
	Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// FilterPopupKeyMap holds keybindings used inside the filter popup.
type FilterPopupKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Toggle key.Binding
	Apply  key.Binding
	Cancel key.Binding
}

// ShortHelp implements help.KeyMap.
func (k FilterPopupKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Toggle, k.Apply, k.Cancel}
}

// FullHelp implements help.KeyMap.
func (k FilterPopupKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// DefaultFilterPopupKeyMap is the default keybinding set for the filter popup.
var DefaultFilterPopupKeyMap = FilterPopupKeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Toggle: key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle/select")),
	Apply:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "apply")),
	Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
}

// DefaultDetailKeyMap is the key map shown in the footer when the detail panel is open.
var DefaultDetailKeyMap = DetailKeyMap{
	ScrollUp:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	ScrollDown: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	Close:      key.NewBinding(key.WithKeys("esc", "enter"), key.WithHelp("esc/↵", "close")),
	Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
