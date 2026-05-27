# ADR-0002: mrboard becomes a read-write GitLab client

**Status**: Accepted

## Context

mrboard was a read-only board. All GitLab interactions were fetches. The approver editor requires writing the `"Approvers"` MR-level approval rule back to GitLab, and fetching project members (Developer+) for the extended picker.

## Decision

Add write methods to `pkg/gitlab/client.go`:
- `GetProjectMembers(ctx, projectID, minAccessLevel)` — for the extended approver picker
- `CreateMRApprovalRule(ctx, projectID, mrIID, payload)` — POST to approval_rules
- `UpdateMRApprovalRule(ctx, projectID, mrIID, ruleID, payload)` — PUT to approval_rules/:id

The PAT already configured for reads is assumed to have `api` scope (required for writes). No new auth mechanism.

## Consequences

- mrboard can now mutate GitLab state. This is intentional and expected.
- The GitLab client interface in `pkg/gitlab` must be extended; any mock of that interface needs regeneration.
- The approver editor is the only write surface; all other operations remain read-only.
