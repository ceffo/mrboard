package demoadpt

import (
	"slices"
	"sync"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
)

// dataset is the single mutable source of truth behind every demo port. Reads
// and writes both go through it, so a reviewer change made in the UI is visible
// to the next fetch — a fake that served immutable fixture data would let the
// card visibly revert on the next refresh tick.
//
// Bubble Tea runs each Cmd on its own goroutine, so fetches race writes: the
// mutex is required, and every read must hand back a deep copy. In particular
// the reviewer slice must be cloned, because the reviewer-write path mutates
// mr.Reviewers[i] in place on whatever slice it was given.
type dataset struct {
	mu sync.RWMutex

	mrs     []domain.MergeRequest
	drafts  map[domain.MRKey]bool
	threads map[domain.MRKey][]domain.Thread
	diffs   map[domain.MRKey]domain.MRDiff

	people       map[string]fixturePerson
	peopleByID   map[int64]fixturePerson
	projectPaths map[int]string
	members      map[int][]domain.ProjectMember
	issueTypes   map[string]string
	sprintKeys   []string

	snapshotWrittenAt time.Time

	// savedState and savedSnapshot back the two store ports, in memory only.
	savedState       domain.AppState
	savedSnapshot    []domain.MergeRequest
	snapshotSavedAt  time.Time
	hasSavedSnapshot bool
}

// cloneMR returns a copy safe to hand across the port boundary.
func cloneMR(mr domain.MergeRequest) domain.MergeRequest {
	mr.Reviewers = slices.Clone(mr.Reviewers)
	return mr
}

// all returns every MR, deep-copied.
func (d *dataset) all() []domain.MergeRequest {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]domain.MergeRequest, 0, len(d.mrs))
	for _, mr := range d.mrs {
		out = append(out, cloneMR(mr))
	}
	return out
}

// find returns the MR with the given key, deep-copied.
func (d *dataset) find(key domain.MRKey) (domain.MergeRequest, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, mr := range d.mrs {
		if mr.Key() == key {
			return cloneMR(mr), true
		}
	}
	return domain.MergeRequest{}, false
}

// mutate applies fn to the stored MR, then re-derives its phase and dependent
// timestamps so a write can move the card to another column.
func (d *dataset) mutate(key domain.MRKey, fn func(*domain.MergeRequest)) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.mrs {
		if d.mrs[i].Key() != key {
			continue
		}
		fn(&d.mrs[i])
		d.mrs[i].UpdatedAt = time.Now()
		applyDerivedFields(&d.mrs[i], d.drafts[key])
		return true
	}
	return false
}

func (d *dataset) threadsFor(key domain.MRKey) []domain.Thread {
	d.mu.RLock()
	defer d.mu.RUnlock()
	src := d.threads[key]
	out := make([]domain.Thread, 0, len(src))
	for _, t := range src {
		t.Notes = slices.Clone(t.Notes)
		out = append(out, t)
	}
	return out
}

func (d *dataset) diffFor(key domain.MRKey) (domain.MRDiff, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	diff, ok := d.diffs[key]
	if !ok {
		return domain.MRDiff{}, false
	}
	diff.Files = slices.Clone(diff.Files)
	return diff, true
}

func (d *dataset) membersOf(projectID int) []domain.ProjectMember {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return slices.Clone(d.members[projectID])
}

// resolve maps usernames to users, silently dropping unknown ones exactly as the
// real port contract specifies.
func (d *dataset) resolve(usernames []string) []domain.User {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var out []domain.User
	for _, u := range usernames {
		if p, ok := d.people[u]; ok {
			out = append(out, domain.User{ID: p.UserID, Username: p.Username, Name: p.Name})
		}
	}
	return out
}

// reviewersFromIDs builds a reviewer set from user IDs, preserving the state and
// timestamps of anyone already reviewing and defaulting newcomers to
// not-started as of now.
func (d *dataset) reviewersFromIDs(existing []domain.ReviewerInfo, userIDs []int64) []domain.ReviewerInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()
	prev := make(map[string]domain.ReviewerInfo, len(existing))
	for _, r := range existing {
		prev[r.Username] = r
	}
	out := make([]domain.ReviewerInfo, 0, len(userIDs))
	for _, id := range userIDs {
		p, ok := d.peopleByID[id]
		if !ok {
			continue
		}
		if r, ok := prev[p.Username]; ok {
			out = append(out, r)
			continue
		}
		out = append(out, domain.ReviewerInfo{
			Username: p.Username, Name: p.Name,
			State: domain.ReviewerNotStarted, WaitingSince: time.Now(),
		})
	}
	return out
}

func (d *dataset) usernamesFromIDs(userIDs []int64) map[string]bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		if p, ok := d.peopleByID[id]; ok {
			out[p.Username] = true
		}
	}
	return out
}
