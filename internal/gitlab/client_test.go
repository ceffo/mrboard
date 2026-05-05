package gitlab

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/mrboard/mrboard/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewClient_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		GitLab: config.GitLab{
			URL:   "https://gitlab.example.com",
			Token: "glpat-test",
		},
	}
	c, err := NewClient(cfg, 30*time.Second, discardLogger())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if c == nil {
		t.Fatal("NewClient() returned nil client")
	}
}

func TestNewClient_InvalidURL(t *testing.T) {
	cfg := &config.Config{
		GitLab: config.GitLab{
			URL:   "://invalid-url",
			Token: "glpat-test",
		},
	}
	_, err := NewClient(cfg, 30*time.Second, discardLogger())
	if err == nil {
		t.Fatal("NewClient() expected error for invalid URL, got nil")
	}
}
