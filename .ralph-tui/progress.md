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
