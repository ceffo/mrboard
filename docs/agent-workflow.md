# Agent Workflow Configuration

Project-specific settings for the generic `developer`, `code-reviewer`, `tester`, `scoper`, `designer`
and `supervisor` agents in `.claude/agents/`. The agents hold no project knowledge of their own: they
read this file (linked from `AGENTS.md`) and the docs it points to. **Section headings below are a
contract; keep them stable.**

## Project docs to read

| Always | `AGENTS.md`, `docs/domain-model.md`, `docs/architecture.md` |
|---|---|
| TUI work (`internal/tui`) | `docs/tui-conventions.md`, `docs/keybindings.md` |
| Fetch path (`internal/adapters/gitlabadpt`, `pkg/gitlab`, `mrsvc.FetchOptions`) | `docs/adr/0005-incremental-fetch-and-selection-identity.md`, the `mrboard fetch` parity rule in `AGENTS.md` |
| Config (`internal/config`) | `docs/configuration.md` |
| Build, tooling, releases | `docs/development.md`, `docs/adr/0011-auto-release-on-merge.md` |
| Design decisions | `docs/adr/` (numbered, one ADR per feature area; the issue body names the ADR section it builds) |

Design decisions are read from these docs and the tracker, never from a memory tool.

## Validation

- Full gate, from the repo root: `just check` (fmt + lint + build + test + release-script self-test).
  Mandatory before any work item is reported done. CI runs `just check-ci`, the same gate but failing
  on unformatted code instead of reformatting it.
- Narrow checks while iterating: `go test ./internal/<pkg>/... -run <TestName>` and
  `go tool golangci-lint run ./internal/<pkg>/...`. Never as a substitute for `just check`, and never
  a bare `go`, `golangci-lint` or `mockery` call in the justfile or a workflow.
- TUI items: `just build`, then drive the TUI headless with `agent-tui` against the demo dataset (see
  "Running the app"). Save each `agent-tui screenshot` (text) to
  `<worktree root>/reports/<KEY>-screens/<step>.txt` and list the files in the report.
- **Never open a visible window on the user's desktop.** UI checks run headless: `agent-tui` drives
  the TUI in its own PTY; never launch `mrboard` in a terminal tab or window.
- Hard gates:
  - Every TUI change is verified with `agent-tui` before it is reported done; `just check` cannot catch
    key-binding, layout or interaction bugs.
  - A change to how MRs are fetched or derived keeps `mrboard fetch` at parity
    (`internal/cmd/mrboard/fetch.go`): same saved settings, cache semantics and override flags.
  - The PR title is a release decision: run `just release-preview "<title>"` and put its level and
    version in the report.

## Worktree setup

- Worktree root: `/Users/moncefnaji/code/github.com/mrboard.worktrees/` beside the checkout (briefs
  in `briefs/`, reports in `reports/`, supervisor ledger in `supervisor/ledger.md`); these are the
  defaults of `fleet-lib.sh`.
- Main checkout: `/Users/moncefnaji/code/github.com/mrboard`; default branch `main`; remote `origin`
  (GitHub `ceffo/mrboard`).
- After creating a worktree: `just build` (agent-tui scripts run `./bin/mrboard`). Go tools come from
  `go.mod` `tool` directives; nothing to install.
- Shared resources: the user's real config (`~/.config/mrboard/mrboard.yaml`), its `state.yaml`, and
  the snapshot and ticket caches under the XDG cache dir. Parallel worktrees never touch them: every
  run uses `--demo`, whose stores are in memory. Nothing else is shared (no ports, no databases).
- `.beads/` is the retired tracker kept for history. Agents never run `br` or `bv`.

## Running the app for acceptance checks

- Build: `just build` → `./bin/mrboard`.
- TUI: `agent-tui run <worktree>/scripts/demo-tui.sh [dataset]`, then `agent-tui wait --stable`,
  `agent-tui screenshot`, keys with `agent-tui press Enter|Escape|ArrowUp|...` (`agent-tui type " "`
  for space), and `agent-tui kill` to stop. Re-screenshot after every action and check counts, labels
  and state, not just "no crash". Datasets are the YAML files in
  `internal/adapters/demoadpt/fixture/` (`docs/adr/0006-demo-mode.md`); add a fixture when a
  criterion needs data the existing ones lack.
- CLI: `./bin/mrboard --demo <subcommand> [flags]` (e.g. `fetch`, `load`).
- Never `scripts/run-tui.sh`, `just run` or `just fetch`: they read the user's real config and hit
  real GitLab and Jira. Testing against real data is offered to the user by the supervisor, never
  run by an agent.

## Generated code

| Trigger | Command |
|---|---|
| An interface listed in `.mockery.yml` is added or changed, or `.mockery.yml` changes | `just generate`; commit the regenerated `mocks/mock_<Interface>.go` with the interface change |
| `just tools-update` bumped mockery | `just generate`; commit `go.mod`/`go.sum` and the mocks together |

Mocks are never written by hand.

## Tracker

Backend: `github` (`FLEET_TRACKER` in `.claude/fleet.env`). Every agent uses
`.claude/scripts/tracker.sh` (`show`, `list`, `ready`; the supervisor and scoper also `create`,
`update`, `block`, `comment`, `state`, `setup`). Run it without arguments for the usage.

- Work item = a tracker issue; id `#n`. File-safe key for branches, briefs and reports: `GH-n`.
- States: Todo (open) -> In Progress (claimed by the supervisor at dispatch) -> In Review (PR opened, by
  `ship.sh`) -> Done (the PR's `Closes <id>` line closes it on merge; nobody closes by hand). A PR closed
  unmerged goes back to Todo. Deferred work is Backlog plus the label `deferred`.
- Priority: Urgent / High / Medium / Low / None.
- Labels: `needs-info` (cannot start: waiting on the user; the description holds a "## Questions"
  section, each question with the default applied meanwhile), `risky` (needs review), `epic`, `deferred`,
  `question`, types `bug` / `feature` / `chore` / `docs`, plus area labels `area:domain`,
  `area:gitlab`, `area:ticket`, `area:config`, `area:cli`, `area:tui`. `tracker.sh setup` creates
  the process labels. Every open `needs-info` or `question` issue is a pending question the supervisor
  restates to the user in every reply (`.claude/scripts/pending-questions.sh`).
- Hierarchy: an epic is a parent issue labelled `epic` (sub-issues via `--parent`); blocking is a
  "blocked by" relation (`--blocked-by`, `tracker.sh block`).
- Slicing convention: one issue = one PR-sized slice through the layers it needs (domain, port,
  adapter, CLI, TUI), citing the ADR section it implements. Split only when a slice is too big for one
  reviewable PR, and then along a port boundary.
- Defaults when filing autonomously: priority `Low`.
- Only the supervisor and the scoper write to the tracker.

## Scope

Current phase: the **Review load view** epic (`docs/adr/0015-review-load-view.md`). CLI first
(`mrboard load`), then the TUI display mode once the CLI output is settled. Out of scope: navigating to
previous sprints (ADR-0015 Non-goals). Anything outside the epic is filed as backlog and never
dispatched without the user's say-so.

## Hard rules for implementers

- **Never open a visible window on the user's desktop.**
- Never run against the user's real config, GitLab or Jira; acceptance runs use `--demo`.
- `internal/domain` imports the stdlib only; `internal/config` and `pkg/gitlab` import no
  charmbracelet; `internal/tui` depends on ports, never on adapters (`docs/architecture.md`).
- No vendor bleeding: a vendor name (`Jira`, `GitLab`, `Teams`) is a Go identifier only in that
  vendor's adapter, `internal/domain` and `internal/core`/`internal/cmd` (AGENTS.md rule 7). Ports and
  the TUI name the capability.
- Keybindings only as `Act(...)` actions in `internal/tui/keys.go` (enforced by
  `internal/tui/keymap_test.go`); lipgloss styles only in `internal/tui/styles.go`; every widget has its
  own `Init`/`Update`/`View`.
- Tests use testify: `require` for preconditions, `assert` for outcomes, never `t.Fatal`/`t.Error`
  (lint). Imports grouped stdlib / testify / `github.com/ceffo/mrboard/...` (gci). Lines ≤ 120 (lll).
- Mocks are generated by mockery (`just generate`), never hand-written.
- GitLab reviewer and approver writes are deltas applied to live state, never a replacement of the
  whole list.
- Persisted files (`state.yaml`, the snapshot) stay readable: a new field is optional, an existing file
  still loads.
- Fetch-path changes keep `mrboard fetch` at parity in the same PR.
- Comments state durable invariants, never the task or a rejected alternative.
- Commit messages are written to a file and committed with `git commit -F`.

## Review

Needs review (any of): the fetch path (`gitlabadpt` stages, `pkg/gitlab` queries and their batch
sizes); GitLab writes (reviewers, approvers, notes, description, undraft); a persisted format
(`state.yaml`, snapshot); a port interface in `internal/domain/service/*svc` or `internal/domain`; the
config schema; reviewer-state, phase, attention or review-load derivation; a wide refactor.
Not reviewed: docs, tests-only changes, TUI copy and styles, small local refactors.

Reviewer checklist, in priority order:
1. Correctness of the derived review state: reviewer state machine, phase, attention, review size and
   load, against `docs/domain-model.md` and the item's ADR section.
2. Data safety: GitLab writes are deltas on live state; existing `state.yaml` and snapshot files still
   load; no cache serves data its key no longer describes.
3. Port contracts and `mrboard fetch` parity; partial results stay valid (MRs alongside errors).
4. Architecture and layering: import rules, vendor naming, widget structure, keys and styles in their
   one file.
5. Tests prove the criteria and would fail on the old code.
6. **Hard rules for implementers** above; challenge the item's or brief's own design against them.

Severity examples: critical = a write that can drop a reviewer or approver, or a change that makes an
existing state or snapshot file fail to load; high = a wrong reviewer state, phase or review size on
the board, broken `fetch` parity, or a fetch that multiplies API calls per refresh; medium = a hard-rule
violation with no user-visible effect, or an acceptance criterion with no test.

## Model selection

Opus: the fetch path (`gitlabadpt`, `pkg/gitlab` GraphQL), domain derivation (reviewer state, review
load), TUI widgets with their own state, cross-package refactors. Sonnet: everything else (config,
port wiring, CLI flags and output, docs, fixtures).
Reviewer: Sonnet, Opus for the fetch path and domain derivation.

## Parallelism

Two items may run in parallel only if they change no persisted format in common, touch no shared
packages/files/components, don't affect each other's generated output, and share no API contract or
domain struct. In this repo, these serialize: `internal/adapters/gitlabadpt/`, `pkg/gitlab/graphql.go`,
`internal/domain/mr.go` (`MergeRequest`), `mrsvc.FetchOptions`, `internal/tui/model.go`,
`internal/tui/keys.go`, `internal/config/config.go` and `.mockery.yml` with its mocks.

## CI and PRs

- `Check` (`.github/workflows/check.yml`) runs `just check-ci` on pushes to `main` and on PRs **into
  `main`** only: a PR stacked on another branch gets no CI until it is retargeted to `main`. Run
  `just check-ci` locally to reproduce it.
- `Release on merge` runs on every push to `main`: the squashed subject (the PR title) picks the bump
  (`scripts/next-version.sh`), and `Release` publishes the tag. A title with `[skip release]`, or a
  non-releasing type, publishes nothing.

PR body has a **What changes for you locally** line (dev commands, ports, tools, local data, one-off
steps; or "nothing"), a `Closes <id>` line, the `just release-preview` level, and ends with the
project's PR footer: `🤖 Generated with [Claude Code](https://claude.com/claude-code)`. Only the user
merges PRs unless they grant the supervisor `land.sh`; agents never ask whether to merge: a PR is
reported as ready (CI green), nothing more.

## Commits

`<type>(<scope>)?: <short description>` ending with the issue id, e.g.
`feat(load): add the load command (#12)`. Types and their release effect are in `AGENTS.md`
("Releasing"); the description stays near 50 characters. The PR title follows the same format and is
checked with `just release-preview`. Never commit broken tests or a failing build.
Trailer: `Co-Authored-By:` naming the model actually running.

## Memory

Engram project `mrboard`. Only situational know-how belongs there (how to do X, tool gotchas,
environment quirks). Design decisions go in `docs/adr/` and the tracker, never in memory.
