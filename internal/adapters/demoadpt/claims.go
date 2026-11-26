package demoadpt

import (
	"context"
	"sync"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// approverLedger keeps each MR's approver-announcement ledger in memory. The
// demo is a single instance, so it has no rival to wait for and no live set
// to re-check: an entry it appends is decided at once.
type approverLedger struct {
	mu      sync.Mutex
	entries map[domain.MRKey][]domain.ApproverClaim
	nextID  int64
}

var _ mrsvc.ApproverClaims = (*approverLedger)(nil)

// ApproverClaims returns the in-memory approver-announcement ledger.
func (a *Adapter) ApproverClaims() mrsvc.ApproverClaims {
	return &approverLedger{entries: make(map[domain.MRKey][]domain.ApproverClaim)}
}

func ledgerKey(projectID, mrIID int64) domain.MRKey {
	return domain.MRKey{ProjectID: int(projectID), IID: int(mrIID)}
}

func (l *approverLedger) Claim(_ context.Context, req mrsvc.ClaimRequest) (mrsvc.ClaimOutcome, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := ledgerKey(req.ProjectID, req.MRIID)
	want := domain.ApproverSetHash(req.Approvers)
	state := domain.FoldApproverClaims(l.entries[key])
	if state.Empty && domain.ApproverSetHash(req.Prior) == want {
		return mrsvc.ClaimOutcome{}, nil
	}
	if !state.Empty && !state.Released && domain.ApproverSetHash(state.Approvers) == want {
		return mrsvc.ClaimOutcome{}, nil
	}
	id := l.append(key, domain.ApproverClaim{Approvers: domain.NormalizeApprovers(req.Approvers), Source: req.Source})
	settled := domain.FoldApproverClaims(l.entries[key])
	if !settled.Announcers[id] {
		return mrsvc.ClaimOutcome{}, nil
	}
	return mrsvc.ClaimOutcome{Owned: true, Previous: settled.Replaced(id, req.Prior), Source: settled.Source(id)}, nil
}

func (l *approverLedger) Release(_ context.Context, projectID, mrIID int64, approvers []string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.append(ledgerKey(projectID, mrIID),
		domain.ApproverClaim{Approvers: domain.NormalizeApprovers(approvers), Released: true})
	return nil
}

func (l *approverLedger) append(key domain.MRKey, c domain.ApproverClaim) int64 {
	l.nextID++
	c.ID = l.nextID
	l.entries[key] = append(l.entries[key], c)
	return c.ID
}
