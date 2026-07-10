package gitlab

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	require.NoError(t, err, "NewClient() error")
	require.NotNil(t, c, "NewClient() returned nil client")
}
