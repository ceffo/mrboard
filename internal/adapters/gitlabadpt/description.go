package gitlabadpt

import "context"

// UpdateDescription implements mrsvc.MergeRequestSource. It is a mechanical
// passthrough — deciding what the new description should contain, and whether
// writing it is even necessary, is the caller's responsibility.
func (a *GitLabAdapter) UpdateDescription(ctx context.Context, projectID, mrIID int64, description string) error {
	return a.client.UpdateMRDescription(ctx, projectID, mrIID, description)
}
