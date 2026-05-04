# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

- **Go module**: `github.com/mrboard/mrboard` (initialized in this session; go.mod now exists at repo root)
- **Phase classification**: Use `domain.ClassifyPhase(draft, openThreads, approvalCount, requiredApprovals, reviewers)` — evaluated in strict priority order (Draft > ReadyToMerge > NeedsAuthorAction > NeedsReview)
- **Duration formatting**: `domain.FormatDuration(d time.Duration)` covers < 1m / Xm / Xh Xm / Xd Xh

---

## 2026-05-04 - mrr-88x.2
- Implemented `internal/config/config.go`: `Config`, `GitLab`, `Source` structs; `Load()` reads `./mrboard.toml` or `$MRBOARD_CONFIG`; `$GITLAB_TOKEN` overrides file token; `RequiredApprovals` defaults to 2; `validate()` returns clear errors for missing URL/token/sources
- Created `internal/config/config_test.go` covering: valid load, explicit approvals, env token override, missing URL/token/sources, default path fallback via `os.Chdir`
- Created `mrboard.toml.example` at repo root
- Added `github.com/BurntSushi/toml v1.6.0` to `go.mod`
- Files changed: `internal/config/config.go`, `internal/config/config_test.go`, `mrboard.toml.example`, `go.mod`, `go.sum`
- **Learnings:**
  - `t.Setenv` is the clean way to set env vars in tests (auto-restores on cleanup); `os.Unsetenv` needed for clearing vars not set by the test
  - Default path test requires `os.Chdir` into the temp dir — wrap in `t.Cleanup` to restore cwd
  - `go test ./...` runs from module root; `os.Chdir` inside a test affects the whole test binary so do it in its own subtest or isolate carefully

---

## 2026-05-04 - mrr-88x.1
- Initialized Go module (`go mod init github.com/mrboard/mrboard`)
- Created `internal/domain/mr.go` with: `ReviewerState`, `MRPhase`, `ReviewerInfo`, `MergeRequest`, `ClassifyPhase`, `FormatDuration`
- Created `internal/domain/mr_test.go` with unit tests for all phase classification rules and FormatDuration variants
- Files changed: `go.mod`, `internal/domain/mr.go`, `internal/domain/mr_test.go`
- **Learnings:**
  - No go.mod existed — had to `go mod init` before any Go tooling works
  - `ClassifyPhase` is a pure function (not a method on MergeRequest) so it can be unit-tested without constructing a full struct
  - Phase rule 3 (NeedsAuthorAction) takes precedence over rule 4 even when mixed with ReReviewRequested reviewers
---

