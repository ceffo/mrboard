# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

- **Go module**: `github.com/mrboard/mrboard` (initialized in this session; go.mod now exists at repo root)
- **Phase classification**: Use `domain.ClassifyPhase(draft, openThreads, approvalCount, requiredApprovals, reviewers)` — evaluated in strict priority order (Draft > ReadyToMerge > NeedsAuthorAction > NeedsReview)
- **Duration formatting**: `domain.FormatDuration(d time.Duration)` covers < 1m / Xm / Xh Xm / Xd Xh

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

