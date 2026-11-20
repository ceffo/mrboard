package teamsnotify

import (
	"fmt"
	"strings"

	"github.com/ceffo/mrboard/internal/domain"
)

const (
	typeTextBlock   = "TextBlock"
	typeActionOpen  = "Action.OpenUrl"
	typeAdaptive    = "AdaptiveCard"
	schemaAdaptive  = "http://adaptivecards.io/schemas/adaptive-card.json"
	versionAdaptive = "1.2"
	weightBolder    = "bolder"
)

// — Adaptive Card types ------------------------------------------------------

// payload is the envelope the Power Automate flow receives.
//
// Two-step flow:
//  1. "Post a message in a chat or channel" (Flow bot) with body = summary
//     → fires the OS push-notification and @mention pings (summary contains
//     <at>email</at> tags that Teams resolves against the tenant directory)
//  2. "Update an adaptive card in a chat or channel" with card = string(card)
//     → silently replaces the text with the rich adaptive card
type payload struct {
	Card    adaptiveCard `json:"card"`
	Summary string       `json:"summary,omitempty"`
}

type adaptiveCard struct {
	Type    string          `json:"type"`
	Schema  string          `json:"$schema"`
	Version string          `json:"version"`
	Body    []any           `json:"body"`
	Actions []openURLAction `json:"actions,omitempty"`
	MsTeams *msTeamsExt     `json:"msteams,omitempty"`
	// FallbackText drives the OS/Teams push-notification preview (the toast
	// text). Without it Teams shows a generic "Card"; the connector's bot name
	// ("Workflows") is fixed by Power Automate and cannot be set here.
	FallbackText string `json:"fallbackText,omitempty"`
}

type msTeamsExt struct {
	Entities []mentionEntity `json:"entities"`
}

type mentionEntity struct {
	Type      string        `json:"type"`
	Text      string        `json:"text"`
	Mentioned mentionedUser `json:"mentioned"`
}

type mentionedUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type textBlock struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Wrap   bool   `json:"wrap,omitempty"`
	Weight string `json:"weight,omitempty"`
	Size   string `json:"size,omitempty"`
	Color  string `json:"color,omitempty"`
}

type openURLAction struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// — Card builder -------------------------------------------------------------

func buildCard(mr domain.MergeRequest, cfg Config) adaptiveCard {
	projectName := projectNameOf(mr)
	body := headerBlocks(mr, cfg, projectName)

	var entities []mentionEntity
	var mentionParts []string
	var approverNames []string
	for _, r := range mr.Reviewers {
		if !r.IsApprover {
			continue
		}
		tag, entity := mentionOf(r.Username, cfg)
		approverNames = append(approverNames, entity.Mentioned.Name)
		mentionParts = append(mentionParts, tag)
		entities = append(entities, entity)
	}

	if len(mentionParts) > 0 {
		body = append(body,
			textBlock{Type: typeTextBlock, Weight: weightBolder, Text: "👌 Approvers"},
			textBlock{
				Type: typeTextBlock,
				Text: strings.Join(mentionParts, " · "),
				Wrap: true,
			},
		)
	}

	summary := fallbackSummary(mr, projectName, approverNames)
	return adaptiveCard{
		Type:         typeAdaptive,
		Schema:       schemaAdaptive,
		Version:      versionAdaptive,
		Body:         body,
		Actions:      cardActions(mr, cfg),
		MsTeams:      msTeamsOf(entities),
		FallbackText: summary,
	}
}

// projectNameOf returns the last segment of the MR's project path.
func projectNameOf(mr domain.MergeRequest) string {
	if i := strings.LastIndex(mr.ProjectPath, "/"); i >= 0 {
		return mr.ProjectPath[i+1:]
	}
	return mr.ProjectPath
}

// headerBlocks renders the person the MR belongs to, its title and its
// reference. The person is the assignee, or the author when unassigned.
func headerBlocks(mr domain.MergeRequest, cfg Config, projectName string) []any {
	assigneeKey := mr.Assignee
	if assigneeKey == "" {
		assigneeKey = mr.Author
	}
	authorName := mr.DisplayAssignee()
	if name, ok := cfg.UserMappings[assigneeKey]; ok {
		authorName = name
	}
	return []any{
		textBlock{
			Type:  typeTextBlock,
			Text:  authorName,
			Size:  "medium",
			Color: "accent",
		},
		textBlock{
			Type:   typeTextBlock,
			Text:   mr.Title,
			Weight: weightBolder,
			Size:   "large",
			Wrap:   true,
		},
		textBlock{
			Type: typeTextBlock,
			Text: fmt.Sprintf("**!%d** `%s`", mr.IID, projectName),
			Wrap: true,
		},
	}
}

func cardActions(mr domain.MergeRequest, cfg Config) []openURLAction {
	actions := []openURLAction{
		{Type: typeActionOpen, Title: "Open MR", URL: mr.WebURL},
	}
	if ticketURL := domain.JiraIssueURL(cfg.TicketBaseURL, cfg.KeyMatcher.ExtractFromTitle(mr.Title)); ticketURL != "" {
		actions = append(actions, openURLAction{Type: typeActionOpen, Title: "Open JIRA", URL: ticketURL})
	}
	return actions
}

// displayNameOf returns the Teams display name mapped to a GitLab username,
// or the username itself.
func displayNameOf(username string, cfg Config) string {
	if name, ok := cfg.UserMappings[username]; ok {
		return name
	}
	return username
}

// mentionOf returns the card's <at> tag for a user and the entity that makes
// Teams resolve it.
func mentionOf(username string, cfg Config) (string, mentionEntity) {
	displayName := displayNameOf(username, cfg)
	tag := fmt.Sprintf("<at>%s</at>", displayName)
	id := displayName
	if uid, ok := cfg.UserIDs[username]; ok {
		id = uid
	}
	return tag, mentionEntity{Type: "mention", Text: tag, Mentioned: mentionedUser{ID: id, Name: displayName}}
}

func msTeamsOf(entities []mentionEntity) *msTeamsExt {
	if len(entities) == 0 {
		return nil
	}
	return &msTeamsExt{Entities: entities}
}

// buildPayload wraps the card in the full webhook payload.
func buildPayload(mr domain.MergeRequest, cfg Config) payload {
	return payload{
		Card:    buildCard(mr, cfg),
		Summary: mentionSummary(mr, projectNameOf(mr), cfg),
	}
}

// mentionSummary builds the one-line text posted as step 1 of the PA flow.
// Approvers with a configured UserID are wrapped in <at>email</at> so Teams
// resolves them against the tenant directory and fires @mention notifications.
// Approvers without a UserID fall back to plain display names (readable but
// won't ping).
func mentionSummary(mr domain.MergeRequest, projectName string, cfg Config) string {
	base := fmt.Sprintf("%s · !%d %s", mr.Title, mr.IID, projectName)
	var parts []string
	for _, r := range mr.Reviewers {
		if !r.IsApprover {
			continue
		}
		parts = append(parts, summaryMentionOf(r.Username, cfg))
	}
	if len(parts) > 0 {
		return base + " — 👌 " + strings.Join(parts, " ")
	}
	return base
}

// summaryMentionOf returns a user as the step-1 summary names them: an
// <at>email</at> tag that pings when a UserID is configured, else the display
// name.
func summaryMentionOf(username string, cfg Config) string {
	if email, ok := cfg.UserIDs[username]; ok {
		return fmt.Sprintf("<at>%s</at>", email)
	}
	return displayNameOf(username, cfg)
}

// fallbackSummary builds the one-line push-notification preview shown by the
// OS/Teams toast. It front-loads the MR title (what a human recognises at a
// glance), then the !IID and project, then the approvers being pinged.
func fallbackSummary(mr domain.MergeRequest, projectName string, approvers []string) string {
	summary := fmt.Sprintf("%s · !%d %s", mr.Title, mr.IID, projectName)
	if len(approvers) > 0 {
		summary += " — 👌 " + strings.Join(approvers, ", ")
	}
	return summary
}

func countApprovers(mr domain.MergeRequest) int {
	n := 0
	for _, r := range mr.Reviewers {
		if r.IsApprover {
			n++
		}
	}
	return n
}
