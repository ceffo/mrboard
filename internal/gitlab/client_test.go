package gitlab

import (
	"testing"

	"github.com/mrboard/mrboard/internal/config"
)

func TestNewClient_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		GitLab: config.GitLab{
			URL:   "https://gitlab.example.com",
			Token: "glpat-test",
		},
	}
	c, err := NewClient(cfg)
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
	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("NewClient() expected error for invalid URL, got nil")
	}
}
