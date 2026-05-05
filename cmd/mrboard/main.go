// Package main is the entry point for mrboard.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mrboard/mrboard/internal/config"
	"github.com/mrboard/mrboard/internal/domain"
	"github.com/mrboard/mrboard/internal/gitlab"
	"github.com/mrboard/mrboard/internal/tui"
)

const defaultTimeout = 30 * time.Second

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "fetch":
		os.Exit(runFetch(os.Args[2:]))
	case "run":
		fmt.Fprintln(os.Stderr, "mrboard run: not implemented")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "mrboard: unknown subcommand %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`mrboard — GitLab MR review board

Usage:
  mrboard fetch [--debug <path>]   Fetch MRs and print as JSON
  mrboard run                      Launch TUI board (not implemented)

Environment:
  MRBOARD_CONFIG   Path to config file (default: mrboard.toml)
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

	cfg, cfgPath, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		return 1
	}
	logger.Debug("config loaded", "path", cfgPath, "sources", len(cfg.Sources))

	timeout := loadTimeout()
	client, err := gitlab.NewClient(cfg, timeout, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		return 1
	}

	mrs, errs := gitlab.FetchAll(client, cfg)

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
// or a no-op discard logger when path is empty.
func setupLogger(path string) *slog.Logger {
	if path == "" {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: open debug log %q: %v\n", path, err)
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// loadConfig reads the config file and returns the config and the resolved path.
func loadConfig() (*config.Config, string, error) {
	path := "mrboard.toml"
	if v := os.Getenv("MRBOARD_CONFIG"); v != "" {
		path = v
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, "", err
	}
	return cfg, path, nil
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
	ID            int            `json:"id"`
	Title         string         `json:"title"`
	WebURL        string         `json:"web_url"`
	Phase         string         `json:"phase"`
	Author        string         `json:"author"`
	ReviewerStates []reviewerJSON `json:"reviewer_states"`
	TimeInPhase   string         `json:"time_in_phase"`
	TimeOpen      string         `json:"time_open"`
	RoundTrips    int            `json:"round_trips"`
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

// runBoard launches the TUI (entry point kept for future use by the run subcommand).
func runBoard() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		return 1
	}

	timeout := loadTimeout()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client, err := gitlab.NewClient(cfg, timeout, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		return 1
	}

	m := tui.New(cfg, client)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		return 1
	}
	return 0
}
