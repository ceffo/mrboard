package gitlab

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewClient_ValidConfig(t *testing.T) {
	cfg := Config{
		URL:     "https://gitlab.example.com",
		Token:   "glpat-test",
		Timeout: 30 * time.Second,
	}
	c, err := NewClient(cfg, discardLogger())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if c == nil {
		t.Fatal("NewClient() returned nil client")
	}
}
