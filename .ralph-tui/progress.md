# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

### goconst across test files
When a string literal appears in both a source file (e.g. map key) and two test-table rows across one or more test files, `goconst` will trigger (threshold: 3 occurrences across the package). Fix: define package-level const in the source file for keys used >1 time in tests; define test-local `const` in `_test.go` for repeated literals. Using a different (semantically equivalent) string in tests avoids cross-file collisions (e.g. use "Incident" instead of "Unknown" to test the fallback path when "Unknown" already appears in other files).

*Add reusable patterns discovered during development here.*

---

## 2026-06-29 - mrr-6bb.1
- Expanded `Jira` struct in `internal/config/config.go` with 5 new fields: `Email`, `APIToken`, `BoardID`, `CacheTTL`, `IssueTypeIcons`
- Added `v.SetDefault("jira.cache_ttl", "24h")` in `Load()`
- Added `v.BindEnv("jira.api_token", "JIRA_TOKEN")` env override (mirrors GITLAB_TOKEN pattern)
- **Files changed:** `internal/config/config.go`
- **Learnings:**
  - Viper defaults for nested keys use dot notation (`jira.cache_ttl`)
  - `BindEnv` error only fires on empty key name — safe to wrap and propagate same as gitlab.token pattern
  - JIRA integration config is fully optional (no validation added); downstream adapters will check presence at runtime

---

## 2026-06-29 - mrr-6bb.2
- Created `pkg/jira/` package with two files: `config.go` and `client.go`
- `Config` struct: `InstanceURL`, `Email`, `APIToken`, `Timeout` — mirrors `pkg/gitlab/config.go` shape
- `Client` struct: Basic Auth pre-encoded in constructor (`base64(email:api_token)`), thin `net/http` wrapper
- `GetIssue(ctx, issueKey)` → `*Issue{Key, Type}` — calls `/rest/api/3/issue/{key}?fields=issuetype`
- `GetActiveSprint(ctx, boardID)` → `*Sprint{ID, Name}` (or nil if no active sprint) — calls `/rest/agile/1.0/board/{id}/sprint?state=active`
- **Files changed:** `pkg/jira/config.go`, `pkg/jira/client.go`
- **Learnings:**
  - `mnd` linter catches bare numeric literals — extract `const defaultTimeout = 30 * time.Second` before using
  - JIRA Cloud Basic Auth: `base64(email:api_token)` in `Authorization: Basic <creds>` header
  - Agile API (`/rest/agile/1.0/`) is a separate path from REST API v3 (`/rest/api/3/`); both use same auth
  - `GetActiveSprint` returns `(nil, nil)` for no active sprint — adapter layer must handle this gracefully

---

## 2026-06-29 - mrr-6bb.3
- Created `internal/domain/service/jirasvc/jirasvc.go` — `JiraEnricher` port with two methods: `GetIssueType(ctx, issueKey)` and `GetActiveSprintIssueKeys(ctx, boardID)`
- Added `jirasvc` package entry to `.mockery.yml` under `packages:`
- Ran `just generate` → mock generated at `internal/domain/service/jirasvc/mocks/mock_JiraEnricher.go`
- **Files changed:** `internal/domain/service/jirasvc/jirasvc.go`, `internal/domain/service/jirasvc/mocks/mock_JiraEnricher.go`, `.mockery.yml`
- **Learnings:**
  - Service port interface (`jirasvc`) lives in `internal/domain/service/jirasvc/` mirroring `mrsvc` — one package per port
  - Adding a new package to `.mockery.yml` requires a full `packages:` entry with `config` + `interfaces` blocks; the `dir` template uses `{{.InterfaceDir}}/mocks` which resolves relative to the interface source
  - `just generate` runs `mockery` at repo root — picks up all packages in `.mockery.yml` automatically
  - `GetActiveSprintIssueKeys` returns `(nil, nil)` for no active sprint — the port contract matches the `pkg/jira` client's `(nil, nil)` convention for missing resources

---

## 2026-06-29 - mrr-6bb.5
- Added `JiraEnricher jirasvc.JiraEnricher` field to `Core` struct in `internal/core/core.go`
- In `New()`, added step 5: if `cfg.Jira.InstanceURL`, `Email`, and `APIToken` are all non-empty, create a `pkgjira.Client` and a `jiraadpt.JiraAdapter`; assign to `Core.JiraEnricher` (nil otherwise)
- Added imports for `jiraadpt`, `jirasvc`, and `pkgjira` packages
- **Files changed:** `internal/core/core.go`
- **Learnings:**
  - Pattern for optional adapters: declare as interface type in Core (nil = disabled), guard construction with a non-empty credential check
  - `pkgjira.NewClient` takes no logger (thin HTTP client); logger is passed to `jiraadpt.New` instead
  - Composition root `New()` follows a numbered-step convention — new adapters get the next step number

---

## 2026-06-29 - mrr-6bb.4
- Added `GetSprintIssueKeys(ctx, sprintID int)` to `pkg/jira/client.go` — paginates through `/rest/agile/1.0/sprint/{id}/issue?fields=key` until all keys are fetched
- Created `internal/adapters/jiraadpt/jiraadpt.go`:
  - `jiraClient` local interface (enables test fakes without mockery)
  - `Config{CacheDir, TTL}` — CacheDir defaults to `os.UserCacheDir()/mrboard/jira`
  - `JiraAdapter` implementing `jirasvc.JiraEnricher` with write-through JSON disk cache
  - `cacheEntry{Value json.RawMessage, ExpiresAt time.Time}` — generic via `json.RawMessage`
  - `readCache` / `writeCache` helpers — cache errors are warnings, live data always returned
  - `sanitizeKey` replaces `/`, `\`, `:` with `_` for safe filenames
- Created `internal/adapters/jiraadpt/jiraadpt_test.go` with 7 tests covering live+cache, TTL disabled, no-sprint, nil-issue, sanitize, and bad-dir scenarios
- **Files changed:** `pkg/jira/client.go`, `internal/adapters/jiraadpt/jiraadpt.go`, `internal/adapters/jiraadpt/jiraadpt_test.go`
- **Learnings:**
  - `mnd` linter flags octal literals (`0o700`, `0o600`) — extract as named constants (`cacheDirPerm`, `cacheFilePerm`)
  - `goconst` linter requires 3+ occurrences of the same string literal in tests to be a constant — use named constants when repeating test fixture strings
  - Using `json.RawMessage` for the cache value field gives generic read/write without Go generics complexity
  - Local `jiraClient` interface in the adapter package avoids needing mockery for adapter unit tests — plain fake structs in `_test.go` files suffice

---

## 2026-06-29 - mrr-9yi.2
- Created `internal/tui/jira_icons.go`: `IssueTypeIconResolver` struct with `NewIssueTypeIconResolver(overrides map[string]string)` and `Resolve(issueType string) string`
- Default map: Bug=🐛 Story=📖 Task=☑️ Epic=⚡ Subtask=↩️; fallback=🎫
- Case-insensitive lookup; override keys (from `jira.issue_type_icons` config) take precedence over defaults
- Map keys extracted as package-level constants (`issueTypeBug`, `issueTypeStory`, etc.) — used both in the map and referenced in tests
- Created `internal/tui/jira_icons_test.go` with two test cases: defaults and overrides
- **Files changed:** `internal/tui/jira_icons.go`, `internal/tui/jira_icons_test.go`
- **Learnings:**
  - `goconst` counts string literals across ALL files in a package — a string in a map key + 2 test rows triggers the linter; fix by defining constants in the source file and reusing them in tests
  - Cross-file "Unknown" collision: the literal "Unknown" was already used in `column.go` and `detail.go`; adding it in a third file triggered `goconst`. Use a semantically equivalent but distinct string (e.g. "Incident") for unrecognized-type fallback tests to avoid the collision without modifying unrelated files.

---

## 2026-06-29 - mrr-9yi.1
- Added `JiraIssueType string` field to `MergeRequest` struct in `internal/domain/mr.go`
- Zero value (`""`) means "not yet fetched or no JIRA issue found" — async enrichment populates it later
- Field placed after `ReviewerSource` in its own block to signal it's populated post-fetch
- **Files changed:** `internal/domain/mr.go`
- **Learnings:**
  - Domain stays stdlib-only; new JIRA fields are plain `string` types
  - Zero value as sentinel ("not yet fetched") is the correct pattern for async-populated fields — no `*string` or separate bool needed

---

## 2026-06-29 - mrr-9yi.3
- Added `JiraIssueTypeMsg{IssueKey, IssueType, Err}` msg type to model.go
- Added `jiraEnricher jirasvc.JiraEnricher` and `iconResolver IssueTypeIconResolver` fields to `Model` struct
- Added `jiraFetchTimeout = 30 * time.Second` constant
- Updated `tui.New()` to accept `jiraEnricher jirasvc.JiraEnricher`; initializes `iconResolver` from `cfg.Jira.IssueTypeIcons`
- Added `makeJiraFetchCmd(ctx, enricher, issueKey)` — returns a `tea.Cmd` calling `GetIssueType` wrapped in `JiraIssueTypeMsg`
- Added `makeJiraEnrichCmds()` method — deduplicates issue keys across allMRs, returns `tea.Batch` of fetch cmds; no-op when `jiraEnricher == nil`
- Added `handleJiraIssueType(msg)` handler — logs errors, updates `allMRs[i].JiraIssueType` for matching key, calls `applyMRFilter()`
- FetchResultMsg handler now returns `m.makeJiraEnrichCmds()` instead of `nil`
- Updated `internal/cmd/mrboard/board.go` to pass `c.JiraEnricher` to `tui.New`
- Fixed 3 `tui.New` call sites in `model_test.go` to pass `nil` for the new `jiraEnricher` parameter
- **Files changed:** `internal/tui/model.go`, `internal/cmd/mrboard/board.go`, `internal/tui/model_test.go`
- **Learnings:**
  - Dedup by issue key before fanning out fetch commands — multiple MRs can share the same JIRA ticket
  - `iconResolver IssueTypeIconResolver` added to Model now so mrr-9yi.4 (card rendering) can use it without constructor changes
  - Pattern for adding a new parameter to `tui.New`: update signature → update model struct → update board.go call site → update all _test.go call sites
  - `just check` catches test call-site mismatches immediately; always run after signature changes

---
