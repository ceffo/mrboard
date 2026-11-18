# ADR-0002: mrboard becomes a read-write GitLab client

**Status**: Accepted

## Context

mrboard was a read-only board. All GitLab interactions were fetches. The approver editor requires writing the MR-level approval rule (named `"Approvers"` at the time of this ADR, `approvers_mrboard` since the read-all-rules change) back to GitLab, and fetching project members (Developer+) for the extended picker.

## Decision

Add write methods to `pkg/gitlab/client.go`:
- `GetProjectMembers(ctx, projectID, minAccessLevel)` — for the extended approver picker
- `CreateMRApprovalRule(ctx, projectID, mrIID, payload)` — POST to approval_rules
- `UpdateMRApprovalRule(ctx, projectID, mrIID, ruleID, payload)` — PUT to approval_rules/:id

The PAT already configured for reads is assumed to have `api` scope (required for writes). No new auth mechanism.

### mrboard writes only its own approval rule

`SaveApprovers` creates or updates the `approvers_mrboard` rule and nothing else: it never updates
or deletes another approval rule. Rules that come from the repository or project configuration and
rules people add by hand are not mrboard's to change, so mrboard has no rule-deleting call at all.

Consequences for the approver editor:
- The managed rule's members are the user's own picks. Its `approvals_required` is its member count.
- A reviewer another rule makes an approver is shown ticked and locked, labelled with that rule's
  name (`🔒 <rule>`), and cannot be unticked. They are never copied into the managed rule; they are
  approvers already. Approvers other rules add who are not reviewers are summarized on one
  `🔒 also approvers:` line.
- Applying an edit to sibling MRs propagates only the user's picks: a locked approver on the edited
  MR joins a sibling as a plain reviewer, never as a managed-rule member.
- `domain.MergeRequest.ApproverRules` carries each rule's name and approvers, with the managed one
  flagged, so the editor and `mrsvc.ApplyReviewerChanges` can tell the two apart.

## Consequences

- mrboard can now mutate GitLab state. This is intentional and expected.
- The GitLab client interface in `pkg/gitlab` must be extended; any mock of that interface needs regeneration.
- The approver editor is the only write surface; all other operations remain read-only.
- Every other approval rule on an MR is read-only for mrboard, whatever the editor does.
