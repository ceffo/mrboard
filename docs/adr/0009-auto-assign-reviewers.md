# ADR-0009: Automatic Reviewer Assignment

**Status**: Accepted

## Context

mrboard can write to GitLab as a side effect of fetching, not just in response to a keybinding —
the JIRA description back-link (ADR-0003) is the precedent. MRs authored by team members often
sit with no reviewers until someone remembers to add them, delaying the first round of feedback.
`mrsvc.SetReviewers` already exists as a generic, reviewer/approver-distinguishing write primitive
(ADR-0008), so the reviewer set can be populated without a human opening the reviewer editor.

## Non-goals

- No per-MR opt-out. Auto-assignment is a global invariant enforced on every fetch, not a
  suggestion a human can dismiss for one MR while leaving the feature enabled.
- `mrboard fetch` does not perform this write. It stays a read-only reporting path.
- No new "team" concept. Auto-assignment reuses the roster the reviewer editor already resolves
  from `sources: type: user`; it does not introduce a second definition of "the team."

## Decision

When `auto_assign_reviewers.enabled` is true, every fetch cycle (boot, periodic refresh, manual
refresh — the same call sites as the JIRA back-link and ticket-link writes) evaluates each fetched
MR against four criteria:

1. `mr.Author` is a member of `teamRoster` (`internal/tui/model.go`, resolved once at boot from
   `sources: type: user` entries — the same roster the reviewer editor already uses).
2. The MR title contains a ticket key: `issueKey := keyMatcher.ExtractFromTitle(mr.Title)`,
   `issueKey != ""`.
3. `len(mr.Reviewers) == 0`.
4. `mr.Phase != domain.PhaseDraft`.

An MR meeting all four gets every `teamRoster` member except its own author assigned as a
reviewer (not an approver) via `mrsvc.SetReviewers`. The decision logic lives in the TUI's
fetch-result handling (`internal/tui/model.go`), alongside `makeTicketLinkCmds` and
`makeTicketDescriptionLinkCmds` — `gitlabadpt` only ever executes the plain write it is told to
make; it never decides when to make it.

The check re-runs, unmemoized, on every fetch. If a human clears an eligible MR's reviewers, the
next fetch cycle reassigns the whole team — criterion 3 is a live read of GitLab state, not a
one-time trigger, so there is no session flag to remember or reset. An empty `teamRoster` (no
`sources: type: user` entries) is not a config error: the feature loads, matches nothing, and logs
a runtime warning once so the silent no-op is discoverable rather than mysterious.

`mrboard fetch` is a permanent, explicit exception to the fetch/TUI parity rule for this one
behavior: it must never mutate GitLab state as a side effect of printing JSON, even though the TUI
performs a write on the equivalent fetch.

Both success and failure toast, one per MR — unlike the silent-on-success JIRA link writes, this
action notifies an entire team on GitLab, and the person watching the board should see it happen
rather than discover it later in the log. Partial failure (one reviewer rejected, others succeed)
logs a warning and keeps whatever succeeded; there is no dedicated retry mechanism beyond the next
fetch cycle re-evaluating criterion 3, which stays true until reviewers are actually present.

## Consequences

- There is no way to permanently exempt one eligible MR from auto-assignment short of disabling
  the feature entirely, removing the ticket key from its title, or keeping it in draft. This is
  deliberate, not an oversight.
- `mrboard fetch`'s output can no longer be treated as "exactly what the TUI would do with this
  data" for this one feature — it is documented here as the one accepted gap in that guarantee.
- Toast volume scales with how many MRs newly qualify in a single fetch cycle — enabling the
  feature against an existing backlog of eligible MRs produces one toast per MR in that first
  cycle.
