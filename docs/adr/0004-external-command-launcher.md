# ADR-0004: External Command Launcher for MR Cards

**Status**: Accepted — all wayfinder decisions resolved; execution work not yet started

## Context

mrboard shells out to `difft` today as a text filter: `internal/tui/diff_view.go` writes
old/new file content to temp files, captures `difft`'s colored output, and renders that text
inside mrboard's own scrollable pane. Building and maintaining that embedded file-list/diff-view
widget has not been worth its ergonomics, and better dedicated review tools already exist
(`tuicr`, `hunk`) — but those are full interactive TUI programs meant to take over the terminal,
not text filters whose output gets captured.

The destination: add a generic, user-configured external-command launcher — a config-defined
list of named commands, each an argv template (binary + args, no shell interpretation) expanded
against MR metadata, triggered directly from a selected MR card in the kanban board, available
unconditionally on every card. Every command uniformly suspends mrboard's terminal (via
`tea.ExecProcess`, confirmed present in `charm.land/bubbletea/v2@v2.0.7`), hands full control to
the child process, and resumes mrboard on exit — letting tools like `tuicr` or `hunk` be plugged
in without mrboard modeling their behavior. The existing `difft`-based inline diff viewer stays
untouched as the zero-config fallback. mrboard does not resolve a local git checkout path for a
given MR's project — that remains the user's own configured command's responsibility.

## Non-goals

- mrboard posting review comments/approvals back to GitLab itself — scope is launch-only;
  whatever the external tool does once handed the MR is that tool's business.
- Mapping GitLab projects to local git checkout paths — mrboard only supplies MR metadata as
  template variables; locating a local working tree is the user's own command's problem.
- Per-project/per-label conditional visibility of configured commands — decided against in favor
  of a flat, unconditional list available on every card.

## Decision

### Config-driven keybindings vs. the static `Act()` system (resolved 2026-07-30)

mrboard's keybinding system (`docs/keybindings.md`, `internal/tui/keymap.go`/`keys.go`) requires
every binding to be a static `Act(...)` registration in `keys.go`, enforced by
`TestBindingsDefinedOnlyInKeys` — a grep over call *sites*, not over whether arguments are
compile-time literals. This effort's commands are configured by the user in `mrboard.toml` at
runtime, with an a-priori-unknown count and set of keys — incompatible with `NewContext`'s
existing reflection-over-a-fixed-struct registration path, but not actually in conflict with the
"single builder function" invariant itself.

Resolution:

1. **New Context constructor.** Add a slice-based sibling to `NewContext` —
   `NewDynamicContext(name, title string, actions []*Action, opts ...ContextOpt) *Context` in
   `keymap.go` — building the same `byKey` conflict map from a `[]*Action` instead of struct
   reflection. All downstream machinery (`footerItems`, `helpSections`, cross-context shadowing)
   is reused unchanged, since none of it depends on how `actions`/`byKey` were populated.
2. **Construction call site.** Each configured command's `Action` is built via the existing
   `Act(cmd.Key, cmd.Name, PriorityModal, CategoryAct)`, called from a new wrapper function that
   lives in `keys.go` (e.g. `BuildCustomCommandsContext(cmds []config.Command) *Context`) —
   satisfying `TestBindingsDefinedOnlyInKeys` with zero changes to that test, since it checks call
   site, not argument literalness.
3. **Stack placement.** The resulting context is pushed only when `contextStack()` would return
   `[Base, Board]` — becoming `[Base, Board, Custom]` — never in Detail or other overlays. Sitting
   above `Board` means a configured key colliding with a Board default (e.g. `r`) shadows it
   automatically via the *existing* stacking rule: "custom overrides default" requires no new
   override logic, only stack position.
4. **Conflict handling.** Two configured commands claiming the same key is a config-validation
   error in `internal/config` at load time (mrboard refuses to start, with a clear message) — not
   the existing init-time panic, which is reserved for genuine code bugs in `keys.go`, not user
   config mistakes.
5. **Footer/modal policy.** Every configured command is `PriorityModal` (never competes for the
   footer's scarce width, regardless of how many are configured) and `CategoryAct` (grouped with
   `refresh`/`open MR`/`reviewers`/`diff` in the `?` help modal) — appearing there automatically
   once the context is on the stack, no separate wiring needed.

### Verification strategy for the suspend/resume flow (resolved 2026-07-30)

`agent-tui` drives a virtual terminal/PTY, not "mrboard the process" specifically — it screenshots
whatever is drawn to that terminal, regardless of which process currently owns it. That makes the
suspend/resume handoff verifiable with the existing tool, contrary to the initial assumption that
an external child process would be unreachable.

Resolution:

1. A small fixture script is checked permanently into the repo (e.g.
   `scripts/testdata/fixture-command.sh` — prints something distinctive, then exits) as the
   configured command used to exercise this flow, so it can be re-verified any time the mechanism
   changes, not just once at initial implementation.
2. Verification is the existing one-time manual `agent-tui` gate — screenshot mrboard's board,
   trigger the configured command, screenshot the fixture's output, screenshot mrboard's redraw on
   resume — matching how `agent-tui` is used elsewhere in this repo (an interactive check the
   agent runs, not committed automation). `just check` remains the automated regression gate.
3. The `tea.Cmd` wrapping `tea.ExecProcess` and the argv-construction logic follow this project's
   normal table-driven testify conventions — nothing special beyond that.

### Template variable set (resolved 2026-07-30)

Neither `domain.MergeRequest` nor `MRDiff` currently stores source/target branch names — the
`difft` path never needed them, since it diffs file content it already fetched itself. A tool like
`tuicr`/`hunk` operates on a local git checkout, so branch names are the one genuinely new piece of
data this feature needs, not just a convenience.

Resolution:

1. **New domain fields.** Add `SourceBranch` and `TargetBranch` (`string`) to
   `domain.MergeRequest`, populated in `gitlabadpt` from GitLab's `source_branch`/`target_branch`
   MR fields — the same kind of straight vendor-shaped passthrough as the existing
   `DetailedMergeStatus` field.
2. **A named projection, not a reflected struct dump.** Template variables are a stable contract
   the user's `mrboard.toml` depends on; reflecting `domain.MergeRequest` directly would turn every
   future domain field addition (it gains fields regularly — `JiraIssueType`, `ReviewerSource`,
   etc.) into an implicit, unversioned change to that contract. Execution work defines a small,
   explicitly-named template-data type independent of `domain.MergeRequest`'s exact shape, built by
   whichever package ends up constructing the child process's argv.
3. **The variable set itself** (Go template names, `{{.Name}}`): `ProjectPath`, `IID`,
   `SourceBranch`, `TargetBranch`, `WebURL`, `Title`, `Author`. `IID` (not `ID`) because it's the
   project-scoped number GitLab URLs/CLIs use. Nothing else on `domain.MergeRequest` serves the
   launch-a-tool use case — reviewer lists, timestamps, and thread counts stay out of the
   projection; add to it later if a concrete command needs more, rather than exposing the full
   struct speculatively.

### UX for a failing or missing configured command (resolved 2026-07-30)

The existing `notifyCmd`/`NotifyResultMsg`/`handleNotifyResult` pattern in `internal/tui/model.go`
already establishes how this codebase reports the outcome of an async, fire-and-forget action:
log at `Error`/`Info`, surface a `toast.ErrorAlert`/`toast.InfoAlert`. The configured-command flow
reuses that pattern rather than inventing a new one.

Resolution:

1. **Startup check is advisory, not blocking.** Unlike the duplicate-key case (a static,
   unambiguous user mistake in `mrboard.toml`), binary presence is an environment condition:
   `exec.LookPath` at config-load time can legitimately disagree with what's on `PATH` when the
   command actually runs (shell init order, tools installed after mrboard started, etc.). A missing
   binary at startup logs a warning and leaves the command enabled — it does not refuse to start.
2. **Runtime failure reuses the toast pattern, on failure only.** `tea.ExecProcess`'s completion
   callback's `error` already covers both cases uniformly — "binary not found" (`*exec.Error`) and
   "non-zero exit" (`*exec.ExitError`) both surface through the same callback, needing no separate
   pre-flight check at invocation time. A new `CommandResultMsg` (mirroring `NotifyResultMsg`) is
   handled by logging `Error` and showing `toast.ErrorAlert` with the command name and underlying
   error. On success, nothing is shown — the resumed, redrawn board is itself the success signal, so
   an extra toast (unlike `Notify`, which has no other visible effect) would just be noise.

### Package placement for the argv builder (resolved 2026-07-30)

The projection/resolution logic needs both `domain.MergeRequest` and `config.Command`, so it
cannot live in `internal/domain` (stdlib-only) or `internal/domain/service/mrsvc` (imports only
`internal/domain`, per docs/architecture.md).

Resolution:

1. **`internal/tui`.** Of the packages that already import both `internal/domain` and
   `internal/config` (`internal/core` and `internal/tui`), `internal/tui` is the better fit:
   `internal/core` is the composition root, not a home for reusable business logic, and the
   feature's actual `exec` call rides on `tea.ExecProcess` — a bubbletea primitive only
   `internal/tui` may import — so the pure argv builder sits next to its future caller
   (execution work, a later ticket) instead of across a package boundary from it.
2. **Plain function, not a widget.** `internal/tui/command_argv.go` holds a private
   `commandTemplateData` struct (the named projection: `ProjectPath`, `IID`, `SourceBranch`,
   `TargetBranch`, `WebURL`, `Title`, `Author`) and an exported `BuildCommandArgv(mr, cmd)
   ([]string, error)` — no `Init`/`Update`/`View`, since it renders nothing. `internal/tui/filter.go`
   already establishes that plain non-widget helper files are an accepted pattern in this package.
3. **Returns resolved args only, not the binary.** `BuildCommandArgv` returns `cmd.Args` resolved
   via `text/template`, mirroring `exec.Command(name, arg...)`'s own name/args split — the
   execution ticket calls `exec.Command(cmd.Binary, argv...)` directly rather than re-splitting a
   combined slice.

## Consequences

- `internal/tui/keymap.go` gains a second Context-construction path (slice-based, alongside the
  existing reflection-based one); both must stay behaviorally identical downstream.
- Duplicate-key validation for user config lives in `internal/config`, separate from (and with
  different failure semantics than) the existing dev-time panic in `keymap.go`.
- Configured commands are permanently modal-only in the footer — a future request to surface one
  in the footer would need a deliberate policy change, not just a `Priority` tweak.
- `domain.MergeRequest` gains `SourceBranch`/`TargetBranch`; the template-variable set is a
  separate, explicitly-named contract layered on top, not a reflection of the domain type.
- A missing configured-command binary is discoverable at startup (log warning) but never fatal;
  failures are only ever reported after the fact, once control returns to mrboard.
