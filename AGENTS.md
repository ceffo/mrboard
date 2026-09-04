# mrboard — Agent Instructions

mrboard is a Go + Charmbracelet Bubble Tea TUI for viewing GitLab merge request review status
in a kanban board. Primary use: team daily standups.

## Quick orientation

| Layer | Package | Purpose |
|---|---|---|
| Types | `internal/domain` | Pure Go domain types — zero non-stdlib imports |
| Config | `internal/config` | YAML loading and validation |
| API | `pkg/gitlab` | GitLab REST API client |
| Service ports | `internal/domain/service/mrsvc` | Interfaces (ports) owned by the business layer |
| Adapters | `internal/adapters/` | Concrete implementations of service ports |
| Composition root | `internal/core` | Wires config → adapters → stores; no TUI imports |
| UI | `internal/tui` | Bubble Tea TUI — only layer allowed to import charmbracelet |
| CLI | `internal/cmd/mrboard` | Cobra commands; boots core, launches TUI |
| Entry | `cmd/mrboard` | Signal handling + calls `mrboardcmd.Execute` |

**Architecture docs (read before coding):**
- [`docs/architecture.md`](docs/architecture.md) — package boundaries, data flow, dependency rules
- [`docs/domain-model.md`](docs/domain-model.md) — domain types, reviewer state machine, phase rules
- [`docs/tui-conventions.md`](docs/tui-conventions.md) — TUI file structure, widget rules, keybinding conventions
- [`docs/clean_architecture.md`](docs/clean_architecture.md) — generic principles for building a (micro) service in go following a ports-and-adapters architecture. Use that when you need to redesign a significant part of the architecture or when evaluating architectural improvements.
- [`docs/adr/`](docs/adr/) — numbered Architecture Decision Records, one per feature area. The durable record of *why* a design was chosen, including decisions reached via a `/wayfinder` ticket.

## Branching

`main` is protected — new work always starts on a branch, never on `main` directly.

Before making any change, create a branch from `main` named `type/name-of-the-feature`,
where `type` is a conventional-commit type (`feat`, `fix`, `chore`, `docs`, `refactor`,
`test`, `perf`, `ci`, `build` — see [Commit message rules](#commit-message-rules)) and
`name-of-the-feature` is a short kebab-case description of the work.

If the purpose of the work isn't clear enough to derive a branch name from, ask the user
what to call it rather than guessing.

## Quality gates

Every bead must pass before closing (use the justfile — never bare `go` commands):
```
just check      # fmt + lint + build + test
just generate   # regenerate all mocks (run after changing any interface in internal/service)
```

## End of session checklist

Before ending a coding session on this repo:
- If a valid mrboard config is reachable (`./mrboard.yaml`, a path given to `--config`/`-c`, or
  the XDG default at `~/.config/mrboard/mrboard.yaml`), offer to test the change against real data
  before reporting the task done — `agent-tui` for TUI changes, `mrboard fetch` (see the parity
  rule below) for anything else. A green `just check` proves the code compiles and the unit tests
  pass; it does not prove the change behaves correctly against a real board.
- Offer to update the architecture docs (`docs/architecture.md`, `docs/domain-model.md`,
  `docs/tui-conventions.md`, `docs/adr/`) if the change affected behavior, data flow, or a decision
  any of them describe.

Both are offers, not silent actions: get the user's go-ahead before running a real-data test or
editing a doc, same as for any other non-trivial action.

## MANDATORY: TUI testing with agent-tui

**Every TUI change MUST be verified with agent-tui before the task is considered done.**
`just check` only validates compilation and unit tests — it cannot catch runtime key-binding bugs,
layout regressions, or interaction failures. Do not skip this step.

```bash
# Standard loop — always use the wrapper script
agent-tui run /path/to/mrboard/scripts/run-tui.sh
agent-tui wait --stable
agent-tui screenshot          # verify initial state
# exercise the feature: press keys, observe results, screenshot after each action
agent-tui screenshot          # verify outcome
agent-tui kill                # always clean up
```

Key facts for testing:
- Use `agent-tui type " "` to send space (NOT `agent-tui press space` — that errors)
- Use `agent-tui press Enter/Escape/ArrowUp/ArrowDown` for special keys
- Always re-screenshot after each action; element refs go stale after any UI change
- Check counts, labels, and visible state changes — not just "no crash"

## MANDATORY: keep `mrboard fetch` at parity with the TUI

Whenever a change touches how the TUI fetches or derives MR data — `mrsvc.FetchOptions`, the fetch
commands in `internal/tui/model.go`, or anything in `internal/adapters/gitlabadpt` the TUI's fetch
path exercises — update `internal/cmd/mrboard/fetch.go` so `mrboard fetch` keeps behaving the same
way: same saved settings it reads, same snapshot/cache semantics, same flags to override them.

`mrboard fetch` exists so discussion-derived bugs (reviewer state, round trips, open threads, etc.)
can be reproduced and verified against real GitLab data from the command line, without driving the
interactive TUI through agent-tui. Letting it drift out of parity removes that capability silently
— the next debugging session won't know it's gone until it needs it.

## Writing tests

**All test assertions use testify — never call `t.Fatal`, `t.Fatalf`, `t.Error`, or `t.Errorf` directly.**

- `require.*` for PRECONDITIONS — anything that must hold before the test can safely continue
  (an `err != nil` check before using the returned value, a `len(x) != n` check immediately
  before indexing into `x`). A failed `require` stops the test immediately, just like `t.Fatal`.
- `assert.*` for the actual TEST ASSERTIONS — the outcome checks the test exists to make. A
  failed `assert` records the failure and lets the rest of the test keep running, so independent
  checks in the same test all get reported instead of stopping at the first one.
- Reference files for the established call-site and import-grouping style:
  `internal/domain/service/mrsvc/filter_test.go` and `internal/adapters/jiraadpt/jiraadpt_test.go`.
- Import grouping (`gci`, see `.golangci.yml`): stdlib group, blank line, then
  `github.com/stretchr/testify/assert` / `github.com/stretchr/testify/require`, blank line, then
  this repo's own packages (`github.com/ceffo/mrboard/...`).
- `just check` enforces this via `golangci-lint`, including the `lll` line-length rule (120 chars)
  — wrap a long `assert.Xxx(...)` call across two lines rather than exceeding the limit.

## Mocks

**Never write mocks by hand.** All test doubles are generated by **mockery v3** from `.mockery.yml` at the repo root.

### Workflow

1. Add or change an interface in `internal/service/`.
2. Add (or verify) an entry in `.mockery.yml` under `packages:`.
3. Run `just generate` — this runs `mockery` which reads `.mockery.yml`.
4. The generated file lands in `internal/service/mocks/mock_<InterfaceName>.go`.
5. Commit the generated file alongside the interface change.

### Commit message rules

- Use conventional commits format: `<type>(<scope>): <description>`
- Do not reference beads task ids anywhere, those are internal
- The description should be a concise summary of the change, ideally no more than 50 characters

### Using a mock in tests

```go
import "github.com/ceffo/mrboard/internal/service/mocks"

func TestFoo(t *testing.T) {
    src := mocks.NewMockMergeRequestSource(t)
    src.EXPECT().FetchAll().Return([]domain.MergeRequest{...}, nil)
    // ... exercise code under test
}
```

`NewMockMergeRequestSource(t)` registers `AssertExpectations` via `t.Cleanup` automatically — no manual assertion call needed.

### Adding a new interface to .mockery.yml

```yaml
packages:
  github.com/ceffo/mrboard/internal/service:
    config:
      dir: "{{.InterfaceDir}}/mocks"
      pkgname: "mocks"
      filename: "mock_{{.InterfaceName}}.go"
      structname: "Mock{{.InterfaceName}}"
    interfaces:
      MergeRequestSource:   # existing
      MyNewPort:            # add new entries here
```

### Prerequisites (one-time install)

```bash
brew install mockery          # or go install github.com/vektra/mockery/v3@latest
go install golang.org/x/tools/cmd/goimports@latest
```

## TUI verification with agent-tui

After TUI changes, verify visually with agent-tui (installed at `/opt/homebrew/bin/agent-tui`):

```bash
# Always use scripts/run-tui.sh — it sets cwd to project root before launching
agent-tui run /path/to/mrboard/scripts/run-tui.sh

# Standard loop
agent-tui wait --stable
agent-tui screenshot       # inspect layout
agent-tui kill             # always clean up
```

The script at `scripts/run-tui.sh` `cd`s to the project root and execs `./bin/mrboard` with no
arguments (the root command launches the board directly — there is no `run` subcommand), so the
binary finds `mrboard.yaml`. **Never point agent-tui at the binary directly** — it won't find
the config file.

## Non-negotiable rules

1. `internal/domain` — stdlib only. No exceptions.
2. `internal/config` and `pkg/gitlab` — no charmbracelet imports.
3. All keybindings defined in `internal/tui/keys.go` as `Act(...)` actions registered in contexts (see `docs/keybindings.md`). No hardcoded key strings or `key.NewBinding` elsewhere — enforced by tests.
4. All lipgloss styles defined in `internal/tui/styles.go`. No inline `lipgloss.NewStyle()` calls in widgets.
5. Every TUI widget is a self-contained struct with its own `Init`, `Update`, `View`. No monolithic root Update.
6. Config loaded from `./mrboard.yaml`, the XDG default, or a path given to `--config`/`-c`. PAT also overridable via `$GITLAB_TOKEN`.
7. **No vendor bleeding.** A concrete vendor name (`Jira`, `GitLab`, `Teams`, ...) may appear as a Go identifier
   (type, interface, field, function, message name) in exactly three places:
   - that vendor's own adapter package (e.g. `internal/adapters/jiraadpt`, `internal/adapters/gitlabadpt`) —
     the adapter's whole job is translating vendor specifics into generic ports;
   - `internal/domain` — shared pure business rules that legitimately encode a vendor's data *shape* are fine here
     (e.g. `domain.ExtractJiraID` parses a ticket-key pattern out of an MR title; `domain.HasJiraLink` recognizes the
     back-link marker). This is the one exception to rule 1's "stdlib only" being about imports, not naming;
   - `internal/core` (and CLI wiring in `internal/cmd/`) — the composition root necessarily names concrete adapters
     to construct them.

   Everywhere else — service ports (`internal/domain/service/*svc`), `internal/tui`, other adapters — must name
   the *capability*, not the vendor: `ticketsvc.TicketLinker`, not `jirasvc.JiraLinker`; `mrsvc.UpdateDescription`,
   not `mrsvc.InjectJiraLink`. Cross-vendor data crosses a port as primitive values (a title string, a URL string),
   never as an imported vendor type or a vendor-named parameter.
   Exception: config schema (`internal/config`'s `Jira` struct, the `jira:` YAML key, `$JIRA_TOKEN`) stays
   vendor-named — it's the user-facing contract for the one tool actually integrated, not internal wiring.
   User-visible strings (toast text, log messages, keybinding *labels* like `"open jira"`) may also name the
   vendor — that's display text for a human, not architecture.
   **Before adding a method to a port or a field to the TUI, ask: does this name make sense if we swapped the
   vendor out tomorrow?** If not, it belongs in the adapter, not the port.


## Engram memory: YOU MUST COMPLY WITH THIS

At the **start of every session**, retrieve context before touching any code:
```bash
# 1. detect project
mcp__plugin_engram_engram__mem_current_project

# 2. load recent session history
mcp__plugin_engram_engram__mem_context

# 3. search for relevant prior decisions if working on a specific area
mcp__plugin_engram_engram__mem_search  query="<topic>"
```

During the session, **save proactively** after every significant decision, bug fix, or discovery:
```
mcp__plugin_engram_engram__mem_save  title="..."  type="decision|bugfix|pattern|architecture"
```

At the **end of every session** (before saying "done"), save a summary:
```
mcp__plugin_engram_engram__mem_session_summary  content="## Goal\n...\n## Discoveries\n...\n## Accomplished\n..."
```

Never skip these steps. Memory is how work persists across context resets.

## Token saving strategies: YOU MUST COMPLY WITH THIS

Tokens are scarce and costly. You should do your best not to squander them.

- use `/file-search` skill instead of find and grep 
- don't read whole files if you don't need to. assess the size of a file with `wc` before hand.
- use `toon` for raw JSON outputs (e.g. `bv --robot-* | toon`); `br` commands use `--format toon` flag instead — never combine both
- use `/caveman` skill to save on tokens 

## Minimizing interactions 

IMPORTANT: Do your best not to bother the user with constant need for authorizing commands.
DO NOT go for ad-hoc python commands whenever you feel so. If you need more tooling, propose to build them once in scripts and/or skills to be able to reuse them.

<!-- br-agent-instructions-v1 -->

---

## Beads Workflow Integration

This project uses [beads_rust](https://github.com/Dicklesworthstone/beads_rust) (`br`/`bd`) for issue tracking. Issues are stored in `.beads/` and tracked in git.

### Essential Commands

```bash
# View ready issues (open, unblocked, not deferred)
br ready              # or: bd ready

# List and search
br list --status=open # All open issues
br show <id>          # Full issue details with dependencies
br search "keyword"   # Full-text search

# Create and update
br create --title="..." --description="..." --type=task --priority=2
br update <id> --status=in_progress
br close <id> --reason="Completed"
br close <id1> <id2>  # Close multiple issues at once

# Sync with git
br sync --flush-only  # Export DB to JSONL
br sync --status      # Check sync status
```

### Workflow Pattern

1. **Start**: Run `br ready` to find actionable work
2. **Claim**: Use `br update <id> --status=in_progress`
3. **Work**: Implement the task
4. **Complete**: Use `br close <id>`
5. **Sync**: Always run `br sync --flush-only` at session end

### Key Concepts

- **Dependencies**: Issues can block other issues. `br ready` shows only open, unblocked work.
- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (use numbers 0-4, not words)
- **Types**: task, bug, feature, epic, chore, docs, question
- **Blocking**: `br dep add <issue> <depends-on>` to add dependencies

### Session Protocol

**Before ending any session, run this checklist:**

```bash
git status              # Check what changed
git add <files>         # Stage code changes
br sync --flush-only    # Export beads changes to JSONL
git commit -m "..."     # Commit everything
git push                # Push to remote
```

### Best Practices

- Check `br ready` at session start to find available work
- Update status as you work (in_progress → closed)
- Create new issues with `br create` when you discover tasks
- Use descriptive titles and set appropriate priority/type
- Always sync before ending session

<!-- end-br-agent-instructions -->
