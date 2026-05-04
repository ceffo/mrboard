# Architecture

## Package dependency rules

```
cmd/mrboard
    ├── internal/config     (stdlib + BurntSushi/toml only)
    ├── internal/gitlab     (stdlib + xanzy/go-gitlab + internal/config + internal/domain)
    └── internal/tui        (charmbracelet/* + internal/domain)

internal/domain             (stdlib only — the cross-layer contract)
```

`internal/tui` never imports `internal/gitlab` directly. `cmd/mrboard/main.go` fetches data
and passes `[]domain.MergeRequest` into the TUI. This keeps the backend swappable.

## Data flow

```
main.go
  → config.Load()                         reads mrboard.toml / $MRBOARD_CONFIG
  → gitlab.NewClient(cfg)                 builds authenticated API client
  → gitlab.NewFetcher(client, cfg)
  → fetcher.Fetch()                       returns ([]domain.MergeRequest, []error)
  → tui.NewModel(mrs, cfg)
  → tea.NewProgram(model).Run()
```

On manual refresh (`r` key), the TUI emits a `tea.Cmd` that calls `fetcher.Fetch()` again
and sends the result back as a `FetchResultMsg`.

## File layout

```
mrboard/
  cmd/mrboard/main.go
  internal/
    config/
      config.go           # Config struct, Load(), validate()
    domain/
      mr.go               # All domain types (see domain-model.md)
    gitlab/
      client.go           # GitLab API client wrapper (xanzy/go-gitlab)
      mapper.go           # Maps raw API types → domain.MergeRequest
      fetcher.go          # Orchestrates fetches, deduplicates, runs concurrently
    tui/
      keys.go             # Single KeyMap — all bindings live here
      styles.go           # Single Styles struct — all lipgloss styles live here
      model.go            # Root tea.Model — owns program state, composes children
      board.go            # Board widget (manages columns)
      column.go           # Column widget (one per MRPhase)
      card.go             # MR card widget (one per MergeRequest)
      footer.go           # Help/keybinding bar
      spinner.go          # Loading state
  docs/
    architecture.md       # This file
    domain-model.md
    tui-conventions.md
  mrboard.toml.example
  AGENTS.md
```

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/xanzy/go-gitlab` | GitLab REST API client |
| `github.com/charmbracelet/bubbletea` | TUI event loop (Elm architecture) |
| `github.com/charmbracelet/lipgloss` | Terminal styling |
| `github.com/charmbracelet/bubbles` | Pre-built widgets (spinner, key bindings) |
| `github.com/BurntSushi/toml` | TOML config parsing |

## Config

Loaded from `./mrboard.toml` or path in `$MRBOARD_CONFIG`.
`$GITLAB_TOKEN` overrides `gitlab.token` from the config file (useful for CI).

```toml
[gitlab]
url                = "https://gitlab.example.com"
token              = "glpat-xxx"          # or use $GITLAB_TOKEN
required_approvals = 2                    # default: 2

[[sources]]
type = "group"
id   = "my-team"

[[sources]]
type     = "user"
username = "alice"
```
