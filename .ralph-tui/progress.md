# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

### Charmbracelet v2 widget pattern
Every widget is a value receiver struct implementing `Init() tea.Cmd`, `Update(tea.Msg) (tea.Model, tea.Cmd)`, `View() tea.View`. Pointer receiver helper methods (SetFocused, SetWidth, SetCards) mutate state; the tea.Model methods use value receivers. Root model is the only one that sets `v.AltScreen = true`.

### Lipgloss rendering without importing lipgloss in leaf files
Leaf widgets (card.go, column.go) call `.Render()` on styles stored in a `Styles` struct without importing lipgloss themselves — the compiler infers the type. Only files that create styles or use lipgloss constants (`JoinHorizontal`, `Place`) need the import.

### Spinner update delegation
Spinner ticks are unhandled messages caught in `default:` in root model's switch. Only propagated when `state == stateLoading` to avoid wasted CPU.

---

## 2026-05-04 - mrr-88x.13
- Implemented entire `internal/tui` package: keys.go, styles.go, card.go, column.go, board.go, footer.go, spinner.go, model.go
- Created `cmd/mrboard/main.go` wiring config → gitlab client → tui.Model → tea.NewProgram
- Added charmbracelet v2 dependencies: `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`
- Files changed: internal/tui/* (8 new files), cmd/mrboard/main.go, go.mod, go.sum
- **Learnings:**
  - `charm.land/bubbles/v2` resolves correctly (not `github.com/charmbracelet/bubbles/v2`)
  - `tea.NewProgram(m)` takes no options in v2; AltScreen is set on the `tea.View` returned by root model
  - `key.Matches(msg, binding)` accepts `tea.KeyPressMsg` directly in v2 — no changes needed
  - `spinner.Model.Update()` returns `(spinner.Model, tea.Cmd)` (concrete type), not `(tea.Model, tea.Cmd)`, unlike custom widgets
  - `lip.Place(w, h, lip.Center, lip.Center, str)` works for centering loading/error overlays
  - Package boundary rules fully enforced: domain=stdlib only, config/gitlab=no charmbracelet, tui=only charmbracelet importer
---

