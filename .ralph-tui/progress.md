# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

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

