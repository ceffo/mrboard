package demoadpt

import (
	"context"
	"fmt"
	"time"

	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

// mrSource implements mrsvc.MergeRequestSource against the in-memory dataset.
// Writes mutate the dataset, so the next fetch reflects them — that is what
// makes the reviewer editor feel real in demo mode instead of reverting.
type mrSource struct{ a *Adapter }

// FetchAll returns the whole dataset. Options are honoured only where they are
// observable: ReviewerSource MRs are filtered out unless asked for. There is no
// two-phase diffing to do, since every read is already local.
func (s *mrSource) FetchAll(ctx context.Context, opts mrsvc.FetchOptions) ([]domain.MergeRequest, []error) {
	if err := s.sleep(ctx); err != nil {
		return nil, []error{err}
	}
	all := s.a.ds.all()
	out := make([]domain.MergeRequest, 0, len(all))
	for _, mr := range all {
		if mr.ReviewerSource && !opts.IncludeReviewerMRs {
			continue
		}
		out = append(out, mr)
	}
	s.a.logger.Info("demo: fetch done", "mrs", len(out), "include_reviewer_mrs", opts.IncludeReviewerMRs)
	return out, nil
}

func (s *mrSource) GetDetail(ctx context.Context, projectID, mrIID int64) (string, []domain.Thread, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	key := domain.MRKey{ProjectID: int(projectID), IID: int(mrIID)}
	mr, ok := s.a.ds.find(key)
	if !ok {
		return "", nil, fmt.Errorf("demo: no MR %d!%d", projectID, mrIID)
	}
	return mr.Description, s.a.ds.threadsFor(key), nil
}

func (s *mrSource) FetchMR(ctx context.Context, projectID, mrIID int64) (domain.MergeRequest, error) {
	if err := ctx.Err(); err != nil {
		return domain.MergeRequest{}, err
	}
	mr, ok := s.a.ds.find(domain.MRKey{ProjectID: int(projectID), IID: int(mrIID)})
	if !ok {
		return domain.MergeRequest{}, fmt.Errorf("demo: no MR %d!%d", projectID, mrIID)
	}
	return mr, nil
}

func (s *mrSource) GetProjectMembers(ctx context.Context, projectID int64) ([]domain.ProjectMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.a.ds.membersOf(int(projectID)), nil
}

// SaveApprovers flips IsApprover to match userIDs. The dataset re-derives the
// phase afterwards, so promoting the last outstanding approver visibly moves the
// card into the Approved column.
func (s *mrSource) SaveApprovers(ctx context.Context, projectID, mrIID int64, userIDs []int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	approvers := s.a.ds.usernamesFromIDs(userIDs)
	key := domain.MRKey{ProjectID: int(projectID), IID: int(mrIID)}
	if !s.a.ds.mutate(key, func(mr *domain.MergeRequest) {
		for i := range mr.Reviewers {
			mr.Reviewers[i].IsApprover = approvers[mr.Reviewers[i].Username]
		}
	}) {
		return fmt.Errorf("demo: no MR %d!%d", projectID, mrIID)
	}
	s.a.logger.Info("demo: approvers saved", "project_id", projectID, "mr_iid", mrIID, "count", len(userIDs))
	return nil
}

// SetReviewers replaces the reviewer set. Reviewers who survive keep their state
// and timestamps; newcomers start as not-started.
func (s *mrSource) SetReviewers(ctx context.Context, projectID, mrIID int64, userIDs []int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := domain.MRKey{ProjectID: int(projectID), IID: int(mrIID)}
	mr, ok := s.a.ds.find(key)
	if !ok {
		return fmt.Errorf("demo: no MR %d!%d", projectID, mrIID)
	}
	// Computed before mutate: the dataset lock is not reentrant.
	next := s.a.ds.reviewersFromIDs(mr.Reviewers, userIDs)
	s.a.ds.mutate(key, func(m *domain.MergeRequest) { m.Reviewers = next })
	s.a.logger.Info("demo: reviewers set", "project_id", projectID, "mr_iid", mrIID, "count", len(next))
	return nil
}

func (s *mrSource) GetDiff(ctx context.Context, projectID, mrIID int64) (domain.MRDiff, error) {
	if err := ctx.Err(); err != nil {
		return domain.MRDiff{}, err
	}
	diff, ok := s.a.ds.diffFor(domain.MRKey{ProjectID: int(projectID), IID: int(mrIID)})
	if !ok {
		return domain.MRDiff{}, fmt.Errorf("demo: no diff for %d!%d", projectID, mrIID)
	}
	return diff, nil
}

// GetFileContent always reports absence. The diff view treats that as "no blob
// available" and renders with its built-in colouriser, which is the path the
// demo is meant to show — shipping side-by-side blobs would make the output
// depend on whether an external differ happens to be installed.
func (s *mrSource) GetFileContent(_ context.Context, _ int64, path, ref string) ([]byte, error) {
	return nil, fmt.Errorf("demo: no blob for %s@%s", path, ref)
}

func (s *mrSource) ResolveUsers(ctx context.Context, usernames []string) ([]domain.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.a.ds.resolve(usernames), nil
}

// UpdateDescription accepts and applies the write, so the back-link path is
// genuinely exercised rather than silently swallowed.
func (s *mrSource) UpdateDescription(ctx context.Context, projectID, mrIID int64, description string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := domain.MRKey{ProjectID: int(projectID), IID: int(mrIID)}
	if !s.a.ds.mutate(key, func(mr *domain.MergeRequest) { mr.Description = description }) {
		return fmt.Errorf("demo: no MR %d!%d", projectID, mrIID)
	}
	return nil
}

// sleep waits out the configured latency, or returns early if the caller's
// context is cancelled first.
func (s *mrSource) sleep(ctx context.Context) error {
	if s.a.latency <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(s.a.latency)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
