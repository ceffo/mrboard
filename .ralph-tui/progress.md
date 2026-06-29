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

