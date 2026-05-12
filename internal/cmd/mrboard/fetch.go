package mrboardcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ceffo/mrboard/internal/app"
	"github.com/ceffo/mrboard/internal/domain"
)

func buildFetchCmd() *cobra.Command {
	var debugPath string
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch MRs and print as JSON",
		RunE: func(_ *cobra.Command, _ []string) error {
			return execFetch(debugPath)
		},
	}
	cmd.Flags().StringVar(&debugPath, "debug", "", "write debug logs to this path (overrides $MRBOARD_DEBUG)")
	return cmd
}

func execFetch(debugPath string) error {
	if debugPath == "" {
		debugPath = os.Getenv("MRBOARD_DEBUG")
	}

	timeout := app.TimeoutFromEnv()
	svc, err := app.New(timeout, app.LoggerFromPath(debugPath))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	mrs, errs := svc.MRSource.FetchAll(ctx)

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

// mrJSON is the JSON representation of a MergeRequest for the fetch command.
type mrJSON struct {
	ID             int            `json:"id"`
	Title          string         `json:"title"`
	WebURL         string         `json:"web_url"`
	Phase          string         `json:"phase"`
	Author         string         `json:"author"`
	ReviewerStates []reviewerJSON `json:"reviewer_states"`
	TimeInPhase    string         `json:"time_in_phase"`
	TimeOpen       string         `json:"time_open"`
	RoundTrips     int            `json:"round_trips"`
}

type reviewerJSON struct {
	Username string `json:"username"`
	State    string `json:"state"`
}

func toMRJSON(mr domain.MergeRequest) mrJSON {
	reviewers := make([]reviewerJSON, 0, len(mr.Reviewers))
	for _, r := range mr.Reviewers {
		reviewers = append(reviewers, reviewerJSON{
			Username: r.Username,
			State:    r.State.String(),
		})
	}
	now := time.Now()
	return mrJSON{
		ID:             mr.ID,
		Title:          mr.Title,
		WebURL:         mr.WebURL,
		Phase:          mr.Phase.String(),
		Author:         mr.Author,
		ReviewerStates: reviewers,
		TimeInPhase:    domain.FormatDuration(now.Sub(mr.WaitingSince)),
		TimeOpen:       domain.FormatDuration(now.Sub(mr.CreatedAt)),
		RoundTrips:     mr.RoundTripCount,
	}
}
