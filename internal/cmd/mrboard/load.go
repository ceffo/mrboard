package mrboardcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/core"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/domain/service/mrsvc"
	"github.com/ceffo/mrboard/internal/domain/service/ticketsvc"
	"github.com/ceffo/mrboard/internal/tui"
)

// Values of --group and --sort.
const (
	loadPerson = "person"
	loadJira   = "jira"
)

// Values of --color.
const (
	colorAuto   = "auto"
	colorAlways = "always"
	colorNever  = "never"
)

// loadCmdOptions are the flags of `mrboard load` (docs/adr/0015, "CLI command shape").
type loadCmdOptions struct {
	group         mrsvc.LoadGrouping
	sort          mrsvc.LoadSort
	remainingOnly bool
	json          bool
	color         string
	noFilters     bool
	cold          bool

	// viewOverride and reviewerMRsOverride replace the saved settings when set.
	viewOverride        *domain.ViewMode
	reviewerMRsOverride *bool
}

func buildLoadCmd() *cobra.Command {
	var group, sortBy, color, view string
	var remaining, asJSON, noFilters, cold, reviewerMRs bool
	cmd := &cobra.Command{
		Use:   "load",
		Short: "Print the active sprint's review load per approver or ticket",
		Long: "Print how much review size each approver carries in the active sprint: open MRs and MRs merged " +
			"since the sprint started, filtered as the board's header filters are (except sprint). Group, sort " +
			"and remaining-only are flags only and never saved.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := loadCmdOptions{
				remainingOnly: remaining, json: asJSON, color: color, noFilters: noFilters, cold: cold,
			}
			var err error
			if opts.group, err = parseLoadGroup(group); err != nil {
				return err
			}
			if opts.sort, err = parseLoadSort(sortBy); err != nil {
				return err
			}
			if color != colorAuto && color != colorAlways && color != colorNever {
				return fmt.Errorf("mrboard: --color must be \"auto\", \"always\" or \"never\", got %q", color)
			}
			if cmd.Flags().Changed("view") {
				mode, err := parseViewMode(view)
				if err != nil {
					return err
				}
				opts.viewOverride = &mode
			}
			if cmd.Flags().Changed("reviewer-mrs") {
				opts.reviewerMRsOverride = &reviewerMRs
			}
			return execLoad(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&group, "group", loadPerson, "group the tree by \"person\" or \"jira\" ticket")
	f.StringVar(&sortBy, "sort", "size", "order groups and lines by \"person\", \"jira\" or \"size\"")
	f.BoolVar(&remaining, "remaining", false, "only what is left to approve")
	f.BoolVar(&asJSON, "json", false, "emit the computed rows as JSON instead of the tree")
	f.StringVar(&color, "color", colorAuto,
		"\"auto\" colours on a terminal with NO_COLOR unset, \"always\", or \"never\"")
	f.BoolVar(&noFilters, "no-filters", false, "ignore the TUI's saved header filters and view scope")
	f.StringVar(&view, "view", "", "override the saved view scope: \"mine\" or \"all\"")
	f.BoolVar(&reviewerMRs, "reviewer-mrs", false,
		"include reviewer-sourced MRs (default: the TUI's saved setting)")
	f.BoolVar(&cold, "cold", false, "ignore the snapshot, merged snapshot and .gitattributes caches")
	return cmd
}

func parseLoadGroup(s string) (mrsvc.LoadGrouping, error) {
	switch s {
	case loadPerson:
		return mrsvc.LoadByPerson, nil
	case loadJira:
		return mrsvc.LoadByTicket, nil
	}
	return mrsvc.LoadByPerson, fmt.Errorf("mrboard: --group must be \"person\" or \"jira\", got %q", s)
}

func parseLoadSort(s string) (mrsvc.LoadSort, error) {
	switch s {
	case loadPerson:
		return mrsvc.LoadSortPerson, nil
	case loadJira:
		return mrsvc.LoadSortTicket, nil
	case "size":
		return mrsvc.LoadSortSize, nil
	}
	return mrsvc.LoadSortSize, fmt.Errorf("mrboard: --sort must be \"person\", \"jira\" or \"size\", got %q", s)
}

// activeSprint reads the sprint the load is scoped to. A load without a usable
// sprint is an error, never a near-empty report: with no issue keys every
// ticketed MR would drop out of it silently.
func activeSprint(ctx context.Context, c *core.Core) (*ticketsvc.Sprint, error) {
	if c.TicketEnricher == nil || c.Config.Jira.BoardID == 0 {
		return nil, errors.New("mrboard: the load is scoped to the active sprint, but no ticket board is configured")
	}
	sprint, err := c.TicketEnricher.GetActiveSprint(ctx, c.Config.Jira.BoardID)
	if err != nil {
		return nil, fmt.Errorf("mrboard: reading the active sprint: %w", err)
	}
	switch {
	case sprint == nil:
		return nil, errors.New("mrboard: the board has no active sprint, so there is no load to report")
	case len(sprint.IssueKeys) == 0:
		return nil, fmt.Errorf("mrboard: sprint %q has no issues, so there is no load to report", sprint.Name)
	case sprint.Start.IsZero():
		return nil, fmt.Errorf("mrboard: sprint %q reports no start date, so merged MRs cannot be bounded",
			sprint.Name)
	}
	return sprint, nil
}

// loadFilter is the header filter the load applies: the board's saved filters
// and view scope, or none under --no-filters. A --view override applies either way.
func loadFilter(
	opts loadCmdOptions, state domain.AppState, currentUser string, km domain.TicketKeyMatcher,
) mrsvc.FilterOptions {
	if opts.noFilters {
		state = domain.AppState{}
		state.ViewMode = domain.ViewAll
	}
	view := state.ViewMode
	if opts.viewOverride != nil {
		view = *opts.viewOverride
	}
	return boardFilterOptions(state, view, currentUser, km)
}

func execLoad(ctx context.Context, opts loadCmdOptions, out io.Writer) error {
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

	sprint, err := activeSprint(fetchCtx, c)
	if err != nil {
		return err
	}
	start := sprint.StartDay()
	mrs, errs, err := fetchMRs(fetchCtx, c, fetchRequest{
		includeReviewerMRs: includeReviewerMRs,
		cold:               opts.cold,
		mergedSince:        start,
	})
	if err != nil {
		return err
	}
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "mrboard: fetch error: %v\n", e)
	}
	if len(mrs) == 0 && len(errs) > 0 {
		return errors.New("mrboard: nothing fetched, so there is no load to report")
	}

	km := domain.NewTicketKeyMatcher(c.Config.Jira.CaseInsensitiveTicketMatch)
	keys := make(map[string]bool, len(sprint.IssueKeys))
	for _, k := range sprint.IssueKeys {
		keys[k] = true
	}
	rs := c.Config.ReviewSize
	load := mrsvc.ComputeLoad(mrs, mrsvc.LoadOptions{
		SprintStart:   start,
		SprintKeys:    keys,
		KeyMatcher:    km,
		Team:          config.TeamUsernames(c.Config.Sources),
		Filter:        loadFilter(opts, state, c.Config.CurrentUser, km),
		Exclusions:    rs.ExclusionsFor(),
		Thresholds:    rs.Buckets.Thresholds(),
		Group:         opts.group,
		Sort:          opts.sort,
		RemainingOnly: opts.remainingOnly,
	})

	view := tui.LoadSprint{Name: sprint.Name, Start: start, End: sprint.End}
	partial := len(errs) > 0
	if opts.json {
		err = writeLoadJSON(out, load, view, partial)
	} else {
		var styles tui.Styles
		if useColor(opts.color, out) {
			styles = tui.NewLoadStyles(state.ThemeName, state.ThemeMode)
		}
		_, err = io.WriteString(out, tui.RenderLoad(load, view, styles))
	}
	if err != nil {
		return err
	}
	if partial {
		return fmt.Errorf("mrboard: the load is partial: %d fetch %s failed, so MRs may be missing",
			len(errs), plural(len(errs), "error", "errors"))
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// fetchTimeout bounds one CLI fetch by the configured GitLab timeout.
func fetchTimeout(c *core.Core) time.Duration {
	const defaultTimeout = 30 * time.Second
	if t := c.Config.GitLab.Timeout; t != 0 {
		return t
	}
	return defaultTimeout
}

// useColor resolves --color: "auto" colours only a terminal with NO_COLOR unset.
func useColor(mode string, out io.Writer) bool {
	switch mode {
	case colorAlways:
		return true
	case colorNever:
		return false
	}
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	f, ok := out.(*os.File)
	return ok && isatty.IsTerminal(f.Fd())
}

type loadJSON struct {
	Sprint        loadSprintJSON `json:"sprint"`
	Group         string         `json:"group"`
	Sort          string         `json:"sort"`
	RemainingOnly bool           `json:"remaining_only"`
	// Partial is true when the fetch reported errors: MRs may be missing from the load.
	Partial         bool            `json:"partial"`
	MRCount         int             `json:"mr_count"`
	PopulationCount int             `json:"population_count"`
	TeamTotal       loadTotalJSON   `json:"team_total"`
	Groups          []loadGroupJSON `json:"groups"`
}

type loadSprintJSON struct {
	Name  string `json:"name"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// loadTotalJSON is a sum of review sizes; Partial means some summed size was
// unknown, so Lines is a lower bound.
type loadTotalJSON struct {
	Lines   int  `json:"lines"`
	Partial bool `json:"partial"`
}

type loadGroupJSON struct {
	Key      string         `json:"key"`
	Name     string         `json:"name"`
	CatchAll bool           `json:"catch_all"`
	Total    loadTotalJSON  `json:"total"`
	Lines    []loadLineJSON `json:"lines"`
}

type loadLineJSON struct {
	Project string `json:"project"`
	IID     int    `json:"iid"`
	Title   string `json:"title"`
	WebURL  string `json:"web_url"`
	Ticket  string `json:"ticket,omitempty"`
	// ReviewSize is null when the MR's diff stats are unknown, and SizeBucket is then "?".
	ReviewSize *int               `json:"review_size"`
	SizeBucket string             `json:"size_bucket"`
	Draft      bool               `json:"draft"`
	Merged     bool               `json:"merged"`
	Approver   string             `json:"approver,omitempty"`
	Approved   bool               `json:"approved"`
	Approvers  []loadApproverJSON `json:"approvers"`
}

type loadApproverJSON struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Approved bool   `json:"approved"`
	InTeam   bool   `json:"in_team"`
}

func writeLoadJSON(out io.Writer, load mrsvc.Load, sprint tui.LoadSprint, partial bool) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(toLoadJSON(load, sprint, partial))
}

func toLoadJSON(load mrsvc.Load, sprint tui.LoadSprint, partial bool) loadJSON {
	groupNames := map[mrsvc.LoadGrouping]string{mrsvc.LoadByPerson: loadPerson, mrsvc.LoadByTicket: loadJira}
	sortNames := map[mrsvc.LoadSort]string{
		mrsvc.LoadSortPerson: loadPerson, mrsvc.LoadSortTicket: loadJira, mrsvc.LoadSortSize: "size",
	}
	out := loadJSON{
		Sprint:          loadSprintJSON{Name: sprint.Name, Start: rfc3339(sprint.Start), End: rfc3339(sprint.End)},
		Group:           groupNames[load.Group],
		Sort:            sortNames[load.Sort],
		RemainingOnly:   load.RemainingOnly,
		Partial:         partial,
		MRCount:         load.MRCount,
		PopulationCount: load.PopulationCount,
		TeamTotal:       loadTotalJSON(load.TeamTotal),
		Groups:          make([]loadGroupJSON, 0, len(load.Groups)),
	}
	for _, g := range load.Groups {
		jg := loadGroupJSON{
			Key: g.Key, Name: g.Name, CatchAll: g.CatchAll,
			Total: loadTotalJSON(g.Total), Lines: make([]loadLineJSON, 0, len(g.Lines)),
		}
		for _, l := range g.Lines {
			jg.Lines = append(jg.Lines, toLoadLineJSON(l))
		}
		out.Groups = append(out.Groups, jg)
	}
	return out
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func toLoadLineJSON(l mrsvc.LoadLine) loadLineJSON {
	var size *int
	if l.Size.Known {
		lines := l.Size.Lines
		size = &lines
	}
	approvers := make([]loadApproverJSON, 0, len(l.Approvers))
	for _, a := range l.Approvers {
		approvers = append(approvers, loadApproverJSON(a))
	}
	return loadLineJSON{
		Project:    l.MR.ProjectPath,
		IID:        l.MR.IID,
		Title:      l.MR.Title,
		WebURL:     l.MR.WebURL,
		Ticket:     l.Ticket,
		ReviewSize: size,
		SizeBucket: string(l.Bucket),
		Draft:      l.Draft,
		Merged:     l.Merged,
		Approver:   l.Approver,
		Approved:   l.Approved,
		Approvers:  approvers,
	}
}
