# Domain Model

All types live in `internal/domain/mr.go`. No external dependencies.

## ReviewerState

```go
type ReviewerState int

const (
    ReviewerNotStarted        ReviewerState = iota // assigned, no activity
    ReviewerCommented                              // left comments; ball in author's court
    ReviewerReReviewRequested                      // author re-requested; ball in reviewer's court
    ReviewerApproved                               // approved (terminal unless revoked)
)
```

### State derivation (from GitLab discussion timeline)

For each formally assigned reviewer, scan discussions chronologically and evaluate
these conditions in order — the first match wins:

| Condition | State |
|---|---|
| Reviewer has approved | `Approved` |
| Reviewer has never commented | `NotStarted` |
| Last "requested review from @X" note timestamp > reviewer's last comment | `ReReviewRequested` |
| otherwise (reviewer's last comment is the most recent activity) | `Commented` |

"Reviewer has never commented" takes priority over the re-review check: GitLab
emits the identical "requested review from @X" note both when a reviewer is
first assigned and when the author genuinely re-requests review, so a request
note alone — with no comment ever recorded — must not be read as a re-review.

System note text to match (case-insensitive): `"requested review from @<username>"`
Draft toggle note to match: `"marked as ready"`

Active reviewers = those in the MR's formal **Reviewers** field only.
Commenters not in that field are ignored for phase computation.

## MRPhase

```go
type MRPhase int

const (
    PhaseDraft           MRPhase = iota // MR is still a draft
    PhaseNeedsReview                    // ball is in reviewer(s)' court
    PhaseNeedsAuthorAction              // ball is in author's court
    PhaseReadyToMerge                   // all threads resolved + enough approvals
)
```

### Phase classification rules (evaluated in order)

1. `PhaseDraft` — if GitLab `draft: true`
2. `PhaseReadyToMerge` (labeled "Approved" in the TUI) — if there is at least one designated
   **Approver** among the reviewers and every one of them has approved
3. `PhaseNeedsAuthorAction` — if ANY active reviewer is `Commented`
4. `PhaseNeedsReview` — otherwise (all reviewers are NotStarted or ReReviewRequested, no
   reviewers, or there are no designated Approvers at all)

Rule 3 takes precedence over rule 4 in mixed states (some commented, some re-requested).
`detailed_merge_status` does not gate this classification — it only tints an Approved card green
(mergeable) or red (something still blocks it); see `DetailedMergeStatus` below.

## MRKey

```go
type MRKey struct {
    ProjectID int
    IID       int
}
```

Uniquely identifies a merge request across all sources, independent of position in any slice or
column — the single identity type for selection tracking, dedup, and snapshot-cache lookups (see
docs/adr/0005-incremental-fetch-and-selection-identity.md). `MergeRequest.Key()` returns it.

## MergeRequest

```go
type MergeRequest struct {
    ID           int
    IID          int    // per-project MR number
    ProjectID    int
    Title        string
    Author       string // GitLab username — canonical ID
    AuthorName   string // display name; falls back to Author if empty
    Assignee     string // GitLab username of the current assignee
    AssigneeName string // display name; falls back to Assignee if empty
    WebURL       string
    ProjectPath  string // namespace/project without domain, e.g. "group/repo"
    Description  string

    Phase               MRPhase
    DetailedMergeStatus string // raw value from GitLab's detailed_merge_status field — tints the
                                // Approved card green/red only, does not gate Phase (see above)
    SourceBranch        string
    TargetBranch        string
    Reviewers           []ReviewerInfo // Approvers appear first; distinguished by IsApprover

    // Approvers is the full membership of the "Approvers" approval rule — usernames eligible to
    // approve, regardless of whether they are currently assigned as a reviewer on this MR.
    // Distinct from ReviewerInfo.IsApprover, which only flags entries already present in Reviewers.
    Approvers []string

    CreatedAt         time.Time
    UpdatedAt         time.Time // GitLab's updated_at; bumped on notes, approvals, reviewer
                                 // changes, title/draft edits — the version marker the snapshot
                                 // cache diffs against (docs/adr/0005)
    NonDraftSince     time.Time // "marked as ready" note, or CreatedAt if never a draft
    WaitingSince      time.Time // when current phase started
    ReadyToMergeSince time.Time // when the phase most recently became PhaseReadyToMerge

    OpenThreads    int
    RoundTripCount int // total "requested review from @X" notes across all reviewers

    ReviewerSource bool // true when this MR came only from a reviewer-source fetch

    IssueType string // populated asynchronously; "" means not yet fetched or no linked ticket
}

// Approval counts are derived at render time from Reviewers:
//   required = len(r where r.IsApprover)
//   given    = len(r where r.IsApprover && r.State == ReviewerApproved)
// If no reviewer has IsApprover=true, no approval display is shown.
```

## ReviewerInfo

```go
type ReviewerInfo struct {
    Username     string
    Name         string
    State        ReviewerState
    WaitingSince time.Time // when ball landed in their court (or author's)
    ApprovedAt   time.Time // zero unless State == ReviewerApproved
    IsApprover   bool      // true if this reviewer is in the MR's "Approvers" approval rule
}
```

An **Approver** is a reviewer who is listed in the MR-level GitLab approval rule named `"Approvers"`.
Being an Approver is not a separate role — it is a property of a reviewer.
`IsApprover` is populated from `GET .../merge_requests/:iid/approval_rules` (rule name `"Approvers"`,
`eligible_approvers[].username`). If no such rule exists on the MR, all `IsApprover` fields are false.

## Time helpers

`FormatDuration(d time.Duration) string` lives in `internal/domain/mr.go`:
- `< 1m`
- `45m`
- `3h 20m`
- `2d 4h`
