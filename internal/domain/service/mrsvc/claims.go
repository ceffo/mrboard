package mrsvc

import "context"

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
}

// ApproverClaims is the driven port for the approver-announcement ledger: a
// per-MR, append-only record shared by every mrboard instance, through which
// independent instances agree on which of them announces a given change.
type ApproverClaims interface {
	// Claim records req.Approvers and reports whether the caller owns the
	// announcement of that set. It reports false, without error, when the set
	// is already recorded or announced, when another instance won the same
	// race, or when the MR's live approver set no longer matches req.Approvers.
	Claim(ctx context.Context, req ClaimRequest) (owned bool, err error)

	// Release withdraws the caller's announcement of approvers after delivery
	// failed, so the next instance to observe that set announces it instead.
	Release(ctx context.Context, projectID, mrIID int64, approvers []string) error
}
