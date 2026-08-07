package tui

import (
	"time"

	"github.com/ceffo/mrboard/internal/domain"
)

// dirtySet tracks MRs with a local write not yet confirmed by a landed
// fetch — the write-race guard from docs/adr/0005, "The write race that
// ungating creates". Model owns one: every local write path calls Mark,
// startFetch calls Keys to force those MRs stale on the next fetch, and a
// landing fetch calls Resolve to decide, per key, whether the local write
// survives or the landing snapshot's value wins.
type dirtySet map[domain.MRKey]time.Time

func newDirtySet() dirtySet {
	return make(dirtySet)
}

// Mark records a local write to key at t, unconfirmed until a fetch started
// at or after t lands.
func (d dirtySet) Mark(key domain.MRKey, t time.Time) {
	d[key] = t
}

// Keys returns the set's keys, for FetchOptions.ForceStale.
func (d dirtySet) Keys() []domain.MRKey {
	if len(d) == 0 {
		return nil
	}
	keys := make([]domain.MRKey, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	return keys
}

// Resolve merges a landing snapshot against the live board and clears every
// entry the landing confirms. For a dirty key whose write happened after
// fetchStartedAt, the landing entry is stale relative to that write: the
// live entry is kept in its place. Every other key — clean, or dirty but
// confirmed by this fetch — takes the landing snapshot's value, matching the
// pre-dirty-set behavior of a wholesale replace. A dirty-and-stale key can be
// absent from the landing snapshot entirely (e.g. a concurrent phase-1
// hiccup); the live entry is kept rather than silently dropped.
func (d dirtySet) Resolve(live, landing []domain.MergeRequest, fetchStartedAt time.Time) []domain.MergeRequest {
	if len(d) == 0 {
		return landing
	}

	liveByKey := make(map[domain.MRKey]domain.MergeRequest, len(live))
	for _, mr := range live {
		liveByKey[mr.Key()] = mr
	}

	seen := make(map[domain.MRKey]bool, len(landing))
	result := make([]domain.MergeRequest, 0, len(landing))
	for _, mr := range landing {
		key := mr.Key()
		seen[key] = true
		if writeAt, dirty := d[key]; dirty && fetchStartedAt.Before(writeAt) {
			if old, ok := liveByKey[key]; ok {
				result = append(result, old)
				continue
			}
		}
		result = append(result, mr)
	}
	for key, writeAt := range d {
		if seen[key] || !fetchStartedAt.Before(writeAt) {
			continue
		}
		if old, ok := liveByKey[key]; ok {
			result = append(result, old)
		}
	}
	for key, writeAt := range d {
		if !fetchStartedAt.Before(writeAt) {
			delete(d, key)
		}
	}
	return result
}
