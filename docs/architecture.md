# Architecture

## Package dependency rules

Ports-and-adapters (hexagonal) layout. Dependency arrows point inward — outer layers depend on
inner abstractions, never the reverse.

```
cmd/mrboard/main.go
  └── internal/cmd/mrboard      (Cobra commands; boots core, launches TUI)
        ├── internal/core        (composition root — wires config → adapters → stores)
        │     ├── internal/config
        │     ├── pkg/gitlab                (REST + GQL client; no domain imports)
        │     └── internal/adapters/gitlabadpt   (implements mrsvc.MergeRequestSource)
        └── internal/tui         (Bubble Tea TUI — only layer that imports charmbracelet)
              └── internal/domain/service/mrsvc  (port interfaces owned by business layer)

internal/domain                  (stdlib only — zero non-stdlib imports)
internal/domain/service/mrsvc    (port interfaces; imports only internal/domain)
pkg/gitlab                       (REST + GQL client; imports only stdlib + net/http libs)
internal/adapters/gitlabadpt     (implements mrsvc; imports pkg/gitlab + internal/domain)
internal/adapters/statestore     (implements domain.StateStore; stdlib + file I/O)
internal/adapters/snapshotstore  (implements domain.SnapshotStore; stdlib + file I/O)
internal/core                    (composition root; no TUI imports)
internal/tui                     (charmbracelet v2; depends on mrsvc interfaces, never on adapters)
```

`internal/tui` depends on `mrsvc.MergeRequestSource` (the port), not on any adapter directly.
This keeps every backend swappable and makes the TUI fully unit-testable with generated mocks.

The same rule applies to every other vendor integration (JIRA, Teams, ...), not just GitLab: a
concrete vendor name may appear as a Go identifier only inside that vendor's own adapter package,
in `internal/domain` (shared business rules that legitimately encode a vendor's data shape, e.g.
`domain.ExtractJiraID`), and in `internal/core` (the composition root, which must name concrete
adapters to construct them). Service ports (`internal/domain/service/*svc`) and `internal/tui`
name the capability, not the vendor — see AGENTS.md rule 7 for the full statement and exceptions
(config schema, user-visible display text).

## Data flow

```
cmd/mrboard/main.go
  → config.Load()                         reads mrboard.yaml / $MRBOARD_CONFIG
  → core.New(ctx, cfg)                    wires logger → pkg/gitlab.Client → gitlabadpt →
                                             statestore → snapshotstore
  → tui.New(ctx, cfg, core.MRSource, core.StateStore, core.SnapshotStore, ...)
  → tea.NewProgram(model).Run()
```

On startup the TUI boots from `domain.SnapshotStore.Load()` (see
`docs/adr/0005-incremental-fetch-and-selection-identity.md`): a warm cache renders the board
immediately, fully interactive, at any age, while a `FetchAllCmd` runs in the background. A cold
cache (nothing to load) is the only case that shows the loading state. `FetchAllCmd` calls
`MergeRequestSource.FetchAll(ctx, mrsvc.FetchOptions{Previous: m.allMRs, ...})`, passing the
current in-memory snapshot so `gitlabadpt` can diff `UpdatedAt` and skip re-fetching discussions
for unchanged MRs. Every landed `FetchResultMsg` is persisted via `SnapshotStore.Save`. Manual
refresh (`r`) and the `refresh_interval` timer both repeat the same cycle; a tick is skipped while
a fetch is already in flight.

Detail panel (`↵`) calls `MergeRequestSource.GetDetail(ctx, projectID, mrIID)`.

Diff view (`d`) calls `MergeRequestSource.GetDiff(ctx, projectID, mrIID)`, then lazily calls
`MergeRequestSource.GetFileContent(ctx, projectID, path, ref)` per file on demand.

Approver editor (`a`) calls `GetProjectMembers` / `SaveApprovers` and re-fetches the affected
MR via `FetchMR` after a successful write.

## File layout

```
mrboard/
  cmd/mrboard/
    main.go                # Signal handling; calls mrboardcmd.Execute
  internal/
    cmd/mrboard/
      root.go              # Cobra root command; boots core, passes to board
      board.go             # `mrboard run` subcommand
      fetch.go             # Background fetch loop
      version.go           # `mrboard version` subcommand
    config/
      config.go            # AppConfig, Load(), typed sub-config accessors
    core/
      core.go              # Composition root — builds and wires all dependencies
    domain/
      mr.go                # All domain types (see domain-model.md)
      state.go             # StateStore + SnapshotStore interfaces
      service/mrsvc/
        mrsvc.go           # MergeRequestSource port + SourceType, Source, Config
        filter.go          # MR filtering helpers
        mocks/             # mockery-generated doubles
    adapters/
      gitlabadpt/
        gitlabadpt.go      # MergeRequestSource implementation (REST + GQL)
        mapper.go          # Maps pkg/gitlab types → domain.MergeRequest
        dedup.go           # Cross-source deduplication
      statestore/
        statestore.go      # domain.StateStore on local disk (XDG data dir)
      snapshotstore/
        snapshotstore.go   # domain.SnapshotStore — versioned JSON cache (XDG cache dir)
    log/
      log.go               # slog wrapper (file + stderr)
    tui/
      keys.go              # All KeyMap types — keybindings live here only
      styles.go            # Styles struct — all lipgloss styles live here only
      model.go             # Root tea.Model — program state, message routing
      board.go             # Board widget — column layout, cross-column focus
      column.go            # Column widget — one per MRPhase
      card.go              # MR card widget — one per domain.MergeRequest
      detail.go            # Detail panel widget — description + discussion threads
      diff_view.go         # Full-screen diff view widget (press d)
      approver_editor.go   # Approver editor overlay (press a)
      filter_popup.go      # Filter popup overlay (press f)
      theme_picker.go      # Theme picker overlay (press t)
      footer.go            # Help/keybinding bar
      header.go            # Header bar (title + stats)
      spinner.go           # Loading overlay
      state.go             # Shared TUI state types
      viewport.go          # Viewport helper
      theme.go             # Theme application to Styles
      themes.go            # Built-in theme definitions
  pkg/
    gitlab/
      client.go            # Authenticated REST + GQL client
      graphql.go           # GraphQL query helpers
      config.go            # pkg/gitlab.Config
    theme/
      model.go             # Theme model
      theme.go             # Token → color resolution
  docs/
    architecture.md        # This file
    domain-model.md
    tui-conventions.md
    clean_architecture.md
    adr/                   # Architecture Decision Records
  mrboard.yaml.example
  CLAUDE.md
```

## Dependencies

| Package | Purpose |
|---|---|
| `charm.land/bubbletea/v2` | TUI event loop (Elm architecture) |
| `charm.land/lipgloss/v2` | Terminal styling |
| `charm.land/bubbles/v2` | Pre-built widgets (spinner, key bindings, help, viewport) |
| `github.com/spf13/viper` | YAML config loading + env-variable binding |
| `github.com/go-ozzo/ozzo-validation/v4` | Declarative config validation |
| `github.com/spf13/cobra` | CLI command structure |

## Config

Loaded from `~/.config/mrboard/mrboard.yaml` (XDG), `./mrboard.yaml`, or path in `$MRBOARD_CONFIG`.
`$GITLAB_TOKEN` overrides `gitlab.token` from the config file.

```yaml
gitlab:
  url: https://gitlab.example.com
  token: glpat-xxx          # or set $GITLAB_TOKEN; needs api scope for write operations
  timeout: 30s              # default: 30s

sources:
  - type: group
    ids: [my-team]

  - type: user
    ids: [alice, bob]

excluded_authors:
  - renovate-bot

current_user: alice

log:
  path: /tmp/mrboard.log    # optional; omit to disable
  level: info               # debug | info | warn | error
```
