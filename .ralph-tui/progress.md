# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

*Add reusable patterns discovered during development here.*

---

## 2026-05-28 - mrr-ypr.2
- Added `ReviewerUsernames []string` to `gitlabadpt.Config` in `internal/adapters/gitlabadpt/gitlabadpt.go`
- Added `CurrentUser string` to `config.GitLabAdapterConfig` and updated `AppConfig.GitLabAdapterConfig()` accessor in `internal/config/config.go`
- Added `deriveReviewerUsernames(sources []mrsvc.Source, currentUser string) []string` to `internal/core/core.go` — collects IDs from user-type sources, appends currentUser, deduplicates
- Wired `ReviewerUsernames` into `gitlabadpt.Config` construction in `core.New`
- **Learnings:**
  - `config.GitLabAdapterConfig` is the typed bridge between full AppConfig and gitlabadpt — add fields there when adapter needs config values that aren't already exposed
  - Derivation logic (merging sources + currentUser) lives in `core.go` since that's the composition root with access to both; keeps adapters and config packages dumb
---

## 2026-05-28 - mrr-ypr.1
- Added `FetchOptions` struct to `internal/domain/service/mrsvc/mrsvc.go` with `IncludeReviewerMRs bool` field
- Changed `FetchAll(ctx context.Context)` → `FetchAll(ctx context.Context, opts FetchOptions)` in `MergeRequestSource` interface
- Updated `gitlabadpt.FetchAll` to accept opts (ignored with `_` — wiring comes in mrr-ypr.4)
- Updated callers: `internal/tui/model.go` (2 sites), `internal/cmd/mrboard/fetch.go` (added mrsvc import)
- Updated mock expectations in `internal/tui/model_test.go` to pass `mock.Anything` for the new opts arg
- Ran `just generate` to regenerate `internal/domain/service/mrsvc/mocks/mock_MergeRequestSource.go`
- **Learnings:**
  - `model_test.go` uses `src.EXPECT().FetchAll(mock.Anything)` — after adding a param, must add `mock.Anything` for each new arg
  - The adapter blanks the opts with `_` (not `opts`) because the wiring task (mrr-ypr.4) will fill it in later; this keeps the diff minimal and the intent clear
---

