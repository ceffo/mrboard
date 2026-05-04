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

For each formally assigned reviewer, scan discussions chronologically:

| Condition | State |
|---|---|
| Reviewer has approved | `Approved` |
| Reviewer has never commented | `NotStarted` |
| Reviewer's last comment timestamp > last "requested review from @X" note | `Commented` |
| Last "requested review from @X" note timestamp > reviewer's last comment | `ReReviewRequested` |

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
2. `PhaseReadyToMerge` — if `open_threads == 0` AND `approval_count >= required_approvals`
3. `PhaseNeedsAuthorAction` — if ANY active reviewer is `Commented`
4. `PhaseNeedsReview` — otherwise (all reviewers are NotStarted or ReReviewRequested, or no reviewers)

Rule 3 takes precedence over rule 4 in mixed states (some commented, some re-requested).

## MergeRequest

```go
type MergeRequest struct {
    ID         int
    IID        int    // per-project MR number
    ProjectID  int
    Title      string
    Author     string
    WebURL     string

    Phase             MRPhase
    Reviewers         []ReviewerInfo

    CreatedAt         time.Time
    NonDraftSince     time.Time // "marked as ready" note, or CreatedAt if never a draft
    WaitingSince      time.Time // when current phase started

    ApprovalCount     int
    RequiredApprovals int
    OpenThreads       int
    RoundTripCount    int // total "requested review from @X" notes across all reviewers
}
```

## ReviewerInfo

```go
type ReviewerInfo struct {
    Username     string
    Name         string
    State        ReviewerState
    WaitingSince time.Time // when ball landed in their court (or author's)
}
```

## Time helpers

`FormatDuration(d time.Duration) string` lives in `internal/domain/mr.go`:
- `< 1m`
- `45m`
- `3h 20m`
- `2d 4h`
