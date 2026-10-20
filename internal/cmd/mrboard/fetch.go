package mrboardcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/core"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
)

func buildFetchCmd() *cobra.Command {
	var reviewerMRs bool
	var cold bool
	var filtered bool
	var view string
	var verbose bool
	var mergedSince string
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch MRs and print as JSON, mirroring the TUI's own fetch",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := fetchCmdOptions{cold: cold, filtered: filtered, verbose: verbose}
			if mergedSince != "" {
				since, err := parseMergedSince(mergedSince)
				if err != nil {
					return err
				}
				opts.mergedSince = since
			}
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
		"ignore the on-disk snapshot, merged snapshot and .gitattributes caches and recompute every MR "+
			"from scratch, bypassing incremental fetch (docs/adr/0005)")
	cmd.Flags().StringVar(&mergedSince, "merged-since", "",
		"also list MRs merged on or after this date (YYYY-MM-DD, local time), "+
			"caching them in the merged snapshot tagged with that date")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false,
		"list each MR's counted and excluded files behind its review size")
	return cmd
}

// fetchCmdOptions controls how closely this one-shot dump mirrors the TUI's
// own fetch, so reviewer-state and other discussion-derived bugs can be
// reproduced from the CLI without driving the interactive board.
type fetchCmdOptions struct {
	// reviewerMRsOverride, when set, replaces the TUI's saved IncludeReviewerMRs
	// setting for this run.
	reviewerMRsOverride *bool
	// cold, when true, fetches with no Previous snapshot and bypasses the
	// adapter's .gitattributes cache — every MR is recomputed from scratch
	// instead of reusing the on-disk caches.
	cold bool
	// filtered, when true, runs the fetched MRs through the same filter and sort
	// the board applies, using the TUI's saved state.
	filtered bool
	// viewOverride, when set, replaces the saved view mode and implies filtered.
	viewOverride *domain.ViewMode
	// verbose, when true, lists the counted and excluded files behind each MR's
	// review size.
	verbose bool
	// mergedSince, when set, also lists the MRs merged on or after it.
	mergedSince time.Time
}

// parseMergedSince reads a YYYY-MM-DD date as midnight local time: an MR counts
// from the start of that day wherever it is run.
func parseMergedSince(s string) (time.Time, error) {
	t, err := time.ParseInLocation(time.DateOnly, s, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("mrboard: --merged-since must be a YYYY-MM-DD date, got %q", s)
	}
	return t, nil
}

// fetchRequest is what one CLI fetch asks of the MR source.
type fetchRequest struct {
	includeReviewerMRs bool
	// cold bypasses every cache: no previous snapshot is diffed against, and the
	// adapter re-reads its own on-disk caches.
	cold bool
	// mergedSince, when set, also lists the MRs merged on or after it.
	mergedSince time.Time
}

// fetchMRs is the fetch every CLI command shares. It diffs against the board
// snapshot and, with mergedSince set, the merged snapshot listed for that date;
// it then replaces the merged snapshot with the merged MRs it listed, unless the
// fetch failed with none. The board snapshot is read only: the TUI owns it.
// Partial-result errors come back alongside the MRs.
func fetchMRs(ctx context.Context, c *core.Core, req fetchRequest) ([]domain.MergeRequest, []error, error) {
	var previous []domain.MergeRequest
	if !req.cold {
		board, _, err := c.SnapshotStore.Load()
		if err != nil {
			return nil, nil, fmt.Errorf("mrboard: loading snapshot: %w", err)
		}
		previous = board
		if !req.mergedSince.IsZero() {
			merged, err := c.MergedSnapshotStore.Load(req.mergedSince)
			if err != nil {
				return nil, nil, fmt.Errorf("mrboard: loading merged snapshot: %w", err)
			}
			previous = append(previous, merged...)
		}
	}

	mrs, errs := c.MRSource.FetchAll(ctx, mrsvc.FetchOptions{
		IncludeReviewerMRs: req.includeReviewerMRs,
		Previous:           previous,
		Cold:               req.cold,
		MergedSince:        req.mergedSince,
	})
	if !req.mergedSince.IsZero() {
		merged := domain.MergedMRs(mrs)
		if len(merged) > 0 || len(errs) == 0 {
			if err := c.MergedSnapshotStore.Save(req.mergedSince, merged); err != nil {
				errs = append(errs, fmt.Errorf("saving merged snapshot: %w", err))
			}
		}
	}
	return mrs, errs, nil
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

	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout(c))
	defer cancel()

	state, err := c.StateStore.Load()
	if err != nil {
		return fmt.Errorf("mrboard: loading app state: %w", err)
	}
	includeReviewerMRs := state.IncludeReviewerMRs
	if opts.reviewerMRsOverride != nil {
		includeReviewerMRs = *opts.reviewerMRsOverride
	}

	mrs, errs, err := fetchMRs(fetchCtx, c, fetchRequest{
		includeReviewerMRs: includeReviewerMRs,
		cold:               opts.cold,
		mergedSince:        opts.mergedSince,
	})
	if err != nil {
		return err
	}
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
	rs := c.Config.ReviewSize
	sizing := sizeOptions{
		exclusions: reviewSizeExclusions(rs),
		thresholds: rs.Buckets.Thresholds(),
		verbose:    opts.verbose,
	}
	return printJSON(mrs, c.Config.CurrentUser, sizing)
}

// reviewSizeExclusions is the per-MR exclusion lookup every command measures review
// size with, so `mrboard fetch` and the review load report the same size.
func reviewSizeExclusions(rs config.ReviewSize) domain.SizeExclusionsFor {
	return func(mr domain.MergeRequest) domain.SizeExclusions {
		return domain.NewSizeExclusions(rs.DefaultExcludes, rs.Exclude, mr.GeneratedRules)
	}
}

// sizeOptions controls how fetch output reports each MR's review size
// (docs/adr/0015, "Review size metric").
type sizeOptions struct {
	exclusions domain.SizeExclusionsFor
	thresholds domain.SizeThresholds
	verbose    bool
}

func printJSON(mrs []domain.MergeRequest, currentUser string, sizing sizeOptions) error {
	out := make([]mrJSON, 0, len(mrs))
	for _, mr := range mrs {
		out = append(out, toMRJSON(mr, currentUser, sizing))
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
	// MergedAt is set for a merged MR only (--merged-since), RFC 3339.
	MergedAt   string `json:"merged_at,omitempty"`
	RoundTrips int    `json:"round_trips"`
	// Roles and NeedsAttention are relative to the configured current_user and
	// omitted when none is set.
	Roles          []domain.Role `json:"roles,omitempty"`
	NeedsAttention bool          `json:"needs_attention,omitempty"`
	// ReviewSize is null when the MR's diff stats are unknown, and SizeBucket is
	// then "?". SizeFiles is set in verbose mode for an MR of known size.
	ReviewSize *int           `json:"review_size"`
	SizeBucket string         `json:"size_bucket"`
	SizeFiles  *sizeFilesJSON `json:"size_files,omitempty"`
}

// sizeFilesJSON lists the files behind an MR's review size.
type sizeFilesJSON struct {
	Counted  []domain.FileDiffStat `json:"counted"`
	Excluded []domain.FileDiffStat `json:"excluded"`
}

type reviewerJSON struct {
	Username   string `json:"username"`
	State      string `json:"state"`
	IsApprover bool   `json:"is_approver,omitempty"`
}

func mergedAtJSON(mr domain.MergeRequest) string {
	if !mr.Merged() {
		return ""
	}
	return mr.MergedAt.Format(time.RFC3339)
}

func toMRJSON(mr domain.MergeRequest, currentUser string, sizing sizeOptions) mrJSON {
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
	m := mr.MeasureReviewSize(sizing.exclusions.Of(mr))
	var reviewSize *int
	var sizeFiles *sizeFilesJSON
	if m.Size.Known {
		lines := m.Size.Lines
		reviewSize = &lines
		if sizing.verbose {
			sizeFiles = &sizeFilesJSON{
				Counted:  append([]domain.FileDiffStat{}, m.Counted...),
				Excluded: append([]domain.FileDiffStat{}, m.Excluded...),
			}
		}
	}
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
		MergedAt:          mergedAtJSON(mr),
		RoundTrips:        mr.RoundTripCount,
		Roles:             mr.RolesOf(currentUser),
		NeedsAttention:    mr.NeedsAttention(currentUser),
		ReviewSize:        reviewSize,
		SizeBucket:        string(sizing.thresholds.Bucket(m.Size)),
		SizeFiles:         sizeFiles,
	}
}
