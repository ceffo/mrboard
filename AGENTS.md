# mrboard — Agent Instructions

mrboard is a Go + Charmbracelet Bubble Tea TUI for viewing GitLab merge request review status
in a kanban board. Primary use: team daily standups.

## Quick orientation

| Layer | Package | Purpose |
|---|---|---|
| Types | `internal/domain` | Pure Go domain types — zero non-stdlib imports |
| Config | `internal/config` | TOML loading and validation |
| API | `internal/gitlab` | GitLab REST API client + mapper + fetcher |
| UI | `internal/tui` | Bubble Tea TUI — only layer allowed to import charmbracelet |
| Entry | `cmd/mrboard` | Wires config → fetcher → tui |

**Architecture docs (read before coding):**
- [`docs/architecture.md`](docs/architecture.md) — package boundaries, data flow, dependency rules
- [`docs/domain-model.md`](docs/domain-model.md) — domain types, reviewer state machine, phase rules
- [`docs/tui-conventions.md`](docs/tui-conventions.md) — TUI file structure, widget rules, keybinding conventions

## Quality gates

Every bead must pass before closing:
```
go build ./...
go vet ./...
go test ./...
```

## Non-negotiable rules

1. `internal/domain` — stdlib only. No exceptions.
2. `internal/config` and `internal/gitlab` — no charmbracelet imports.
3. All keybindings defined in `internal/tui/keys.go` using `bubbles/key`. No hardcoded strings elsewhere.
4. All lipgloss styles defined in `internal/tui/styles.go`. No inline `lipgloss.NewStyle()` calls in widgets.
5. Every TUI widget is a self-contained struct with its own `Init`, `Update`, `View`. No monolithic root Update.
6. Config loaded from `./mrboard.toml` or `$MRBOARD_CONFIG`. PAT also overridable via `$GITLAB_TOKEN`.

## Beads

Epic: `mrr-88x` — run with:
```
ralph-tui run --tracker beads-rust --epic mrr-88x
```
