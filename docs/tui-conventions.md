# TUI Conventions

The TUI lives entirely in `internal/tui/`. Only this package may import charmbracelet libraries.

## Ecosystem versions — import paths

We target the **v2** ecosystem. Do not use v1 import paths. Mixing v1 and v2 is a compile error.

| Library | v1 (do NOT use) | v2 (use this) |
|---|---|---|
| Bubble Tea | `github.com/charmbracelet/bubbletea` | `charm.land/bubbletea/v2` |
| Lip Gloss | `github.com/charmbracelet/lipgloss` | `charm.land/lipgloss/v2` |
| Bubbles | `github.com/charmbracelet/bubbles` | `charm.land/bubbles/v2` ⚠️ |

⚠️ Bubbles v2 import path: the upgrade guide confirms bubbletea and lipgloss moved to `charm.land`.
Bubbles is expected to follow. **Verify against `go.mod` when first running `go get`** — if
`charm.land/bubbles/v2` resolves, use it; otherwise fall back to `github.com/charmbracelet/bubbles/v2`.

## v2 breaking changes you must know

These are hard API differences from v1. Do not write v1-style code.

**`View()` returns `tea.View`, not `string`:**
```go
// WRONG (v1)
func (m model) View() string { return "..." }

// CORRECT (v2)
func (m model) View() tea.View { return tea.NewView("...") }
```

**AltScreen and mouse are set in `View()`, not `NewProgram()`:**
```go
// WRONG (v1)
tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

// CORRECT (v2) — root model's View() only
func (m rootModel) View() tea.View {
    v := tea.NewView(m.renderBoard())
    v.AltScreen = true
    return v
}
// NewProgram takes no options:
tea.NewProgram(model{})
```

**Key events use `tea.KeyPressMsg`, not `tea.KeyMsg`:**
```go
// WRONG (v1)
case tea.KeyMsg:
    switch msg.String() { ... }

// CORRECT (v2)
case tea.KeyPressMsg:
    switch msg.String() { ... }
```

**`key.Matches` works the same in v2** — the `bubbles/key` API is unchanged.

## Bubbles widget inventory

Use widgets from the bubbles library before building your own. Reference:

| Widget | Package | Use in mrboard |
|---|---|---|
| `spinner.Model` | `bubbles/spinner` | ✅ Loading overlay in `spinner.go` |
| `key.Binding` | `bubbles/key` | ✅ All keybindings in `keys.go` |
| `help.Model` | `bubbles/help` | ✅ Footer keybinding bar in `footer.go` |
| `viewport.Model` | `bubbles/viewport` | ✅ Column scroll if card count exceeds height |
| `list.Model` | `bubbles/list` | ❌ Too opinionated — build own card list |
| `table.Model` | `bubbles/table` | ❌ Not a kanban layout |
| `textinput.Model` | `bubbles/textinput` | ❌ No text input needed |
| `progress.Model` | `bubbles/progress` | ❌ Not needed |
| `paginator.Model` | `bubbles/paginator` | ❌ Not needed |

**Do not reimplement spinner, key matching, or help rendering.** These are provided by bubbles.

### Spinner usage pattern

```go
import "charm.land/bubbles/v2/spinner" // adjust path after go.mod resolution

type loadingModel struct {
    spinner spinner.Model
}

func newLoadingModel() loadingModel {
    s := spinner.New()
    s.Spinner = spinner.Dot
    return loadingModel{spinner: s}
}

func (m loadingModel) Init() tea.Cmd     { return m.spinner.Tick }
func (m loadingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    m.spinner, cmd = m.spinner.Update(msg)
    return m, cmd
}
func (m loadingModel) View() tea.View    { return tea.NewView(m.spinner.View()) }
```

### Help/footer usage pattern

```go
import "charm.land/bubbles/v2/help"
import "charm.land/bubbles/v2/key"

type footerModel struct {
    help help.Model
    keys KeyMap
}

func (m footerModel) View() tea.View {
    return tea.NewView(m.help.View(m.keys))
}
```

`KeyMap` must implement `help.KeyMap` (i.e. `ShortHelp() []key.Binding` and
`FullHelp() [][]key.Binding`) — do this in `keys.go`.

## File responsibilities

| File | Owns |
|---|---|
| `keys.go` | All keybindings — one `KeyMap` struct, nowhere else |
| `styles.go` | All lipgloss styles — one `Styles` struct, nowhere else |
| `model.go` | Root `tea.Model` — program state, child composition, message routing |
| `board.go` | Board widget — column layout, cross-column focus |
| `column.go` | Column widget — one per `MRPhase`, owns its card list |
| `card.go` | Card widget — renders one `domain.MergeRequest` |
| `footer.go` | Footer bar — renders active keybindings via `help.Model` |
| `spinner.go` | Loading overlay |

## Widget contract

Every widget must be a struct implementing:

```go
func (w *Widget) Init() tea.Cmd
func (w *Widget) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (w *Widget) View() tea.View
```

Only the **root model's** `View()` sets `AltScreen = true`. Child widgets return plain
`tea.NewView(content)` — the root composes them with Lip Gloss and wraps the final string.

Widgets carry their own `focused bool`. When `focused == false`, render with muted styles.
Parent models call `widget.SetFocused(bool)` before delegating `Update`.

Root `model.go` is the only place that handles `tea.WindowSizeMsg` and propagates dimensions
down to children. Widgets never call `tea.WindowSize()` themselves.

## Keybindings — keys.go

Use `charm.land/bubbles/v2/key` (verify path — see import table above):

```go
type KeyMap struct {
    Up      key.Binding
    Down    key.Binding
    Left    key.Binding
    Right   key.Binding
    Refresh key.Binding
    Open    key.Binding
    Quit    key.Binding
}

var DefaultKeyMap = KeyMap{
    Up:      key.NewBinding(key.WithKeys("up", "k"),   key.WithHelp("↑/k", "up")),
    Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
    Left:    key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
    Right:   key.NewBinding(key.WithKeys("right", "l"),key.WithHelp("→/l", "right")),
    Refresh: key.NewBinding(key.WithKeys("r"),         key.WithHelp("r", "refresh")),
    Open:    key.NewBinding(key.WithKeys("o"),         key.WithHelp("o", "open in browser")),
    Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"),key.WithHelp("q", "quit")),
}
```

No other file may use raw string key checks (`msg.String() == "r"`). Always use `key.Matches`.

## Styles — styles.go

```go
type Styles struct {
    // Column
    ColumnHeader         lipgloss.Style
    ColumnBorder         lipgloss.Style
    ColumnBorderFocused  lipgloss.Style

    // Card
    Card                 lipgloss.Style
    CardFocused          lipgloss.Style
    CardTitle            lipgloss.Style
    CardMeta             lipgloss.Style

    // Reviewer pills
    PillNotStarted       lipgloss.Style
    PillCommented        lipgloss.Style
    PillReReview         lipgloss.Style
    PillApproved         lipgloss.Style

    // Status
    DurationUrgent       lipgloss.Style  // > 2 days
    DurationWarning      lipgloss.Style  // > 1 day
    DurationOk           lipgloss.Style
}
```

Instantiated once in `NewStyles()` and passed into every widget at construction time.

## Message types

Custom messages are defined in `model.go`:

```go
type FetchResultMsg struct {
    MRs    []domain.MergeRequest
    Errors []error
}

type FetchErrMsg struct{ Err error }
```

The refresh `tea.Cmd` returns one of these. Widgets receive them via the normal `Update` dispatch.

## Layout

```
┌─────────────────────────────────────────────────────────────┐
│  Draft (0)   │  Needs Review (2)  │  Needs Author (1)  │  Ready (1)  │
│              │  ┌──────────────┐  │  ┌──────────────┐  │  ┌───────┐  │
│  (empty)     │  │ MR title...  │  │  │ MR title...  │  │  │ ...   │  │
│              │  │ @author  2d  │  │  │ @author  4h  │  │  └───────┘  │
│              │  │ [alice ⏳ 1d]│  │  │ [bob 💬 3h]  │  │             │
│              │  └──────────────┘  │  └──────────────┘  │             │
│              │  ┌──────────────┐  │                     │             │
│              │  │ ...          │  │                     │             │
│              │  └──────────────┘  │                     │             │
├─────────────────────────────────────────────────────────────────────────┤
│  ↑/↓ navigate  ←/→ switch column  r refresh  o open  q quit           │
└─────────────────────────────────────────────────────────────────────────┘
```

Reviewer pill icons: `⏳` not_started · `💬` commented · `🔄` re_review_requested · `✓` approved

## UX rules

- Focus starts on the first non-empty column's first card at startup
- `↑`/`↓` wraps within a column (bottom → top, top → bottom)
- `←`/`→` moves to the nearest card in the adjacent column (same row index, clamped)
- Refresh (`r`) shows the spinner overlay; board is not interactive while loading
- If the focused card is removed after a refresh, focus moves to the card above it (or the column header if empty)
- Error messages are shown inline below the board, not as a modal
