# Design: Unified Reviewer Editor

**Status:** Approved design, not yet implemented.
**Epic:** `mrr-1cu`
**Supersedes:** the standalone approver editor (`internal/tui/approver_editor.go`, key `a`).

This document is the authoritative spec for the reviewer-editing feature. It records every
decision reached during design so implementing agents do not have to re-derive them. Read it
fully before touching code.

---

## 1. Concept

An MR has a **list of reviewers**. "Approver" is **a per-reviewer boolean flag**
(`domain.ReviewerInfo.IsApprover`), not a separate population. The invariant is
**approver ⊆ reviewer**: you cannot be an approver without being a reviewer.

This unifies what used to be two overlapping ideas. The old standalone approver editor (opened
with `a`) is **removed** and replaced by a single reviewer editor opened with `r`.

### Why these are two different GitLab concepts under the hood

- **Reviewers** = the MR's `reviewer_ids`, set via `PUT /projects/:id/merge_requests/:iid`
  (REST `UpdateMergeRequest`). Setting `reviewer_ids` **replaces the full reviewer set**
  (empty array clears all reviewers). Independent of approvals.
- **Approvers** = the `"Approvers"` approval rule's `user_ids` / `eligible_approvers`, written by
  the existing `SaveApprovers` (`Create/UpdateMRApprovalRule`). Independent of `reviewer_ids`.

The editor presents these as one list with a flag, but **saving touches both APIs** (see §6).

---

## 2. The "team"

- **Definition:** `team = flatten(cfg.Sources where Type == "user" → IDs)`. A group-only config
  has an **empty team** (the `T` action is then a no-op — see §5). **No new config field** is
  added; the team is derived from existing `sources`.
- **Resolution:** team usernames are resolved to GitLab **user IDs once, at TUI startup**, async,
  and **cached** on the model. GitLab user IDs are global (instance-wide), so this resolution is
  project-independent and only needs to happen once per session.
- **Feedback:** resolution happens in the TUI startup phase specifically so failures/invalid
  usernames are visible to the user (status line / log). Do not bury it in the composition root.

---

## 3. Domain & ports (new code)

### `internal/domain` (stdlib only)
- Add `type User struct { ID int64; Username string; Name string }`.
  (Deliberately distinct from `ProjectMember` — `User` is an instance user from a username lookup,
  not a project-scoped membership.)

### `internal/domain/service/mrsvc` — extend `MergeRequestSource`
```go
// SetReviewers replaces the MR's reviewer set with the given user IDs.
// An empty slice clears all reviewers.
SetReviewers(ctx context.Context, projectID int64, mrIID int64, userIDs []int64) error

// ResolveUsers looks up GitLab users by username (instance-wide), for resolving the
// configured team to user IDs. Unknown usernames are omitted from the result; the caller
// can diff against the input to detect/report invalid entries.
ResolveUsers(ctx context.Context, usernames []string) ([]domain.User, error)
```

### `pkg/gitlab.Client` (no charmbracelet imports)
- `SetMRReviewers(ctx, projectID, mrIID int64, userIDs []int64) error` —
  `gl.MergeRequests.UpdateMergeRequest(pid, iid, &gl.UpdateMergeRequestOptions{ReviewerIDs: &userIDs}, gl.WithContext(ctx))`.
  Library: `gitlab.com/gitlab-org/api/client-go v1.46.0` (already pinned; `reviewer_ids` confirmed
  present in `UpdateMergeRequestOptions`).
- User lookup by username for `ResolveUsers` —
  `gl.Users.ListUsers(&gl.ListUsersOptions{Username: gl.Ptr(name)}, gl.WithContext(ctx))` per
  username (returns 0 or 1). Add to `pkg/gitlab/interfaces.go` writer/reader interface as needed.

### `internal/adapters/gitlabadpt`
- Implement both new `MergeRequestSource` methods against the new client methods.

### Mocks
- Add `SetReviewers` / `ResolveUsers` to the mocked interface; run `just generate`
  (mockery reads `.mockery.yml`). Commit the regenerated
  `internal/domain/service/mrsvc/mocks/mock_MergeRequestSource.go`.

---

## 4. UI: `internal/tui/reviewer_editor.go`

Rename/rewrite `approver_editor.go` into the reviewer editor. Follow the existing widget pattern
(self-contained struct with `Init`/`Update`/`View`; styles from `styles.go`; keys from `keys.go`).

### Opening
- Bound to **`r`** on a focused card. **Remove the `a` binding** and `KeyMap.Approvers`.
- Rename `overlayKindApproverEditor → overlayKindReviewerEditor` in `overlay_router.go` and update
  the `model.go` wiring (field, `New` init, `handleKey` routing, `renderContent`, `applyTheme`).

### Layout
Rows are the **current reviewers** (a local, staged working copy — see §6). Each row:
`username (Name)` + an approver indicator + the existing reviewer-state annotation
(`reviewerStateLabel`).

```
 Edit Reviewers — !558 Some MR title

   alice (Alice A.)   ★ approver   (approved)
 > bob                
   carol (ext)        

 ↑/↓ move  space:approver  d:remove  /:search  T:team  ↵:save  esc:cancel
```

### Keys (define `ReviewerEditorKeyMap` + `DefaultReviewerEditorKeyMap` in `keys.go`)
| Key | Action |
|-----|--------|
| `↑`/`↓` | move cursor |
| `space` | toggle the **approver flag** on the focused reviewer |
| `d` / `del` | **remove** the focused reviewer from the list |
| `/` | open **search** sub-mode (§5) |
| `T` | **set whole team** (§5) |
| `Enter` | **commit** staged edits (§6) |
| `Esc` / `r` | **discard** staged edits and close |

### Author
The MR author is **hidden everywhere** — never shown in the list, filtered from search results,
and skipped by `T`. (GitLab won't have the author as a reviewer anyway.)

---

## 5. Sub-actions

### Search-to-add (`/`) — multi-select
- Query input filters **project members** (`GetProjectMembers`, Developer+ — already implemented;
  returns `domain.ProjectMember` with `UserID`).
- `space` toggles selection on matches; `Enter` adds **all selected** to the reviewer list and
  returns to the main view; `Esc` cancels (adds nothing).
- Results **exclude** the author and anyone already in the staged reviewer list.

### Set whole team (`T`) — additive
- Appends every team roster member (from the cached startup resolution, §2) **not already** in the
  staged list, minus the author. **Idempotent.** Added members are reviewers, **not** approvers.
- Existing reviewers (including ad-hoc / non-team ones) are **kept**.
- If the roster is **not yet resolved**, show a transient `resolving team…` hint and no-op.
- Empty team (group-only config) → no-op (optionally a `no team configured` hint).

---

## 6. Commit model (staging buffer)

The editor is a **staging buffer**. All edits (add via search, remove, toggle approver flag, set
team) mutate a **local working set only** and change nothing on GitLab until committed.

- **`Enter` commits:**
  1. **Always** write `reviewer_ids` = the full staged reviewer list (replace semantics) via
     `SetReviewers`. An empty list clears all reviewers — **no extra confirm** (Enter is the
     deliberate apply gate).
  2. Write the **Approvers approval rule only if the approver flag set changed** vs. the state the
     editor opened with (`SaveApprovers` with the flagged subset's user IDs). Skip the call when
     unchanged, so the rule's `approvals_required` is not disturbed by reviewer-only edits.
  3. Then `FetchMR` to refresh and update the board (mirror the existing
     `ApproversSavedMsg` → re-fetch flow; rename messages to reviewer equivalents).
- **`Esc` discards** all staged changes.

### User-ID resolution at save time
`domain.ReviewerInfo` has **no `UserID`** (only `Username`/`Name`). To build `reviewer_ids` for the
reviewers being kept, resolve their usernames → IDs via `GetProjectMembers` (current reviewers are
project members), exactly as `approver_editor.go`'s `saveCmd` does today (lazy fetch + `userIDByName`
map; fetch on demand if a username is unresolved). Team members added via `T` already carry IDs from
the startup roster; search-added members carry IDs from `GetProjectMembers`.

### Invariants enforced locally
- Toggling approver on is only meaningful for a row that is a reviewer — and every row **is** a
  reviewer, so `space` simply flips `IsApprover`.
- Removing a reviewer drops them from the list entirely, which also drops any approver flag (they
  leave both `reviewer_ids` and the rule on the next commit).

---

## 7. Task breakdown (children of `mrr-1cu`)

1. **Domain + ports + client + mocks** — `domain.User`; `SetReviewers`/`ResolveUsers` on
   `mrsvc.MergeRequestSource`; `pkg/gitlab` client methods (`SetMRReviewers`, user lookup);
   `gitlabadpt` implementations; regenerate mocks. (§3)
2. **TUI startup team resolution** — `Init` fires a resolve command using `ResolveUsers` over the
   `type:user` source IDs; `TeamResolvedMsg{roster, err}` cached on the model; surface feedback;
   handle invalid usernames. (§2)
3. **Reviewer editor widget** — rewrite `approver_editor.go` → `reviewer_editor.go`; staged list;
   keys (`space`/`d`/`/`/`T`/`Enter`/`Esc`); search multi-select sub-mode; `T` additive; author
   hidden; commit model (reviewer_ids always + rule-if-changed + re-fetch). `keys.go`
   `ReviewerEditorKeyMap`, `overlay_router.go` `overlayKindReviewerEditor`, `model.go` wiring,
   remove `a`/`KeyMap.Approvers`. (§4–6)
4. **Verify + docs** — `just check`; mandatory `agent-tui` walkthrough (open with `r`, toggle
   approver, remove, search-add multi, `T`, save, esc); brief doc/README note (no shortcut table —
   the keybinding bar is self-documenting).

---

## 8. Key file references

| Concern | File |
|---|---|
| Template to rewrite | `internal/tui/approver_editor.go` |
| Save→re-fetch flow to mirror | `approver_editor.go` `saveCmd`, `ApproversSavedMsg` |
| Existing approver rule write | `internal/adapters/gitlabadpt/gitlabadpt.go` `SaveApprovers` |
| Member fetch (search + save resolution) | `mrsvc.GetProjectMembers`, `gitlabadpt` impl |
| Reviewers built from requested reviewers only | `internal/adapters/gitlabadpt/mapper.go` (`MapMR`, `applyApproverFlag`) |
| Keys | `internal/tui/keys.go` (`Approvers` binding → remove; add `ReviewerEditorKeyMap`) |
| Overlay routing | `internal/tui/overlay_router.go` (`overlayKindApproverEditor`) |
| Model wiring | `internal/tui/model.go` |
| GitLab client | `pkg/gitlab/client.go`, `pkg/gitlab/interfaces.go` |
| Config / sources | `internal/config/config.go` (`Source{Type,IDs}`) |

> Note on the mapper: `MapMR` builds `Reviewers` purely from GitLab's **requested reviewers**, then
> flags `IsApprover` on top (`applyApproverFlag`). An approver who is **not** a requested reviewer
> therefore never appears in `mr.Reviewers`, and committing the editor will drop them from the
> Approvers rule. Under the approver ⊆ reviewer model this is intended cleanup, not a bug.
