package gitlabadpt

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ceffo/mrboard/internal/domain"
	ilog "github.com/ceffo/mrboard/internal/log"
)

// jiraMarker is the HTML comment appended alongside the JIRA link as a sentinel.
// Its presence in the description means the link has already been injected.
const jiraMarker = "<!-- mrboard -->"

// injectJiraLinksBackground fires a background goroutine for each MR that has a
// JIRA ID in its title. Each goroutine fetches the description, checks for the
// marker, and writes the link once if the marker is absent. No-op when
// JiraInstanceURL is not configured.
func (a *GitLabAdapter) injectJiraLinksBackground(ctx context.Context, mrs []domain.MergeRequest) {
	if a.cfg.JiraInstanceURL == "" {
		return
	}
	logger := ilog.FromContext(ctx)
	bgCtx := context.WithoutCancel(ctx)
	for _, mr := range mrs {
		issueKey := domain.ExtractJiraID(mr.Title)
		if issueKey == "" {
			continue
		}
		mr := mr
		go func() {
			err := a.maybeInjectJiraLink(bgCtx, int64(mr.ProjectID), int64(mr.IID), issueKey, logger)
			if err != nil {
				logger.Warn("gitlab: jira link inject failed",
					"project_id", mr.ProjectID, "mr_iid", mr.IID, "issue_key", issueKey, "error", err)
			}
		}()
	}
}

// maybeInjectJiraLink fetches the MR description and appends a JIRA link when
// the mrboard marker is absent. Idempotent — no write is performed if the marker
// is already present.
func (a *GitLabAdapter) maybeInjectJiraLink(
	ctx context.Context,
	projectID, mrIID int64,
	issueKey string,
	logger *slog.Logger,
) error {
	desc, err := a.client.GetMRDescription(ctx, projectID, mrIID)
	if err != nil {
		return fmt.Errorf("get description project=%d MR=%d: %w", projectID, mrIID, err)
	}
	if strings.Contains(desc, jiraMarker) {
		return nil
	}
	newDesc := appendJiraLink(desc, a.cfg.JiraInstanceURL, issueKey)
	if err := a.client.UpdateMRDescription(ctx, projectID, mrIID, newDesc); err != nil {
		return fmt.Errorf("update description project=%d MR=%d: %w", projectID, mrIID, err)
	}
	logger.Info("gitlab: jira link injected",
		"project_id", projectID, "mr_iid", mrIID, "issue_key", issueKey)
	return nil
}

// appendJiraLink builds the updated description by appending the JIRA link line.
// Format follows the design: existing body + "\n---\n🎫 [KEY](url) <!-- mrboard -->"
func appendJiraLink(desc, instanceURL, issueKey string) string {
	url := domain.JiraIssueURL(instanceURL, issueKey)
	suffix := fmt.Sprintf("---\n🎫 [%s](%s) %s", issueKey, url, jiraMarker)
	if desc == "" {
		return suffix
	}
	return desc + "\n" + suffix
}
