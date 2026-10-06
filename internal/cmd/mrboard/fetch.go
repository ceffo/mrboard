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
	var reviewerMRs bool
	var cold bool
	var filtered bool
	var view string
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch MRs and print as JSON, mirroring the TUI's own fetch",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := fetchCmdOptions{cold: cold, filtered: filtered}
			if cmd.Flags().Changed("reviewer-mrs") {
				opts.reviewerMRsOverride = &reviewerMRs
			}
			if cmd.Flags().Changed("view") {
				mode, err := parseViewMode(view)
				if err != nil {
					return err
				}
				opts.viewOverride = &mode
			}
			return execFetch(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&filtered, "filtered", false,
		"apply the TUI's saved view mode, filters and sort, as the board does")
	cmd.Flags().StringVar(&view, "view", "",
		"override the saved view mode: \"mine\" or \"all\" (implies --filtered)")
	cmd.Flags().BoolVar(&reviewerMRs, "reviewer-mrs", false,
		"include reviewer-sourced MRs (default: the TUI's saved setting)")
	cmd.Flags().BoolVar(&cold, "cold", false,
		"ignore the on-disk snapshot and recompute every MR from scratch, "+
			"bypassing incremental fetch (docs/adr/0005)")
	return cmd
}

// fetchCmdOptions controls how closely this one-shot dump mirrors the TUI's
// own fetch, so reviewer-state and other discussion-derived bugs can be
// reproduced from the CLI without driving the interactive board.
type fetchCmdOptions struct {
	// reviewerMRsOverride, when set, replaces the TUI's saved IncludeReviewerMRs
	// setting for this run.
	reviewerMRsOverride *bool
	// cold, when true, fetches with no Previous snapshot — every MR is
	// recomputed from scratch instead of reusing the on-disk cache.
	cold bool
	// filtered, when true, runs the fetched MRs through the same filter and sort
	// the board applies, using the TUI's saved state.
	filtered bool
	// viewOverride, when set, replaces the saved view mode and implies filtered.
	viewOverride *domain.ViewMode
}

func parseViewMode(s string) (domain.ViewMode, error) {
	switch s {
	case "mine":
		return domain.ViewMine, nil
	case "all":
		return domain.ViewAll, nil
	}
	return domain.ViewAll, fmt.Errorf("mrboard: --view must be \"mine\" or \"all\", got %q", s)
}

// boardFilterOptions builds the filter the board would apply for the saved
// state. A view mode of "mine" without a current user degrades to "all", as the
// TUI does at startup.
func boardFilterOptions(
	state domain.AppState, viewMode domain.ViewMode, currentUser string, km domain.TicketKeyMatcher,
) mrsvc.FilterOptions {
	if viewMode == domain.ViewMine && currentUser == "" {
		viewMode = domain.ViewAll
	}
	return mrsvc.FilterOptions{
		MyView:             viewMode == domain.ViewMine,
		CurrentUser:        currentUser,
		SortField:          state.SortField,
		SortDesc:           state.SortDesc,
		Phases:             state.Filter.Phases,
		ExcludedAssignees:  state.Filter.ExcludedAssignees,
		ExcludedReviewers:  state.Filter.ExcludedReviewers,
		ExcludedTicketKeys: state.Filter.ExcludedTicketKeys,
		ExcludeTicketless:  state.Filter.ExcludeTicketless,
		KeyMatcher:         km,
	}
}

func execFetch(ctx context.Context, opts fetchCmdOptions) error {
	c := ctx.Value(coreKey{}).(*core.Core)

	const defaultTimeout = 30 * time.Second
	timeout := c.Config.GitLab.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	state, err := c.StateStore.Load()
	if err != nil {
		return fmt.Errorf("mrboard: loading app state: %w", err)
	}
	includeReviewerMRs := state.IncludeReviewerMRs
	if opts.reviewerMRsOverride != nil {
		includeReviewerMRs = *opts.reviewerMRsOverride
	}

	var previous []domain.MergeRequest
	if !opts.cold {
		previous, _, err = c.SnapshotStore.Load()
		if err != nil {
			return fmt.Errorf("mrboard: loading snapshot: %w", err)
		}
	}

	mrs, errs := c.MRSource.FetchAll(fetchCtx, mrsvc.FetchOptions{
		IncludeReviewerMRs: includeReviewerMRs,
		Previous:           previous,
	})
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "mrboard: fetch error: %v\n", e)
	}
	if len(mrs) == 0 && len(errs) > 0 {
		os.Exit(1)
	}
	if opts.filtered || opts.viewOverride != nil {
		viewMode := state.ViewMode
		if opts.viewOverride != nil {
			viewMode = *opts.viewOverride
		}
		km := domain.NewTicketKeyMatcher(c.Config.Jira.CaseInsensitiveTicketMatch)
		mrs = mrsvc.FilterAndSort(mrs, boardFilterOptions(state, viewMode, c.Config.CurrentUser, km))
	}
	return printJSON(mrs, c.Config.CurrentUser)
}

func printJSON(mrs []domain.MergeRequest, currentUser string) error {
	out := make([]mrJSON, 0, len(mrs))
	for _, mr := range mrs {
		out = append(out, toMRJSON(mr, currentUser))
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
	// Roles and NeedsAttention are relative to the configured current_user and
	// omitted when none is set.
	Roles          []domain.Role `json:"roles,omitempty"`
	NeedsAttention bool          `json:"needs_attention,omitempty"`
}

type reviewerJSON struct {
	Username   string `json:"username"`
	State      string `json:"state"`
	IsApprover bool   `json:"is_approver,omitempty"`
}

func toMRJSON(mr domain.MergeRequest, currentUser string) mrJSON {
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
		Roles:             mr.RolesOf(currentUser),
		NeedsAttention:    mr.NeedsAttention(currentUser),
	}
}
