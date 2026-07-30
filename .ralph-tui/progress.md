# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

*Add reusable patterns discovered during development here.*

- **Passing a new raw GitLab field through both REST and GraphQL paths**: this repo fetches
  MRs via two independent transports (`gl.MergeRequest` from go-gitlab for REST, and a
  hand-rolled `GQLMergeRequest` struct + query string in `pkg/gitlab/graphql.go` for GraphQL).
  Adding a new passthrough field (e.g. `DetailedMergeStatus`, `SourceBranch`) touches 5 spots:
  domain struct (`internal/domain/mr.go`), the GQL struct tag, BOTH GraphQL query string
  literals (`gqlUserMRsQuery` and `gqlReviewerMRsQuery` — they are separate literals, not
  shared), and both mapper functions (`MapMR` for REST, `MapMRFromGraphQL` for GraphQL) in
  `internal/adapters/gitlabadpt/mapper.go`. The REST side needs no query-string change since
  go-gitlab's `MergeRequest` struct already returns the full REST payload.

---

## 2026-07-30 - mrr-4z4.1
- Added `SourceBranch`, `TargetBranch string` fields to `domain.MergeRequest` (internal/domain/mr.go), same comment style as `DetailedMergeStatus`.
- REST mapper (`MapMR`): passthrough from `gl.MergeRequest.SourceBranch`/`.TargetBranch` (go-gitlab already exposes these on the REST payload — no query change needed).
- GraphQL: added `SourceBranch`/`TargetBranch string` fields (json `sourceBranch`/`targetBranch`) to `pkggitlab.GQLMergeRequest`, added `sourceBranch`/`targetBranch` to both `gqlUserMRsQuery` and `gqlReviewerMRsQuery` string literals, wired into `MapMRFromGraphQL`.
- Added `TestMapMR_SourceTargetBranch_Stored` and `TestMapMRFromGraphQL_SourceTargetBranch_Stored` in mapper_test.go, mirroring the existing `DetailedMergeStatus` passthrough tests.
- Files changed: internal/domain/mr.go, pkg/gitlab/graphql.go, internal/adapters/gitlabadpt/mapper.go, internal/adapters/gitlabadpt/mapper_test.go.
- `just check` passes (fmt + lint + build + test).
- **Learnings:**
  - See Codebase Patterns entry above re: the 5-touchpoint checklist for new passthrough MR fields.
  - No downstream TUI/domain consumers needed changes yet — these fields are new plumbing for a future feature (external command launcher template variables, per docs/adr/0004), not yet read anywhere.
---

## 2026-07-30 - mrr-4z4.2
- Added `config.Command` struct (`Name`, `Key`, `Binary`, `Args []string`, mapstructure tags `name`/`key`/`binary`/`args`) and `Commands []Command` field on `AppConfig` in internal/config/config.go.
- `validate(cfg)` now returns `([]string, error)` instead of just `error` — warnings are data returned from validation, not a side effect, so they stay unit-testable without capturing log output. `Load()` is the only place that logs them, via `slog.Default().Warn(...)`.
- `validateCommands`: duplicate `Key` across configured commands is a load-time `error` (config refuses to load); a `Binary` not found via `exec.LookPath` is appended as a warning string only — command stays in `cfg.Commands`, load still succeeds.
- Added 4 tests in config_test.go: `TestLoadCommandsValid`, `TestValidationDuplicateCommandKey`, `TestValidationCommandMissingName`, `TestValidationCommandMissingBinaryIsWarningOnly`.
- Files changed: internal/config/config.go, internal/config/config_test.go.
- `just check` passes.
- **Learnings:**
  - **Why `validate()` can't log through the app's real `*slog.Logger`**: `config.Load()` runs in `internal/cmd/mrboard/root.go` *before* `core.New()` builds the real logger (which itself reads settings out of the just-loaded config) — a genuine bootstrap ordering constraint, not a style choice. Returning warnings as `[]string` from a pure `validate`/`validateCommands` and only calling `slog.Default().Warn(...)` inside `Load()` sidesteps needing to inject a logger before one exists, while keeping the validation logic itself trivially testable (assert on returned data, not captured log output).
  - Template-placeholder syntax inside `Command.Args` (the `{{.Var}}` set) is deliberately NOT validated in this package — the ADR (docs/adr/0004-external-command-launcher.md, "Template variable set" decision) assigns the named-projection/resolution logic to a separate ticket (mrr-4z4.3); this bead only validates config-structural correctness (required fields, one duplicate-key check, one binary-presence check).
---

## 2026-07-30 - mrr-4z4.3
- Added `BuildCommandArgv(mr domain.MergeRequest, cmd config.Command) ([]string, error)` in `internal/tui/command_argv.go` — resolves each `cmd.Args` entry as a `text/template` against a private `commandTemplateData` struct (the named projection from the ADR: `ProjectPath`, `IID`, `SourceBranch`, `TargetBranch`, `WebURL`, `Title`, `Author`). Returns only the resolved args, not `cmd.Binary` — mirrors `exec.Command(name, arg...)`'s own name/args split, so mrr-4z4.5 can call `exec.Command(cmd.Binary, argv...)` directly.
- Package placement: `internal/tui`, not a new package. Per docs/architecture.md, the only packages that already import both `internal/domain` and `internal/config` are `internal/core` (composition root — wrong layer for reusable business logic) and `internal/tui` (already imports both, for `themes.go`/`model.go`). `internal/tui` also wins on locality: the ADR ties this feature's `exec` call to `tea.ExecProcess`, which only `internal/tui` may import, so the argv builder lives right next to its future caller (mrr-4z4.5).
- No file/widget-boundary lint or test enforces "widgets only" in `internal/tui` — `filter.go` already holds plain non-widget helper logic there, confirming a new plain-function file is an accepted pattern.
- Unknown template variable errors "for free": `commandTemplateData` is a struct, not a `map[string]any`, so `text/template` execution already fails on an unrecognized field name (`can't evaluate field X in type tui.commandTemplateData`) with no extra validation code or `Option("missingkey=...")` needed — that option only affects map lookups, not struct fields.
- Added `TestBuildCommandArgv` in `internal/tui/command_argv_test.go`: one subtest per of the 7 template variables, one for multiple variables across multiple args, one literal (no-template) arg, plus unknown-variable and malformed-template (unclosed `{{`) error cases — 11 subtests total.
- Files changed: internal/tui/command_argv.go, internal/tui/command_argv_test.go.
- `just check` passes.
- **Learnings:**
  - `golangci-lint`'s `goconst` lint fires repo-wide, not per-file: reusing the literal `"alice"` in a new test file tripped it because `approver_editor_test.go` already defines `editorTestApprover = "alice"` — reuse that existing constant (or add a local one, as done here for `"review"`) instead of introducing a fresh string literal that pushes an existing string over the lint's repetition threshold.
---

## 2026-07-30 - mrr-4z4.4
- Added `NewDynamicContext(name, title string, actions []*Action, opts ...ContextOpt) *Context` in `internal/tui/keymap.go`, right after the existing reflection-based `NewContext` — builds the same `byKey` map from an explicit `[]*Action` slice instead of reflecting over a keymap struct's fields, so `footerItems`/`helpSections`/cross-context shadowing all work unchanged. Deliberately does **not** append to the package-level `allContexts` registry: that registry backs `TestNoKeyConflicts`, a check over the fixed, compile-time set of contexts; a dynamic context gets rebuilt once per config load (real usage) or once per test — registering it globally would let repeated test/model construction leak duplicate entries into that shared slice.
- Added `BuildCustomCommandsContext(cmds []config.Command) *Context` in `internal/tui/keys.go` — one `Act(cmd.Key, cmd.Name, PriorityModal, CategoryAct)` per configured command, calling `NewDynamicContext`. `keys.go` needed a new `internal/config` import (config schema, not a vendor package — no vendor-bleeding rule concern).
- Only `internal/tui/keymap.go` and `keys.go` touched, per the ADR — actually pushing the returned context onto `Model`'s stack (`baseStack()`/`contextStack()` in model.go) is out of scope here; that belongs to mrr-4z4.5 ("wire ExecProcess suspend/resume"), which already owns the keypress-to-exec dispatch in model.go. Confirmed this split against the epic's other child tickets before writing any model.go code.
- Verified "custom overrides default" with a unit test that builds the stack by hand (`[]*Context{BaseCtx, BoardCtx, ctx}`) and calls the existing `helpSections` — no need to touch `Model` at all to exercise the stacking rule.
- Files changed: internal/tui/keymap.go, internal/tui/keys.go, internal/tui/keymap_test.go.
- `just check` passes; `TestBindingsDefinedOnlyInKeys` passes unmodified (it greps call *sites*, not argument literalness, so `Act(cmd.Key, cmd.Name, ...)` inside a loop in keys.go satisfies it same as any static call).
- **Learnings:**
  - Before wiring a "stack placement" requirement described in a ticket, check the epic's sibling tickets (`br show <epic>`) — the ADR's own "Stack placement" bullet reads like implementation work, but the actual push-onto-the-stack code was already assigned to a later ticket that owns `model.go`. A ticket's "Scope" header listing exact files (here: only `keymap.go`/`keys.go`) is the authoritative scope boundary, not the surrounding prose.
  - `NewContext`'s panic-on-duplicate-key is a dev-time bug check (hardcoded keymaps in keys.go); `NewDynamicContext` reuses the same panic for the same *class* of bug (an invariant that should never trigger in practice), while the ADR explicitly assigns user-facing duplicate-key rejection for configured commands to `internal/config` validation at load time (mrr-4z4.2) — two different failure modes for the same shape of collision, at two different layers, and both were already correctly separated by the time this ticket started.
---

## 2026-07-30 - mrr-4z4.5
- Added `Model.customCommandsCtx *Context`, built once in `New()` via `BuildCustomCommandsContext(cfg.Commands)` (mrr-4z4.4's constructor). `baseStack()`'s final branch now returns `append(stack, BoardCtx, m.customCommandsCtx)` — pushed only in the exact `[Base, Board]` case per the ADR, never in Detail/overlay states.
- `handleKeyBoard` checks `matchCustomCommand(msg)` *before* its static switch, so a configured key colliding with a board default (e.g. `r`) is dispatched to the exec flow instead — this is what actually makes "custom overrides default" true, since the switch itself has no notion of the context stack; the stack only governs the earlier quit/help-shadowing check and the footer/help modal.
- `matchCustomCommand` zips `m.customCommandsCtx.actions[i]` against `m.cfg.Commands[i]` by index — safe because `BuildCustomCommandsContext` builds one action per cmd in order with no filtering. Reads the unexported `Context.actions` field directly (same package `tui`, no accessor needed).
- Added `CommandResultMsg{CommandName, Err}` (mirrors `NotifyResultMsg`) and `execCommandCmd`/`handleCommandResult`, next to `notifyCmd`/`handleNotifyResult`. `execCommandCmd` routes *both* argv-template-resolution failure and the `tea.ExecProcess` callback's error through the same `CommandResultMsg`, so there's one outcome handler instead of two — a template error (e.g. a typo'd `{{.Var}}`) and a runtime exec failure (`*exec.Error`/`*exec.ExitError`) both end up as the same error toast.
- `exec.Command(cmd.Binary, argv...)` trips `gosec` G204 (variable first arg) — suppressed with `//nolint:gosec` plus a reason comment; this is inherent to the feature (config-driven binary, no shell interpretation) and unlike `openBrowser`'s `exec.Command("open", url)` elsewhere in this file, whose first arg is a literal so it never triggers G204.
- Files changed: internal/tui/model.go, internal/tui/command_exec_test.go (new).
- `just check` passes.
- **Learnings:**
  - `tea.ExecProcess(cmd, fn)` returns a `Cmd` that, when invoked directly (no live `*tea.Program`), just returns bubbletea's *unexported* `execMsg{cmd, fn}` — it does **not** actually run the process. Real execution only happens inside `Program.exec()`, driven by the bubbletea event loop. This means "success / missing binary / non-zero exit" cannot be unit-tested through the `tea.Cmd` layer at all; the table-driven test instead calls `exec.Command(...).Run()` directly to obtain authentic `*exec.Error`/`*exec.ExitError` values, then feeds those straight into `handleCommandResult` — testing the message-handling logic, not the exec plumbing (which is exactly what mrr-4z4.6's agent-tui manual gate is for).
  - To prove "custom command shadows a board default", asserting `cmd != nil` after a keypress is not enough — the built-in board action being shadowed (e.g. Refresh) *also* returns a non-nil `Cmd` when MRs are loaded. Assert on the resulting *model* mutation instead (e.g. `result.(Model).isRefreshing` stayed `false`) to prove the built-in branch never ran.
  - `goconst` counts exact string literals **per package**, across all files including tests written in earlier tickets (mrr-4z4.3's `"hunk"` in `command_argv_test.go` + mrr-4z4.4's `"hunk"` in `keymap_test.go` already sat at 2) — a 3rd reuse of the same literal in a new test file trips the lint even though each file looks fine in isolation. Grep the exact literal repo-wide before reusing one from a sibling ticket's test fixture.
---

## 2026-07-30 - mrr-4z4.6
- Added `scripts/testdata/fixture-command.sh` (executable, checked in permanently) — prints a distinctive banner + its argv, then `sleep 5` before exiting, giving agent-tui a reliable window to screenshot the child process's output before mrboard resumes.
- Added a `commands:` section to `mrboard.yaml.example` wiring the fixture script to key `x` with all three branch/path template variables, plus a commented-out illustrative `tuicr` entry showing real-world usage.
- Updated `docs/keybindings.md` with a new "Configured commands (external launcher)" section (`NewDynamicContext`, stack placement, shadowing, footer/modal policy, duplicate-key validation) and updated the File layout table; updated `docs/tui-conventions.md`'s File responsibilities table (`command_argv.go`) and Async operations section (exec-launcher is the one documented exception to the mandatory-spinner rule, since `tea.ExecProcess` suspends the terminal entirely instead of rendering into it).
- Files changed: scripts/testdata/fixture-command.sh (new), mrboard.yaml.example, docs/keybindings.md, docs/tui-conventions.md.
- `just check` passes.
- **agent-tui verification** (closing the epic's mandatory manual gate): launched via `scripts/run-tui.sh`, screenshotted the board (real GitLab data via `~/.config/mrboard/mrboard.yaml`), opened `?` help and confirmed `x fixture test` appears under **Act** in the dynamically-built "Commands" context, pressed `x` and screenshotted mid-suspend to capture the fixture's banner + correctly-resolved `{{.ProjectPath}}`/`{{.SourceBranch}}`/`{{.TargetBranch}}` args for the focused card, then waited for the board's footer text to reappear and screenshotted a clean, artifact-free redraw. Killed the session afterward.
- **Learnings:**
  - **The config actually loaded by `scripts/run-tui.sh` is `~/.config/mrboard/mrboard.yaml` (XDG), not any repo file** — `internal/cmd/mrboard/root.go`'s search order checks `--config`, then XDG, then `./mrboard.yaml`, first match wins; XDG exists on this machine so a repo-root `./mrboard.yaml` would never be picked up even though it's gitignored and would otherwise be a tempting place to stage test config. To exercise a real config-driven feature via agent-tui without permanently touching personal state, temporarily edit the XDG file, verify, then revert it to its exact original content — never leave editor-added scratch config in a file outside the repo.
  - **First attempt at screenshotting a suspended child process's output raced and missed it**: `agent-tui wait --stable` and even `agent-tui wait "<fixture text>" --assert` both returned instantly (0ms) on a *second* attempt after the process had already exited and mrboard had redrawn — `wait` appears to match against terminal scrollback/history, not just the live screen, so a phrase that was ever printed keeps matching forever and gives false confidence the state is "current." The reliable pattern: give the fixture a few seconds of `sleep` after printing, then chain the trigger keypress and the screenshot in the *same* Bash tool call (`press x && screenshot`) to minimize the gap introduced by separate tool-call round trips; only trust a screenshot taken with the child process still verifiably running.
  - `exec.Command`'s relative-path resolution (`./scripts/testdata/fixture-command.sh` in `Command.Binary`) works precisely because `run-tui.sh` `cd`s to the project root before exec'ing the mrboard binary, so the binary's (and its children's) cwd is always the repo root regardless of where `agent-tui` itself was invoked from — this is *why* the ADR's fixture-script approach requires the wrapper script and forbids pointing agent-tui at the binary directly.
---
