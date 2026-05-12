package app

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// DefaultTimeout is the HTTP deadline used when MRBOARD_TIMEOUT is not set.
const DefaultTimeout = 30 * time.Second

const logFileMode = 0o600

// TimeoutFromEnv reads MRBOARD_TIMEOUT or returns DefaultTimeout.
func TimeoutFromEnv() time.Duration {
	if v := os.Getenv("MRBOARD_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
		fmt.Fprintf(os.Stderr, "mrboard: invalid MRBOARD_TIMEOUT %q, using default\n", v)
	}
	return DefaultTimeout
}

// LoggerFromPath returns a JSON debug logger writing to path,
// or a discard logger when path is empty.
func LoggerFromPath(path string) *slog.Logger {
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
