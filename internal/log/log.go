// Package log provides structured file logging and context-based logger propagation.
package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type contextKey struct{}

// Config controls log file location and verbosity.
type Config struct {
	Path  string // default: XDG data dir / mrboard / mrboard.log
	Level string // debug | info | warn | error (default: info)
}

// New opens (or creates) the log file and returns a JSON logger and a closer.
// The caller must close the closer on shutdown to flush buffered writes.
func New(cfg Config) (*slog.Logger, io.Closer, error) {
	path := cfg.Path
	if path == "" {
		path = defaultPath()
	}

	const dirPerm, filePerm = 0o700, 0o600
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return nil, nil, fmt.Errorf("log: create dir for %q: %w", path, err)
	}

	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePerm)
	if err != nil {
		return nil, nil, fmt.Errorf("log: open %q: %w", path, err)
	}

	level := parseLevel(cfg.Level)
	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	return slog.New(handler), f, nil
}

// WithLogger attaches l to ctx so all downstream code can retrieve it via FromContext.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext retrieves the logger stored by WithLogger.
// Returns a no-op logger if none is present — never returns nil.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func defaultPath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "mrboard.log"
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "mrboard", "mrboard.log")
}
