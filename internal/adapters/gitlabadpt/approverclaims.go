package gitlabadpt

import (
	"context"
	"errors"
	"fmt"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	ilog "github.com/ceffo/mrboard/internal/log"
)

var _ mrsvc.ApproverClaims = (*GitLabAdapter)(nil)

// DefaultClaimSettle is how long Claim waits after appending its entry before
// re-reading the ledger, so entries other instances appended at the same time
// are visible when the winner is decided.
const DefaultClaimSettle = 1500 * time.Millisecond

// Claim implements mrsvc.ApproverClaims. The ledger is the MR's internal notes
// carrying a domain.ApproverClaim block; note IDs, assigned by GitLab, give
// every instance the same order to replay.
func (a *GitLabAdapter) Claim(ctx context.Context, req mrsvc.ClaimRequest) (bool, error) {
	logger := ilog.FromContext(ctx)
	want := domain.ApproverSetHash(req.Approvers)

	state, err := a.readClaimLedger(ctx, req.ProjectID, req.MRIID)
	if err != nil {
		return false, err
	}
	// Every ledger entry is a note that emails the MR's participants, so an
	// empty ledger stays empty until there is a change to announce. The first
	// entry on an empty ledger owns its announcement, so none is needed to
	// baseline the prior set.
	if state.Empty && domain.ApproverSetHash(req.Prior) == want {
		return false, nil
	}
	if !state.Empty && !state.Released && domain.ApproverSetHash(state.Approvers) == want {
		return false, nil
	}

	// An observation can be stale by the time it is claimed; claiming it would
	// announce a revert of a change another instance already announced.
	if !req.Authoritative {
		live, liveErr := a.liveApprovers(ctx, req.ProjectID, req.MRIID)
		if liveErr != nil {
			return false, liveErr
		}
		if domain.ApproverSetHash(live) != want {
			logger.Debug("gitlab: approver claim skipped, observation stale",
				"project_id", req.ProjectID, "mr_iid", req.MRIID)
			return false, nil
		}
	}

	id, err := a.appendClaimID(ctx, req.ProjectID, req.MRIID, domain.ApproverClaim{Approvers: req.Approvers})
	if err != nil {
		return false, err
	}
	if a.cfg.ClaimSettle > 0 {
		select {
		case <-time.After(a.cfg.ClaimSettle):
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	settled, err := a.readClaimLedger(ctx, req.ProjectID, req.MRIID)
	if err != nil {
		return false, err
	}
	owned := settled.Announcers[id]
	logger.Info("gitlab: approver claim decided", "project_id", req.ProjectID, "mr_iid", req.MRIID,
		"claim_id", id, "owned", owned)
	return owned, nil
}

// Release implements mrsvc.ApproverClaims.
func (a *GitLabAdapter) Release(ctx context.Context, projectID, mrIID int64, approvers []string) error {
	return a.appendClaim(ctx, projectID, mrIID, domain.ApproverClaim{Approvers: approvers, Released: true})
}

func (a *GitLabAdapter) readClaimLedger(ctx context.Context, projectID, mrIID int64) (domain.ClaimLedgerState, error) {
	notes, err := a.client.ListMRNotes(ctx, projectID, mrIID)
	if err != nil {
		return domain.ClaimLedgerState{}, err
	}
	return domain.FoldApproverClaims(claimsFromNotes(notes)), nil
}

// claimsFromNotes extracts the ledger entries from an MR's notes. System notes
// are skipped: only a person's or mrboard's own note can carry a claim.
func claimsFromNotes(notes []*gl.Note) []domain.ApproverClaim {
	var claims []domain.ApproverClaim
	for _, n := range notes {
		if n.System {
			continue
		}
		if c, ok := domain.ParseApproverClaim(n.Body); ok {
			c.ID = n.ID
			claims = append(claims, c)
		}
	}
	return claims
}

func (a *GitLabAdapter) appendClaim(ctx context.Context, projectID, mrIID int64, c domain.ApproverClaim) error {
	_, err := a.appendClaimID(ctx, projectID, mrIID, c)
	return err
}

// appendClaimID writes c as an internal note. GitLab does not reject an
// internal note from a user below Planner: it silently creates a public one.
// The access check keeps such a user from writing at all, and a note that
// still comes back public is deleted and refused.
func (a *GitLabAdapter) appendClaimID(
	ctx context.Context, projectID, mrIID int64, c domain.ApproverClaim,
) (int64, error) {
	level, err := a.client.CurrentUserAccessLevel(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("append approver claim: %w", err)
	}
	if level < int(gl.PlannerPermissions) {
		return 0, fmt.Errorf("append approver claim: access level %d on project %d: %w",
			level, projectID, mrsvc.ErrClaimNotPermitted)
	}
	note, err := a.client.CreateMRNote(ctx, projectID, mrIID, domain.FormatApproverClaim(c), true)
	if err != nil {
		return 0, fmt.Errorf("append approver claim: %w", err)
	}
	if !note.Internal {
		refused := fmt.Errorf("append approver claim: note %d was created public: %w",
			note.ID, mrsvc.ErrClaimNotPermitted)
		return 0, errors.Join(refused, a.client.DeleteMRNote(ctx, projectID, mrIID, note.ID))
	}
	return note.ID, nil
}

func (a *GitLabAdapter) liveApprovers(ctx context.Context, projectID, mrIID int64) ([]string, error) {
	rules, err := a.client.GetMRApprovalRules(ctx, projectID, mrIID)
	if err != nil {
		return nil, err
	}
	return sortedUsernames(approverSetFromRESTRules(rules)), nil
}
