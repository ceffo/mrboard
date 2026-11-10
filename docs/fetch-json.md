# `mrboard fetch` JSON — field reference

`mrboard fetch` prints a JSON array of merge requests on stdout, so scripts can consume mrboard
instead of mrboard growing features. This page lists every key of that array. The source of truth
is the `mrJSON` struct in `internal/cmd/mrboard/fetch.go`; a change to it updates this page in the
same commit (see the parity rule in [AGENTS.md](../AGENTS.md)).

This is a field reference only. It makes no stability or versioning promise.

Which MRs appear and how their values are derived follows the TUI (the parity rule); this page
covers only the shape.

## Output shape

- The top level is always an array: `[]` when there are no MRs, never `null`.
- Every object carries every key below, in this order. A key is never omitted, except `size_files`
  (see [Verbose only](#verbose-only)).
- Errors and diagnostics go to stderr, never to stdout.

## Keys

| Key | Type | Meaning |
| --- | --- | --- |
| `id` | number | GitLab's global MR id. |
| `iid` | number | The MR's number inside its project (the `!412` in the UI). |
| `project_path` | string | Full project path, e.g. `group/subgroup/repo`. |
| `title` | string | MR title. |
| `web_url` | string | URL of the MR in GitLab. |
| `head_sha` | string | SHA of the MR's head commit. |
| `updated_at` | string | GitLab's last-update time, RFC 3339 (local UTC offset). |
| `phase` | string | One of `draft`, `needs_review`, `needs_author_action`, `ready_to_merge`, `unknown`. |
| `author` | string | Author's GitLab username. |
| `reviewer_states` | array of reviewer | One entry per reviewer; `[]` when the MR has none. |
| `approvals_required` | number | Count of reviewers with `is_approver: true`; `0` when no approvers are designated. |
| `time_in_phase` | string | Display text for how long the MR has waited in its phase, e.g. `3h 12m`, `< 1m`, `2mo 4d`. |
| `time_open` | string | Same format, since the MR was created. |
| `merged_at` | string or `null` | `null` for an open MR; RFC 3339 for a merged one. Merged MRs only appear with `--merged-since`. |
| `round_trips` | number | How many times the MR bounced between reviewer and author (re-review requests). `0` when none. |
| `roles` | array of string | The user's connection to the MR; see [Keys relative to `current_user`](#keys-relative-to-current_user). `[]` when none. |
| `needs_attention` | boolean | `true` when the ball is in the user's court; see below. `false` otherwise. |
| `needs_my_review` | boolean | `true` when the user is expected to review the MR; see below. `false` otherwise. |
| `review_size` | number or `null` | Changed lines counted toward review size after the configured exclusions. `null` when the MR's diff stats are unknown. |
| `size_bucket` | string | One of `S`, `M`, `L`, `XL`, or `?` when `review_size` is `null`. |

`time_in_phase` and `time_open` are for display; they are not parseable durations. Use
`updated_at` and `merged_at` for machine-readable times.

### `reviewer_states` entries

| Key | Type | Meaning |
| --- | --- | --- |
| `username` | string | Reviewer's GitLab username. |
| `state` | string | One of `not_started`, `commented`, `re_review_requested`, `approved`, `unknown`. |
| `is_approver` | boolean | `true` when the reviewer is a designated approver of the MR, `false` otherwise. |

## Keys relative to `current_user`

`roles`, `needs_attention` and `needs_my_review` are computed for the `current_user` set in the
mrboard config ([docs/configuration.md](configuration.md)). With no `current_user`, `roles` is `[]`
and both booleans are `false` on every MR.

`roles` lists, in this fixed order, each of:

- `author` — the user authored the MR.
- `assignee` — the user is the MR's assignee.
- `reviewer` — the user is in the reviewer list, in any state, **subject to the approver gate**.

**Approver gate** ([ADR-0013](adr/0013-my-view-visibility-and-attention.md)): when the MR
designates approvers (some `reviewer_states` entry has `is_approver: true`), only an approver
counts as a reviewer. A non-approver reviewer on such an MR has no `reviewer` role, so `roles`
lacks `reviewer` even though the user appears in `reviewer_states`. On an MR with no designated
approvers, every listed reviewer counts.

`needs_attention` is `true` when:

- the user is author or assignee and `phase` is `needs_author_action` or `ready_to_merge`; or
- the user counts as a reviewer (gate above) and their `state` is `not_started` or
  `re_review_requested`.

It is always `false` for a `draft`.

`needs_my_review` is `true` when the MR is open (`merged_at` is `null`), not a `draft`, and the
user's own `reviewer_states` entry has:

- `is_approver: true`, in any `state` (`approved` included); or
- `state` `commented` or `re_review_requested`.

It reads the user's entry directly, **without the approver gate**: a non-approver reviewer who
commented on an MR that designates approvers still gets `true`, though `roles` lacks `reviewer`.
A user eligible under an approval rule but absent from `reviewer_states` gets `false`. It differs
from `needs_attention`: it counts `commented`, and counts `not_started` only for an approver.

## Filtering with `--needs-my-review`

`mrboard fetch --needs-my-review` keeps only the MRs whose `needs_my_review` is `true`, in their
output order. The output is still an array (`[]` when none match). It is applied after
`--filtered` / `--view`, so `--view all --needs-my-review` filters the board's "all" view. Without
a `current_user` it fails before fetching: exit status `1`, nothing on stdout, and an error on
stderr naming `current_user`.

## Verbose only

With `--verbose`, an MR of known size gains one more key, `size_files`. It is the only key that can
be absent, and it is outside the field set above: it is absent without `--verbose` and when
`review_size` is `null`.

| Key | Type | Meaning |
| --- | --- | --- |
| `size_files.counted` | array of file | Files counted toward `review_size`; `[]` when none. |
| `size_files.excluded` | array of file | Files left out by the size exclusions; `[]` when none. |

A file is `{"path": string, "additions": number, "deletions": number}`.

## Exit status and stderr

| Outcome | Exit status | stdout | stderr |
| --- | --- | --- | --- |
| MRs fetched, no errors | `0` | JSON array | empty |
| No MRs, no errors | `0` | `[]` | empty |
| Partial result: MRs fetched, some fetch errors | `0` | JSON array of the MRs fetched | one error line per error |
| Total failure: no MRs, at least one fetch error | `1` | empty | one error line per error, then the final failure message |
| Bad flag, config, state or snapshot load failure, or `--needs-my-review` without `current_user` | `1` | empty | the error message |

Each fetch error line starts with the fixed prefix `mrboard: fetch error: `, followed by the
error text. A partial result has no separate exit code: a caller that must detect one looks for
that prefix on stderr. `[]` with status `0` means no MRs matched (none fetched, or none left after
`--needs-my-review`); a total failure never prints `[]`.
