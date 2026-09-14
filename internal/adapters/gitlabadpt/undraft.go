package gitlabadpt

import "context"

// Undraft implements mrsvc.MergeRequestSource.
func (a *GitLabAdapter) Undraft(ctx context.Context, projectID, mrIID int64) error {
	return a.client.Undraft(ctx, projectID, mrIID)
}
