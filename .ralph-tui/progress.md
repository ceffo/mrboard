# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

- **Go module**: `github.com/mrboard/mrboard` (initialized in this session; go.mod now exists at repo root)
- **Phase classification**: Use `domain.ClassifyPhase(draft, openThreads, approvalCount, requiredApprovals, reviewers)` — evaluated in strict priority order (Draft > ReadyToMerge > NeedsAuthorAction > NeedsReview)
- **Duration formatting**: `domain.FormatDuration(d time.Duration)` covers < 1m / Xm / Xh Xm / Xd Xh

---

## 2026-05-04 - mrr-88x.6
- Implemented `internal/gitlab/mapper.go`: `DeriveReviewerStates` processes discussions chronologically; detects system notes matching `"requested review from @<username>"` for re-review requests; tracks `lastComment` and `lastReReview` timestamps per reviewer; applies four state rules (approved → Approved, never commented → NotStarted, comment > reReview → Commented, reReview > comment → ReReviewRequested)
- Also implemented `MapMR` (full MR mapper) and `countOpenThreads` helpers
- Created `internal/gitlab/mapper_test.go`: 8 tests covering all four reviewer state transitions, multi-reviewer isolation, non-reviewer note filtering, empty-reviewer edge case, and `extractReReviewUsername` parsing
- Files changed: `internal/gitlab/mapper.go`, `internal/gitlab/mapper_test.go`
- **Learnings:**
  - GitLab system note body for re-review is exactly `"requested review from @<username>"` — match as a prefix since the body contains only the username after the prefix (no trailing text in practice)
  - `gl.Note.Author` is an anonymous inline struct, not a `*BasicUser` — access fields directly (`note.Author.Username`)
  - `gl.MergeRequestApprovals.ApprovalsLeft` + `ApprovalsRequired` gives approval count; `ApprovedBy[i].User` is `*BasicUser`
  - When constructing `gl.Note` in tests, the `Author` field must use the anonymous struct literal (not `basicUser()`)
  - `Discussion.Notes[0].Resolvable && !Notes[0].Resolved` is the correct check for open threads — first note carries the resolution flag

---

## 2026-05-04 - mrr-88x.5
- Work already complete: `NonDraftSince`, `WaitingSince` fields and `FormatDuration` were implemented as part of mrr-88x.1
- All acceptance criteria verified: field presence in `domain.MergeRequest`, all `FormatDuration` edge cases (< 1m, Xm, Xh Xm, Xh, Xd Xh, Xd), full unit test coverage
- Files changed: none (verified existing `internal/domain/mr.go` and `internal/domain/mr_test.go`)
- **Learnings:**
  - Time tracking fields (NonDraftSince, WaitingSince) live in domain types from day one — the population logic (parsing GitLab system notes) belongs to the fetcher/mapper layer in a later bead
  - When beads share domain work, earlier beads may fully implement criteria for later beads — always check progress.md before implementing
---

## 2026-05-04 - mrr-88x.3
- Implemented `internal/gitlab/client.go`: `Client` struct wrapping `xanzy/go-gitlab`; `NewClient(cfg)` builds authenticated client; four methods: `ListGroupMRs`, `ListUserMRs`, `GetMRDiscussions`, `GetMRApprovals` — all paginate fully and return errors
- Created `internal/gitlab/client_test.go` covering valid construction and invalid URL error
- Added `github.com/xanzy/go-gitlab v0.115.0` (+ transitive deps) to `go.mod`
- Files changed: `internal/gitlab/client.go`, `internal/gitlab/client_test.go`, `go.mod`, `go.sum`
- **Learnings:**
  - `ListMergeRequestDiscussionsOptions` is a **type alias** (`type X Y`), not a struct embedding — fields are set directly (`opts.Page`, `opts.PerPage`), no nested `ListOptions`
  - Approvals live on `MergeRequestsService.GetMergeRequestApprovals`, not on `MergeRequestApprovalsService` (which handles project-level approval config)
  - `gl.Ptr(v)` is the helper for pointer values in v0.115.0 (older versions used `gl.String()`)
  - `xanzy/go-gitlab` is deprecated; canonical successor is `gitlab.com/gitlab-org/api/client-go` — keep in mind for future upgrades

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

## 2026-05-04 - mrr-88x.4
- Work already complete: `ClassifyPhase` and all unit tests were implemented as part of mrr-88x.1
- All acceptance criteria verified: Draft > ReadyToMerge > NeedsAuthorAction > NeedsReview priority order, no-reviewer case, mixed-reviewer edge cases
- Files changed: none (verified existing `internal/domain/mr.go` and `internal/domain/mr_test.go`)
- **Learnings:**
  - When beads share domain work, earlier beads may fully implement criteria for later beads — check progress.md before implementing
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

