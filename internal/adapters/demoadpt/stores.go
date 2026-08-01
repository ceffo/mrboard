package demoadpt

import (
	"context"
	"slices"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
)

// stateStore keeps app state for the session only. Load deliberately ignores the
// user's real state file: a saved filter or view mode from their own session
// would silently hide cards from a demo or a recording.
type stateStore struct{ a *Adapter }

func (s *stateStore) Load() (domain.AppState, error) {
	s.a.ds.mu.RLock()
	defer s.a.ds.mu.RUnlock()
	return s.a.ds.savedState, nil
}

func (s *stateStore) Save(st domain.AppState) error {
	s.a.ds.mu.Lock()
	defer s.a.ds.mu.Unlock()
	s.a.ds.savedState = st
	return nil
}

// snapshotStore is the in-memory stand-in for the on-disk incremental-fetch
// cache. Demo mode never constructs the real store, so the user's
// snapshot.json is never opened, written, or even have its directory created.
type snapshotStore struct{ a *Adapter }

// Load returns the full dataset pre-seeded, with an age taken from the fixture.
// That puts the board on screen and interactive from the first frame — the warm
// boot the incremental-fetch design exists to provide — rather than a spinner.
func (s *snapshotStore) Load() ([]domain.MergeRequest, time.Time, error) {
	s.a.ds.mu.RLock()
	seeded := s.a.ds.hasSavedSnapshot
	saved := slices.Clone(s.a.ds.savedSnapshot)
	savedAt := s.a.ds.snapshotSavedAt
	writtenAt := s.a.ds.snapshotWrittenAt
	s.a.ds.mu.RUnlock()

	if seeded {
		return saved, savedAt, nil
	}
	all := s.a.ds.all()
	// The pre-seeded board omits reviewer-source MRs, matching what a fetch with
	// the default settings would have persisted.
	out := make([]domain.MergeRequest, 0, len(all))
	for _, mr := range all {
		if !mr.ReviewerSource {
			out = append(out, mr)
		}
	}
	return out, writtenAt, nil
}

func (s *snapshotStore) Save(mrs []domain.MergeRequest) error {
	s.a.ds.mu.Lock()
	defer s.a.ds.mu.Unlock()
	s.a.ds.savedSnapshot = slices.Clone(mrs)
	s.a.ds.snapshotSavedAt = time.Now()
	s.a.ds.hasSavedSnapshot = true
	return nil
}

// notifier accepts every notification and discards it, which is enough to enable
// the key and show its confirmation toast.
type notifier struct{ a *Adapter }

func (n *notifier) Notify(_ context.Context, mr domain.MergeRequest) error {
	n.a.logger.Info("demo: notification suppressed", "mr_iid", mr.IID, "title", mr.Title)
	return nil
}
