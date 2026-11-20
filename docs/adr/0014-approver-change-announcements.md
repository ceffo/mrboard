# ADR-0014: Approver Changes Are Announced Once, Through a Ledger on the MR

**Status**: Accepted. Amended by #105: an announcement names who gained and who lost
required-approver status and says who changed the rule.

## Context

A Teams notification fired only in the session that made the change: the `n` key, or the approver
editor's save. An approver added in GitLab's web UI, by an author who does not run mrboard, announced
nothing. Several people run mrboard at once, often several instances each, and any of them can be the
first to see such a change, so the fix has to make N instances produce one message.

GitLab offers no event to subscribe to. Measured on GitLab EE 18.10.3:

- An MR whose approval rule was edited by hand has no system note for it, and its resource event
  endpoints return nothing.
- The project audit log records MR approval-rule changes only for the required-approvals count. It
  carries no MR id and no approver membership change, and MR-level rule creation is absent.

Detecting a change therefore means diffing observed states, and agreeing on who announces needs state
every instance shares. The only store all instances already share is GitLab itself.

## Decision

Each MR carries an append-only **ledger** of internal notes. An instance that sees an approver set it
has not handled appends a claim for that set; replaying the ledger in note-ID order decides the
announcer.

- **Entry.** A self-delimiting block, `<!-- mrboard:approvers-claim v2 released=… silent=…
  approvers=a,b -->` … `<!-- /mrboard:approvers-claim -->`, found anywhere in a note body
  (`domain.FormatApproverClaim` / `ParseApproverClaim`). System notes are ignored. `v1` entries,
  written before an approver meant a required approver, are still read, as legacy; a set found on the
  first look at an MR is not claimed against a legacy ledger (ADR-0016).
- **Election.** `domain.FoldApproverClaims` replays the entries. An entry owns an announcement
  exactly when it changes the running state and is not silent. Entries repeating the running state
  are redundant, so instances racing on one change collapse to the lowest note ID, and every instance
  computes the same winner. A→B→A announces three times, because the ledger stores the last announced
  set, not every set ever seen.
- **Protocol** (`gitlabadpt.Claim`, behind `mrsvc.ApproverClaims`). Read the ledger; return if it
  already records the set; confirm the live approver set still matches the observation (a stale
  instance would otherwise announce a revert); append the claim; wait `ClaimSettle` so concurrent
  claims land; read again; own the announcement only if this entry won.
- **Delivery** (`mrsvc.AnnounceApproverChange`). The winner posts a card through
  `domain.Notifier.NotifyApproverChange`. If that fails it appends a `released` entry, which reopens
  the announcement for the next instance, and the release only takes effect while that set is still
  the running state.
- **What is announced** (`domain.ApproverChange`). The sets are required approvers (ADR-0016), so a
  change that only affects optional rules changes no set and is never claimed. The card names who
  became a required approver (mentioned, since they are asked to act) and who stopped being one
  (named, not mentioned). Both are the difference between the claimed set and the set it replaces,
  which `Claim` returns (`mrsvc.ClaimOutcome.Previous`) from the fold (`ClaimLedgerState.Before`):
  the running set before the winning entry, or, when that set's announcement was released, the set
  before it, since a released set was never delivered. Every instance reading the ledger computes the
  same difference. When no entry precedes the winner, or the preceding one is `v1` (its set may hold
  members of optional rules), the claimer's `Prior` stands in, so the meaning change of ADR-0016 is
  never announced as someone losing the status. A claimed set that differs from the one it replaces
  by no one is recorded and not delivered.
- **Source.** The card says whether the rule was changed through mrboard or outside it (a repository
  rule, or an edit by hand in GitLab). A claim the editor makes for its own write
  (`ClaimRequest.Authoritative`) is attributed to mrboard; a change found on a refresh, to another
  source. The ledger does not record the source: the instance that wins the announcement attributes
  it from what it knows.
- **Prior set.** Each claim carries the set assumed to precede the MR's recorded history. An MR with
  no ledger whose observed set equals that prior gets no entry, so history mrboard never saw is not
  announced; otherwise the claim is the ledger's first entry and owns the announcement. An MR found
  for the first time has its current set as prior, except a young MR
  (`notifications.teams.new_mr_window`, default 24h), whose prior is empty because approvers set at
  creation are news. The editor passes the set from before its edit. Silent entries, which record a
  set without announcing it, appear only in ledgers written by v0.22.0 and v0.22.1; they are still
  folded, and no current version writes one.
- **Debounce.** A discovered change must be seen on two consecutive refreshes before it is claimed:
  GitLab's approval-rule read can return the pre-write set shortly after a write (see
  `ApplyStagedApproverFlags`). The editor's own writes skip this and the live check, because the
  written set is authoritative; the session records them as handled so the refresh that confirms them
  does not announce again.
- **Private writes** (`gitlabadpt.appendClaimID`). GitLab does not reject an internal note from a
  user below Planner: it creates a public one. Every ledger write, claim or release, first checks the
  user's effective access level on the project (inherited group membership included, cached per
  project) and refuses below Planner. A note that still comes back public is deleted. Both refusals
  are `mrsvc.ErrClaimNotPermitted`.
- **Opt-in.** `notifications.teams.announce_approver_changes` (default off). Without it nothing
  reads or writes ledgers and the editor keeps its direct notification.

The Power Automate flow is unchanged: it still receives one card per announcement.

## Why notes

Every other per-MR place was worse. A description footer is rewritten whole, so concurrent writers
overwrite each other and people's edits; an end-anchored marker is also hidden by text added after it.
Award emoji carry a name and no set. Labels pollute the project. A note is appended, never rewritten,
its ID gives all readers one total order, and an internal note is visible only to project members.
Twenty-four racing creates showed unique IDs, immediate read-after-write, and no case of a
lower ID hidden from a reader who could see a higher one.

## Consequences

- At most one announcement per real change. Failure modes are bounded: two instances may both win if
  one's entry becomes visible after the other has already read the ledger (the settle pause makes this
  rare, not impossible); a deleted or edited claim is ignored and costs at most a repeat
  announcement.
- The ledger adds one internal note per announced change, plus one per instance that lost a race for
  it and one per release. GitLab emails each of them like any comment: its new-note recipients are
  the MR's participants, project watchers and subscribers, filtered only by permission to read the
  note (Reporter and above), with the note's author skipped. Writing a note also makes its author a
  participant of the MR, so they receive email for its later comments.
- Writing a ledger needs the Planner role or above on the project. An instance refused for lack of
  it warns once and treats the change as handled, since retrying cannot succeed until its access
  changes; it never announces unilaterally. Any other write failure reports an error and retries on
  the next refresh.
- Instances that do not enable the feature neither read nor write ledgers, and do not announce.
- An editor write announced by an instance other than the writer is attributed to another source.
  That happens only when the writer did not deliver it: its delivery failed and it released the
  claim, it lacks the role to write the ledger, or it stopped before claiming. Recording the source in
  the ledger entry would close this gap, at the cost of a ledger format change.
