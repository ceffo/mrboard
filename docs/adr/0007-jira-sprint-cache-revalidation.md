# ADR-0007: JIRA Sprint-Cache Revalidation Owned by the Adapter

**Status**: Accepted

## Context

`ticketsvc.TicketEnricher.GetActiveSprintIssueKeys` used to take a `forceRefresh bool`: the
caller (`internal/tui/model.go`) decided whether to bypass `jiraadpt`'s 24h disk cache and fetch
live. `Init` and the manual-refresh keybinding both passed `forceRefresh=true`; the 60s
auto-refresh tick (`handleRefreshTick`) never called `GetActiveSprintIssueKeys` at all. Sprint
membership could go stale for up to 24h — the full disk-cache TTL — between a manual refresh or a
restart, silently, because nothing forced the missing call site to exist.

The bug was a direct consequence of the design: correctness depended on every trigger site
independently remembering to pass the right bool.

## Decision

Drop `forceRefresh` from the port entirely. `GetActiveSprintIssueKeys(ctx, boardID)` now always
"asks for current data"; `jiraadpt.JiraAdapter` decides internally whether that ask reaches JIRA
or is served from cache, based on a new `SprintCacheTTL` config field
(`jira.sprint_cache_ttl`, default 5m) — separate from the existing `TTL`
(`jira.cache_ttl`, default 24h), which still governs the issue-type and remote-link caches. Those
change rarely enough that a caller-driven live-vs-cached decision was never the problem; sprint
membership needed a much shorter, adapter-owned window.

With the flag gone, every trigger that should notice a sprint rollover simply calls
`Model.sprintFetchCmd()` unconditionally: `Init`, the manual-refresh keybinding, and now also
`handleRefreshTick` on every tick. The adapter's own `SprintCacheTTL` — not the caller — decides
how often that actually turns into a JIRA call.

## Consequences

- Sprint-membership staleness is now bounded by `sprint_cache_ttl` (5m default) instead of
  `cache_ttl` (24h) or "however long since the last manual refresh."
- `handleRefreshTick` fires `sprintFetchCmd()` on every 60s tick regardless of whether an MR fetch
  is already in flight — cheap by design, since a cache hit is the common case.
- Adding a future caller that needs sprint data (e.g. a new keybinding) requires no forceRefresh
  reasoning: call the port, the adapter's policy applies uniformly.
