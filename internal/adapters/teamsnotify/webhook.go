package teamsnotify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

const (
	httpSuccessMax = 300
	bodyReadLimit  = 512
)

// post delivers a JSON payload to a webhook URL via HTTP POST.
// It returns an error if the response status is outside the 2xx range.
func post(ctx context.Context, url string, payload []byte, logger *slog.Logger) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("teamsnotify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("teamsnotify: post: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, bodyReadLimit)) //nolint:errcheck
	if resp.StatusCode >= httpSuccessMax {
		logger.Error("teamsnotify: webhook rejected",
			"status", resp.StatusCode,
			"body", strings.TrimSpace(string(body)),
		)
		return fmt.Errorf("teamsnotify: webhook status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}
