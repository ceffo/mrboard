package demoadpt

import (
	"context"
	"slices"

	"github.com/ceffo/mrboard/internal/domain/service/ticketsvc"
)

// tickets implements both issue-tracker ports off the fixture's lookup tables.
// One instance serves both, mirroring how the real adapter is wired so the
// enricher and the linker share state.
type tickets struct{ a *Adapter }

// GetIssueType returns the fixture's type for a key, or ("", nil) when the key
// is absent — the contract's "not found", which makes the board fall back to its
// generic ticket icon.
func (t *tickets) GetIssueType(ctx context.Context, issueKey string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	t.a.ds.mu.RLock()
	defer t.a.ds.mu.RUnlock()
	return t.a.ds.issueTypes[issueKey], nil
}

// GetActiveSprint returns the fixture's sprint regardless of board ID; the demo
// config only ever names one board. The fixture has no cache to revalidate. A
// fixture with neither a sprint block nor sprint keys has no active sprint.
func (t *tickets) GetActiveSprint(ctx context.Context, _ int) (*ticketsvc.Sprint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t.a.ds.mu.RLock()
	defer t.a.ds.mu.RUnlock()
	if !t.a.ds.hasSprint && len(t.a.ds.sprintKeys) == 0 {
		return nil, nil
	}
	return &ticketsvc.Sprint{
		Name:      t.a.ds.sprintName,
		Start:     t.a.ds.sprintStart,
		End:       t.a.ds.sprintEnd,
		IssueKeys: slices.Clone(t.a.ds.sprintKeys),
	}, nil
}

// UpsertRemoteLink is a no-op. Every ticketed MR in the fixture already carries
// the back-link marker in its description, so the board's idempotency check
// short-circuits and this should not be reached during a normal session.
func (t *tickets) UpsertRemoteLink(_ context.Context, issueKey, _, _, _ string) error {
	t.a.logger.Debug("demo: remote link suppressed", "issue_key", issueKey)
	return nil
}
