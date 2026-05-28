# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

### dupl-safe parallel fetchers in gitlabadpt
When adding a second fetch method that mirrors an existing one (e.g. reviewer vs user), extract `fetchSourceViaGQL` and `fetchSourceViaREST` shared helpers parameterized by a `label string` and the fetch function. Thin wrapper methods call these. This satisfies the `dupl` linter without coupling the two code paths.

---

## 2026-05-28 - mrr-ypr.4
- Renamed `_ mrsvc.FetchOptions` to `opts` in `gitlabadpt.FetchAll`
- Added Phase 1b in `FetchAll`: when `opts.IncludeReviewerMRs && len(a.cfg.ReviewerUsernames) > 0`, calls `listReviewerMRs` and merges raw+mapped results before the dedup phase
- Added `listReviewerMRs` — parallel fetch over `ReviewerUsernames`, same structure as `listAllMRs`
- Added thin wrappers `fetchReviewerSourceGraphQL` / `fetchReviewerSourceREST`
- Extracted shared `fetchSourceViaGQL` and `fetchSourceViaREST` helpers (parameterized by `label string` + fetch func) to avoid `dupl` linter firing on the near-identical user/reviewer method pairs
- Refactored `fetchUserSourceGraphQL` / `fetchUserSourceREST` to delegate to shared helpers
- **Learnings:**
  - `dupl` fires immediately when adding a second method structurally identical to an existing one — always extract a shared helper when the second method is added, not after the linter catches it
  - `fetchSourceViaGQL` takes `gqlFetch func(context.Context, string) ([]pkggitlab.GQLMergeRequest, error)` and `restFallback func(context.Context, string, *slog.Logger, time.Time) sourceResult` — method values (`a.client.FetchReviewerMRsGraphQL`) satisfy these signatures directly
---

## 2026-05-28 - mrr-ypr.3
- Added `gqlReviewerMRsQuery` const + `gqlReviewerMRsResponse` type to `pkg/gitlab/graphql.go`
- Added `FetchReviewerMRsGraphQL(ctx, username)` method that calls `reviewRequestedMergeRequests` GQL field
- Added `ListReviewerMRs(ctx, username)` REST fallback to `pkg/gitlab/client.go` using `ReviewerUsername` filter
- Refactored `ListUserMRs` to use shared `listMRsPaged` private helper (required by dupl linter — both methods are near-identical paged loops)
- **Learnings:**
  - `dupl` linter fires when two functions are structurally identical even with different log strings. Extract a private paged-list helper when adding a second method that calls `ListMergeRequests` with different opts.
  - `lll` linter enforces 120-char line limit — slog calls with many args need variables to hold computed values first
  - GitLab GQL `User.reviewRequestedMergeRequests` mirrors `authoredMergeRequests` — same fields, same pagination args
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

