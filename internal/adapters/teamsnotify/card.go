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
	projectName := mr.ProjectPath
	if i := strings.LastIndex(mr.ProjectPath, "/"); i >= 0 {
		projectName = mr.ProjectPath[i+1:]
	}

	// Use assignee as the primary person; fall back to author when unassigned.
	assigneeKey := mr.Assignee
	if assigneeKey == "" {
		assigneeKey = mr.Author
	}
	authorName := mr.DisplayAssignee()
	if name, ok := cfg.UserMappings[assigneeKey]; ok {
		authorName = name
	}
	body := []any{
		textBlock{
			Type:  typeTextBlock,
			Text:  authorName,
			Size:  "medium",
			Color: "accent",
		},
		textBlock{
			Type:   typeTextBlock,
			Text:   mr.Title,
			Weight: "bolder",
			Size:   "large",
			Wrap:   true,
		},
		textBlock{
			Type: typeTextBlock,
			Text: fmt.Sprintf("**!%d** `%s`", mr.IID, projectName),
			Wrap: true,
		},
	}

	var entities []mentionEntity
	var mentionParts []string
	var approverNames []string
	for _, r := range mr.Reviewers {
		if !r.IsApprover {
			continue
		}
		displayName := r.Username
		if name, ok := cfg.UserMappings[r.Username]; ok {
			displayName = name
		}
		approverNames = append(approverNames, displayName)
		tag := fmt.Sprintf("<at>%s</at>", displayName)
		mentionParts = append(mentionParts, tag)

		id := displayName
		if uid, ok := cfg.UserIDs[r.Username]; ok {
			id = uid
		}
		entities = append(entities, mentionEntity{
			Type:      "mention",
			Text:      tag,
			Mentioned: mentionedUser{ID: id, Name: displayName},
		})
	}

	if len(mentionParts) > 0 {
		body = append(body,
			textBlock{Type: typeTextBlock, Weight: "bolder", Text: "👌 Approvers"},
			textBlock{
				Type: typeTextBlock,
				Text: strings.Join(mentionParts, " · "),
				Wrap: true,
			},
		)
	}

	var msTeams *msTeamsExt
	if len(entities) > 0 {
		msTeams = &msTeamsExt{Entities: entities}
	}

	actions := []openURLAction{
		{Type: typeActionOpen, Title: "Open MR", URL: mr.WebURL},
	}
	if jiraURL := domain.JiraIssueURL(cfg.JiraBaseURL, domain.ExtractJiraID(mr.Title)); jiraURL != "" {
		actions = append(actions, openURLAction{Type: typeActionOpen, Title: "Open JIRA", URL: jiraURL})
	}

	summary := fallbackSummary(mr, projectName, approverNames)
	return adaptiveCard{
		Type:         typeAdaptive,
		Schema:       schemaAdaptive,
		Version:      versionAdaptive,
		Body:         body,
		Actions:      actions,
		MsTeams:      msTeams,
		FallbackText: summary,
	}
}

// buildPayload wraps the card in the full webhook payload.
func buildPayload(mr domain.MergeRequest, cfg Config) payload {
	card := buildCard(mr, cfg)
	projectName := mr.ProjectPath
	if i := strings.LastIndex(mr.ProjectPath, "/"); i >= 0 {
		projectName = mr.ProjectPath[i+1:]
	}
	return payload{
		Card:    card,
		Summary: mentionSummary(mr, projectName, cfg),
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
		if email, ok := cfg.UserIDs[r.Username]; ok {
			parts = append(parts, fmt.Sprintf("<at>%s</at>", email))
		} else {
			name := r.Username
			if mapped, ok := cfg.UserMappings[r.Username]; ok {
				name = mapped
			}
			parts = append(parts, name)
		}
	}
	if len(parts) > 0 {
		return base + " — 👌 " + strings.Join(parts, " ")
	}
	return base
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
