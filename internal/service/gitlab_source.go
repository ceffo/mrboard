package service

import (
	"fmt"

	"github.com/mrboard/mrboard/internal/config"
	"github.com/mrboard/mrboard/internal/domain"
	"github.com/mrboard/mrboard/internal/gitlab"
)

// GitLabSource is the concrete adapter that satisfies MergeRequestSource using
// a live GitLab API client. It is the only place in the codebase that knows
// about both the service port and the gitlab infrastructure package.
type GitLabSource struct {
	client *gitlab.Client
	cfg    *config.Config
}

// NewGitLabSource constructs a GitLabSource adapter.
func NewGitLabSource(client *gitlab.Client, cfg *config.Config) *GitLabSource {
	return &GitLabSource{client: client, cfg: cfg}
}

// FetchAll implements MergeRequestSource.
func (s *GitLabSource) FetchAll() ([]domain.MergeRequest, []error) {
	return gitlab.FetchAll(s.client, s.cfg)
}

// GetDetail implements MergeRequestSource.
func (s *GitLabSource) GetDetail(projectID, mrIID int64) (string, []domain.Thread, error) {
	desc, err := s.client.GetMRDescription(projectID, mrIID)
	if err != nil {
		return "", nil, fmt.Errorf("get detail project=%d MR=%d description: %w", projectID, mrIID, err)
	}
	discussions, err := s.client.GetMRDiscussions(projectID, mrIID)
	if err != nil {
		return desc, nil, fmt.Errorf("get detail project=%d MR=%d discussions: %w", projectID, mrIID, err)
	}
	threads := gitlab.MapDiscussionsToThreads(discussions)
	return desc, threads, nil
}
