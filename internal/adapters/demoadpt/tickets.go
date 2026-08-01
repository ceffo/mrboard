package demoadpt

import (
	"context"
	"slices"
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

// GetActiveSprintIssueKeys returns the fixture's sprint membership regardless of
// board ID; the demo config only ever names one board.
func (t *tickets) GetActiveSprintIssueKeys(ctx context.Context, _ int) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t.a.ds.mu.RLock()
	defer t.a.ds.mu.RUnlock()
	return slices.Clone(t.a.ds.sprintKeys), nil
}

// UpsertRemoteLink is a no-op. Every ticketed MR in the fixture already carries
// the back-link marker in its description, so the board's idempotency check
// short-circuits and this should not be reached during a normal session.
func (t *tickets) UpsertRemoteLink(_ context.Context, issueKey, _, _, _ string) error {
	t.a.logger.Debug("demo: remote link suppressed", "issue_key", issueKey)
	return nil
}
