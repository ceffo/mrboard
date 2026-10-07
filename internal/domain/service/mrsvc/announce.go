package mrsvc

import (
	"context"
	"errors"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
)

// releaseTimeout bounds the release of a failed announcement, which must run
// even when the context that delivered (or failed to deliver) it is done.
const releaseTimeout = 10 * time.Second

// AnnounceApproverChange claims the approver set in req and, when the caller
// owns the announcement, delivers mr through notifier. Of every instance that
// observes the same change, exactly one announces. When delivery fails the
// claim is released so the next instance to observe the change announces it
// instead of the notification being lost. announced reports a delivery.
func AnnounceApproverChange(
	ctx context.Context, claims ApproverClaims, notifier domain.Notifier, mr domain.MergeRequest, req ClaimRequest,
) (announced bool, err error) {
	owned, err := claims.Claim(ctx, req)
	if err != nil || !owned {
		return false, err
	}
	notifyErr := notifier.Notify(ctx, mr)
	if notifyErr == nil {
		return true, nil
	}
	relCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	return false, errors.Join(notifyErr, claims.Release(relCtx, req.ProjectID, req.MRIID, req.Approvers))
}

// DiscoveryPrior returns the approver set to assume before an MR's recorded
// history when mrboard first meets it without having seen the change happen.
// A young MR's approvers were set at or near its creation, so the change is
// still news and the prior is empty. An older MR's approvers are long
// established, so the prior is its current set and nothing is announced.
func DiscoveryPrior(mr domain.MergeRequest, now time.Time, youngWindow time.Duration) []string {
	if !mr.CreatedAt.IsZero() && now.Sub(mr.CreatedAt) < youngWindow {
		return nil
	}
	return mr.Approvers
}
