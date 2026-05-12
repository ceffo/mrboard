// Package main is the entry point for mrboard.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/ceffo/mrboard/internal/app"
	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/tui"
)

const (
	defaultTimeout = 30 * time.Second
	logFileMode    = 0o600
)

// Version is set at build time via -ldflags "-X main.Version=<tag>".
var Version = "dev"

func main() {
	if err := buildRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func buildRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "mrboard",
		Short: "GitLab MR review board for daily standups",
		Long: `mrboard displays GitLab merge requests in a kanban board.

Config search path (first match wins):
  $MRBOARD_CONFIG
  $XDG_CONFIG_HOME/mrboard/mrboard.yaml  (default: ~/.config/mrboard/mrboard.yaml)
  ./mrboard.yaml

Environment:
  MRBOARD_CONFIG   Explicit config file path
  GITLAB_TOKEN     Override gitlab.token from config
  MRBOARD_TIMEOUT  HTTP timeout (default: 30s, e.g. "60s")
  MRBOARD_DEBUG    Write debug logs to this file path`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return execBoard()
		},
	}

	root.AddCommand(buildRunCmd())
	root.AddCommand(buildFetchCmd())
	root.AddCommand(buildVersionCmd())

	return root
}

func buildRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Launch the TUI board (default)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return execBoard()
		},
	}
}

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

func buildVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print mrboard version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(Version)
		},
	}
}

func execBoard() error {
	svc, err := app.New(loadTimeout(), nil)
	if err != nil {
		return err
	}
	st := config.LoadState()
	m := tui.New(svc.Config, svc.MRSource, st, Version)
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func execFetch(debugPath string) error {
	if debugPath == "" {
		debugPath = os.Getenv("MRBOARD_DEBUG")
	}
	logger := setupLogger(debugPath)

	timeout := loadTimeout()
	svc, err := app.New(timeout, logger)
	if err != nil {
		return err
	}
	svc.Logger.Debug("config loaded",
		"sources", len(svc.Config.Sources),
		"excluded_authors", svc.Config.ExcludedAuthors,
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	mrs, errs := svc.MRSource.FetchAll(ctx)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "mrboard: fetch error: %v\n", e)
	}
	if len(mrs) == 0 && len(errs) > 0 {
		os.Exit(1)
	}

	out := make([]mrJSON, 0, len(mrs))
	for _, mr := range mrs {
		out = append(out, toJSON(mr))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func setupLogger(path string) *slog.Logger {
	if path == "" {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: open debug log %q: %v\n", path, err)
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func loadTimeout() time.Duration {
	if v := os.Getenv("MRBOARD_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
		fmt.Fprintf(os.Stderr, "mrboard: invalid MRBOARD_TIMEOUT %q, using default\n", v)
	}
	return defaultTimeout
}

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

func toJSON(mr domain.MergeRequest) mrJSON {
	reviewers := make([]reviewerJSON, 0, len(mr.Reviewers))
	for _, r := range mr.Reviewers {
		reviewers = append(reviewers, reviewerJSON{
			Username: r.Username,
			State:    reviewerStateName(r.State),
		})
	}
	now := time.Now()
	return mrJSON{
		ID:             mr.ID,
		Title:          mr.Title,
		WebURL:         mr.WebURL,
		Phase:          phaseName(mr.Phase),
		Author:         mr.Author,
		ReviewerStates: reviewers,
		TimeInPhase:    domain.FormatDuration(now.Sub(mr.WaitingSince)),
		TimeOpen:       domain.FormatDuration(now.Sub(mr.CreatedAt)),
		RoundTrips:     mr.RoundTripCount,
	}
}

func phaseName(p domain.MRPhase) string {
	switch p {
	case domain.PhaseDraft:
		return "draft"
	case domain.PhaseNeedsReview:
		return "needs_review"
	case domain.PhaseNeedsAuthorAction:
		return "needs_author_action"
	case domain.PhaseReadyToMerge:
		return "ready_to_merge"
	default:
		return "unknown"
	}
}

func reviewerStateName(s domain.ReviewerState) string {
	switch s {
	case domain.ReviewerNotStarted:
		return "not_started"
	case domain.ReviewerCommented:
		return "commented"
	case domain.ReviewerReReviewRequested:
		return "re_review_requested"
	case domain.ReviewerApproved:
		return "approved"
	default:
		return "unknown"
	}
}
