package mrboardcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ceffo/mrboard/internal/core"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

func buildFetchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fetch",
		Short: "Fetch MRs and print as JSON",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return execFetch(cmd.Context())
		},
	}
}

func execFetch(ctx context.Context) error {
	c := ctx.Value(coreKey{}).(*core.Core)

	const defaultTimeout = 30 * time.Second
	timeout := c.Config.GitLab.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	mrs, errs := c.MRSource.FetchAll(fetchCtx, mrsvc.FetchOptions{})
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "mrboard: fetch error: %v\n", e)
	}
	if len(mrs) == 0 && len(errs) > 0 {
		os.Exit(1)
	}
	return printJSON(mrs)
}

func printJSON(mrs []domain.MergeRequest) error {
	out := make([]mrJSON, 0, len(mrs))
	for _, mr := range mrs {
		out = append(out, toMRJSON(mr))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

type mrJSON struct {
	ID                int            `json:"id"`
	Title             string         `json:"title"`
	WebURL            string         `json:"web_url"`
	Phase             string         `json:"phase"`
	Author            string         `json:"author"`
	ReviewerStates    []reviewerJSON `json:"reviewer_states"`
	ApprovalsRequired int            `json:"approvals_required,omitempty"`
	TimeInPhase       string         `json:"time_in_phase"`
	TimeOpen          string         `json:"time_open"`
	RoundTrips        int            `json:"round_trips"`
}

type reviewerJSON struct {
	Username   string `json:"username"`
	State      string `json:"state"`
	IsApprover bool   `json:"is_approver,omitempty"`
}

func toMRJSON(mr domain.MergeRequest) mrJSON {
	reviewers := make([]reviewerJSON, 0, len(mr.Reviewers))
	approvalsRequired := 0
	for _, r := range mr.Reviewers {
		if r.IsApprover {
			approvalsRequired++
		}
		reviewers = append(reviewers, reviewerJSON{
			Username:   r.Username,
			State:      r.State.String(),
			IsApprover: r.IsApprover,
		})
	}
	now := time.Now()
	return mrJSON{
		ID:                mr.ID,
		Title:             mr.Title,
		WebURL:            mr.WebURL,
		Phase:             mr.Phase.String(),
		Author:            mr.Author,
		ReviewerStates:    reviewers,
		ApprovalsRequired: approvalsRequired,
		TimeInPhase:       domain.FormatDuration(now.Sub(mr.WaitingSince)),
		TimeOpen:          domain.FormatDuration(now.Sub(mr.CreatedAt)),
		RoundTrips:        mr.RoundTripCount,
	}
}
