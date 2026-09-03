package mrboardcmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/core"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

func buildUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Run mrboard's automatic write actions (currently: auto-assign reviewers)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return execUpdate(cmd.Context())
		},
	}
}

// execUpdate fetches the same way mrboard fetch does, then runs the same
// mrsvc.AutoAssignReviewers write the TUI performs automatically on every
// fetch — respecting auto_assign_reviewers.enabled exactly like the TUI does,
// so this command reproduces what the TUI would do rather than offering a way
// to bypass the config toggle (docs/adr/0009).
func execUpdate(ctx context.Context) error {
	c := ctx.Value(coreKey{}).(*core.Core)
	logger := c.Logger

	if !c.Config.AutoAssignReviewers.Enabled {
		logger.Info("mrboard: auto-assign reviewers is disabled, nothing to update")
		return nil
	}

	const defaultTimeout = 30 * time.Second
	timeout := c.Config.GitLab.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	updateCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	state, err := c.StateStore.Load()
	if err != nil {
		return fmt.Errorf("mrboard: loading app state: %w", err)
	}

	mrs, errs := c.MRSource.FetchAll(updateCtx, mrsvc.FetchOptions{IncludeReviewerMRs: state.IncludeReviewerMRs})
	for _, e := range errs {
		logger.Warn("mrboard: fetch partial error", "error", e)
	}

	var teamRoster []domain.User
	if usernames := config.TeamUsernames(c.Config.Sources); len(usernames) > 0 {
		teamRoster, err = c.MRSource.ResolveUsers(updateCtx, usernames)
		if err != nil {
			return fmt.Errorf("mrboard: resolving team roster: %w", err)
		}
	}

	matcher := domain.NewTicketKeyMatcher(c.Config.Jira.CaseInsensitiveTicketMatch)
	assigned := 0
	for _, mr := range mrs {
		reviewers, issueKey, ok := domain.AutoAssignCandidates(mr, teamRoster, matcher)
		if !ok {
			continue
		}
		writeErr := mrsvc.AutoAssignReviewers(updateCtx, c.MRSource, int64(mr.ProjectID), int64(mr.IID), reviewers)
		if writeErr != nil {
			logger.Warn("mrboard: auto-assign reviewers failed",
				"project_id", mr.ProjectID, "mr_iid", mr.IID, "ticket", issueKey, "err", writeErr)
			continue
		}
		assigned++
		logger.Info("mrboard: auto-assigned reviewers",
			"project_id", mr.ProjectID, "mr_iid", mr.IID, "ticket", issueKey, "reviewers", domain.Usernames(reviewers))
	}
	logger.Info("mrboard: update complete", "mrs", len(mrs), "assigned", assigned)
	return nil
}
