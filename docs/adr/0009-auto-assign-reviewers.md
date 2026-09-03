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
- `mrboard fetch` never mutates GitLab, under any flag or config. That contract has no exceptions;
  the write lives in a separate command instead (see below).
- No new "team" concept. Auto-assignment reuses the roster the reviewer editor already resolves
  from `sources: type: user`; it does not introduce a second definition of "the team."

## Decision

Eligibility and the write are implemented once, in the business layer, with no bubbletea or CLI
dependency: `mrsvc.AutoAssignReviewers(ctx, mrs, teamRoster, matcher, src)`. It filters `mrs` to
those meeting four criteria:

1. `mr.Author` is a member of `teamRoster` (`internal/tui/model.go`, resolved once at boot from
   `sources: type: user` entries — the same roster the reviewer editor already uses).
2. The MR title contains a ticket key: `issueKey := keyMatcher.ExtractFromTitle(mr.Title)`,
   `issueKey != ""`.
3. `len(mr.Reviewers) == 0`.
4. `mr.Phase != domain.PhaseDraft`.

For each eligible MR it calls `src.SetReviewers` with every `teamRoster` member except the MR's
own author (never as an approver), and returns one result per eligible MR — assigned usernames,
ticket key, and any write error — for the caller to log or report. `gitlabadpt` only ever executes
the plain write it is told to make; it never decides when to make it.

Both surfaces call this function after the same read-only fetch, never before or during it:

- The TUI calls it from `handleFetchResult` on every fetch cycle (boot, periodic refresh, manual
  refresh), wrapped in a `tea.Cmd`, alongside `makeTicketLinkCmds`/`makeTicketDescriptionLinkCmds`.
  Both success and failure toast, one per MR — unlike the silent-on-success JIRA link writes, this
  action notifies an entire team on GitLab, and the person watching the board should see it happen
  rather than discover it later in the log.
- The CLI exposes it as `mrboard update`, a new command distinct from `mrboard fetch`. It runs the
  identical `mrsvc.FetchAll` fetch `mrboard fetch` uses, then `mrsvc.AutoAssignReviewers` over the
  result, then logs what it did with the same log lines the TUI produces (no toast — there's no
  TUI to toast in). `mrboard fetch` itself is untouched: fetch, print JSON, no writes, for every
  caller, unconditionally.

`mrboard update` respects `auto_assign_reviewers.enabled` exactly like the TUI does — invoking it
explicitly does not bypass the config toggle. This is what parity actually means here: the CLI
reproduces what the TUI would do given the current config, rather than offering a way to make
GitLab writes the TUI wouldn't currently make.

The check re-runs, unmemoized, every time it's invoked. If a human clears an eligible MR's
reviewers, the next TUI fetch cycle (or `mrboard update` run) reassigns the whole team — criterion
3 is a live read of GitLab state, not a one-time trigger, so there is no session flag to remember
or reset. An empty `teamRoster` (no `sources: type: user` entries) is not a config error: the
feature loads, matches nothing, and logs a runtime warning once so the silent no-op is discoverable
rather than mysterious. Partial failure (one reviewer rejected, others succeed) logs a warning and
keeps whatever succeeded; there is no dedicated retry mechanism beyond the next invocation
re-evaluating criterion 3, which stays true until reviewers are actually present.

## Consequences

- There is no way to permanently exempt one eligible MR from auto-assignment short of disabling
  the feature entirely, removing the ticket key from its title, or keeping it in draft. This is
  deliberate, not an oversight.
- `mrboard update` re-runs the same `FetchAll` that `mrboard fetch` performs; run back-to-back
  they cost two GitLab round-trips instead of one. Acceptable since `update` is a manual/on-demand
  path, not the polling path the TUI uses.
- Toast volume scales with how many MRs newly qualify in a single TUI fetch cycle — enabling the
  feature against an existing backlog of eligible MRs produces one toast per MR in that first
  cycle.
