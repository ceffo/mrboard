// Package teamsnotify implements domain.Notifier for Microsoft Teams via a
// Power Automate webhook. It posts an Adaptive Card to a configured flow URL.
package teamsnotify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/ceffo/mrboard/internal/domain"
)

// Config holds the credentials and user mappings for Teams notification delivery.
type Config struct {
	WebhookURL   string
	UserMappings map[string]string // gitlab username → Teams display name
	UserIDs      map[string]string // gitlab username → Teams UPN/email for @mention pings
	JiraBaseURL  string            // e.g. "https://northstar.atlassian.net"; empty disables JIRA action
}

// TeamsNotifier delivers MR notifications to a Microsoft Teams channel.
// It satisfies domain.Notifier.
type TeamsNotifier struct {
	cfg    Config
	logger *slog.Logger
}

// New returns a TeamsNotifier wired to the given config.
func New(cfg Config, logger *slog.Logger) *TeamsNotifier {
	return &TeamsNotifier{cfg: cfg, logger: logger}
}

// Notify builds an Adaptive Card for mr and delivers it via the configured
// Power Automate webhook. The flow must extract the card via
// triggerBody()?['card'] and post it with "Post adaptive card in a chat or channel".
func (t *TeamsNotifier) Notify(ctx context.Context, mr domain.MergeRequest) error {
	t.logger.Info("teamsnotify: building card",
		"mr_iid", mr.IID,
		"mr_title", mr.Title,
		"approvers", countApprovers(mr),
	)

	p, err := json.Marshal(payload{Card: buildCard(mr, t.cfg)})
	if err != nil {
		return fmt.Errorf("teamsnotify: marshal: %w", err)
	}

	t.logger.Debug("teamsnotify: posting card", "payload_bytes", len(p))

	if err := post(ctx, t.cfg.WebhookURL, p, t.logger); err != nil {
		t.logger.Error("teamsnotify: delivery failed", "mr_iid", mr.IID, "err", err)
		return err
	}

	t.logger.Info("teamsnotify: card delivered", "mr_iid", mr.IID)
	return nil
}
