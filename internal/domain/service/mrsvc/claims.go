package mrsvc

import (
	"context"
	"errors"
)

// ErrClaimNotPermitted reports that the caller may not write the ledger
// privately. Retrying cannot succeed until the caller's access changes.
var ErrClaimNotPermitted = errors.New("not permitted to write the approver ledger")

// ClaimRequest describes one observation of an MR's approver set.
type ClaimRequest struct {
	ProjectID int64
	MRIID     int64
	// Approvers is the set the caller observed.
	Approvers []string
	// Prior is the set assumed to precede the MR's recorded history, used only
	// when the ledger has no entries yet. A caller that saw the change happen
	// passes the set from before it; one that merely found the MR passes
	// Approvers when the MR is old (nothing to announce) or nil when it is new
	// (its first set is itself the change).
	Prior []string
	// Authoritative marks Approvers as a set the caller just wrote itself, so
	// the MR's live approver set, which can lag a write, is not consulted to
	// detect a stale observation. Such a change is announced as made by
	// mrboard; any other as made elsewhere.
	Authoritative bool
	// Discovered marks Approvers as a set the caller found on its first look at
	// the MR, without seeing it change. Against a ledger last written under the
	// old approver meaning, such a set is not claimed: it may differ from the
	// ledger only because the meaning changed, and nothing is announced or
	// recorded for that.
	Discovered bool
}

// ClaimOutcome is the result of a claim.
type ClaimOutcome struct {
	// Owned reports that the caller owns the announcement.
	Owned bool
	// Previous is the set the announced change replaces, when Owned: the set
	// the ledger last announced before it, or the request's Prior when the
	// ledger holds no set comparable under the current approver meaning.
	Previous []string
}

// ApproverClaims is the driven port for the approver-announcement ledger: a
// per-MR, append-only record shared by every mrboard instance, through which
// independent instances agree on which of them announces a given change.
type ApproverClaims interface {
	// Claim records req.Approvers and reports whether the caller owns the
	// announcement of that set, and which set it replaces. It reports not
	// owned, without error, when the set is already recorded or announced,
	// when another instance won the same race, or when the MR's live approver
	// set no longer matches req.Approvers (unless req.Authoritative).
	Claim(ctx context.Context, req ClaimRequest) (ClaimOutcome, error)

	// Release withdraws the caller's announcement of approvers after delivery
	// failed, so the next instance to observe that set announces it instead.
	Release(ctx context.Context, projectID, mrIID int64, approvers []string) error
}
