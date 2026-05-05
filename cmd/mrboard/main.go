// Package main is the entry point for mrboard.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/mrboard/mrboard/internal/config"
	"github.com/mrboard/mrboard/internal/gitlab"
	"github.com/mrboard/mrboard/internal/tui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		os.Exit(1)
	}

	client, err := gitlab.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		os.Exit(1)
	}

	m := tui.New(cfg, client)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "mrboard: %v\n", err)
		os.Exit(1)
	}
}
