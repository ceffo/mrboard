package teamsnotify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ceffo/mrboard/internal/domain"
)

// NotifyApproverChange delivers a card naming who became and who stopped
// being a required approver of mr, and who changed the rule. Only the users
// who became required approvers are mentioned: they are the ones asked to act.
func (t *TeamsNotifier) NotifyApproverChange(
	ctx context.Context, mr domain.MergeRequest, change domain.ApproverChange,
) error {
	t.logger.Info("teamsnotify: building approver change card",
		"mr_iid", mr.IID, "gained", len(change.Gained), "lost", len(change.Lost))

	p, err := json.Marshal(buildApproverChangePayload(mr, change, t.cfg))
	if err != nil {
		return fmt.Errorf("teamsnotify: marshal: %w", err)
	}
	if err := post(ctx, t.cfg.WebhookURL, p, t.logger); err != nil {
		t.logger.Error("teamsnotify: delivery failed", "mr_iid", mr.IID, "err", err)
		return err
	}
	t.logger.Info("teamsnotify: approver change card delivered", "mr_iid", mr.IID)
	return nil
}

const (
	gainedHeading = "👌 Now required to approve"
	lostHeading   = "No longer required to approve"
)

// sourceText is the card's line saying who changed the approval rule.
func sourceText(source domain.ApproverChangeSource) string {
	if source == domain.ApproverChangedByMrboard {
		return "Approval rule changed through mrboard"
	}
	return "Approval rule changed outside mrboard (a GitLab rule or an edit by hand)"
}

func buildApproverChangeCard(mr domain.MergeRequest, change domain.ApproverChange, cfg Config) adaptiveCard {
	projectName := projectNameOf(mr)
	body := headerBlocks(mr, cfg, projectName)

	var entities []mentionEntity
	var tags []string
	for _, u := range change.Gained {
		tag, entity := mentionOf(u, cfg)
		tags = append(tags, tag)
		entities = append(entities, entity)
	}
	if len(tags) > 0 {
		body = append(body,
			textBlock{Type: typeTextBlock, Weight: weightBolder, Text: gainedHeading},
			textBlock{Type: typeTextBlock, Text: strings.Join(tags, " · "), Wrap: true},
		)
	}
	lost := displayNamesOf(change.Lost, cfg)
	if len(lost) > 0 {
		body = append(body,
			textBlock{Type: typeTextBlock, Weight: weightBolder, Text: lostHeading},
			textBlock{Type: typeTextBlock, Text: strings.Join(lost, " · "), Wrap: true},
		)
	}
	body = append(body, textBlock{
		Type: typeTextBlock, Text: sourceText(change.Source), Wrap: true, Size: "small",
	})

	return adaptiveCard{
		Type:         typeAdaptive,
		Schema:       schemaAdaptive,
		Version:      versionAdaptive,
		Body:         body,
		Actions:      cardActions(mr, cfg),
		MsTeams:      msTeamsOf(entities),
		FallbackText: approverChangeSummary(mr, projectName, displayNamesOf(change.Gained, cfg), lost),
	}
}

func buildApproverChangePayload(mr domain.MergeRequest, change domain.ApproverChange, cfg Config) payload {
	gained := make([]string, 0, len(change.Gained))
	for _, u := range change.Gained {
		gained = append(gained, summaryMentionOf(u, cfg))
	}
	return payload{
		Card:    buildApproverChangeCard(mr, change, cfg),
		Summary: approverChangeSummary(mr, projectNameOf(mr), gained, displayNamesOf(change.Lost, cfg)),
	}
}

// approverChangeSummary is the one-line text of an approver change: the MR,
// then who is now required, then who no longer is.
func approverChangeSummary(mr domain.MergeRequest, projectName string, gained, lost []string) string {
	summary := fmt.Sprintf("%s · !%d %s", mr.Title, mr.IID, projectName)
	if len(gained) > 0 {
		summary += " — 👌 " + strings.Join(gained, ", ")
	}
	if len(lost) > 0 {
		summary += " — no longer required: " + strings.Join(lost, ", ")
	}
	return summary
}

func displayNamesOf(usernames []string, cfg Config) []string {
	names := make([]string, 0, len(usernames))
	for _, u := range usernames {
		names = append(names, displayNameOf(u, cfg))
	}
	return names
}
