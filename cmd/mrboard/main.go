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

	"github.com/ceffo/mrboard/internal/app"
	"github.com/ceffo/mrboard/internal/config"
	"github.com/ceffo/mrboard/internal/domain"
	"github.com/ceffo/mrboard/internal/tui"
)

const (
	defaultTimeout = 30 * time.Second
	minArgs        = 2
	logFileMode    = 0o600
)

// Version is set at build time via -ldflags "-X main.Version=<tag>".
var Version = "dev"

func main() {
	if len(os.Args) < minArgs {
		os.Exit(runBoard())
	}

	switch os.Args[1] {
	case "fetch":
		os.Exit(runFetch(os.Args[2:]))
	case "run":
		os.Exit(runBoard())
	case "version":
		fmt.Println(Version)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "mrboard: unknown subcommand %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`mrboard — GitLab MR review board

Usage:
  mrboard                          Launch TUI board (default)
  mrboard fetch [--debug <path>]   Fetch MRs and print as JSON

Config search path (first match wins):
  $MRBOARD_CONFIG
  $XDG_CONFIG_HOME/mrboard/mrboard.yaml  (default: ~/.config/mrboard/mrboard.yaml)
  ./mrboard.yaml

Environment:
  MRBOARD_CONFIG   Explicit config file path
  GITLAB_TOKEN     Override gitlab.token from config
  MRBOARD_TIMEOUT  HTTP timeout (default: 30s, e.g. "60s")
  MRBOARD_DEBUG    Write debug logs to this file path
`)
}

// runFetch fetches all MRs and prints them as a JSON array to stdout.
// Returns exit code: 0 on success or partial success, 1 if all sources fail.
func runFetch(args []string) int {
	debugPath := parseFetchDebugFlag(args)
	if debugPath == "" {
		debugPath = os.Getenv("MRBOARD_DEBUG")
	}
	logger := setupLogger(debugPath)

	timeout := loadTimeout()
	svc, err := app.New(timeout, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		return 1
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
		return 1
	}

	out := make([]mrJSON, 0, len(mrs))
	for _, mr := range mrs {
		out = append(out, toJSON(mr))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: encode JSON: %v\n", err)
		return 1
	}
	return 0
}

// runBoard loads config and launches the TUI.
func runBoard() int {
	svc, err := app.New(loadTimeout(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		return 1
	}

	st := config.LoadState()
	m := tui.New(svc.Config, svc.MRSource, st, Version)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		return 1
	}
	return 0
}

// parseFetchDebugFlag extracts the --debug <path> value from fetch subcommand args.
func parseFetchDebugFlag(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--debug" {
			return args[i+1]
		}
	}
	return ""
}

// setupLogger returns a logger that writes JSON debug logs to path,
// or a discard logger when path is empty.
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

// loadTimeout reads MRBOARD_TIMEOUT or returns the 30s default.
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

// mrJSON is the JSON representation of a domain.MergeRequest for the fetch subcommand.
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
	timeInPhase := domain.FormatDuration(now.Sub(mr.WaitingSince))
	timeOpen := domain.FormatDuration(now.Sub(mr.CreatedAt))

	return mrJSON{
		ID:             mr.ID,
		Title:          mr.Title,
		WebURL:         mr.WebURL,
		Phase:          phaseName(mr.Phase),
		Author:         mr.Author,
		ReviewerStates: reviewers,
		TimeInPhase:    timeInPhase,
		TimeOpen:       timeOpen,
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
