// Package main is the entry point for mrboard.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	mrboardcmd "github.com/ceffo/mrboard/internal/cmd/mrboard"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := mrboardcmd.Execute(ctx); err != nil {
		os.Exit(1)
	}
}
