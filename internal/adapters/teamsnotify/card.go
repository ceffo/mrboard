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

// payload is the envelope the Power Automate flow extracts via triggerBody()?['card'].
type payload struct {
	Card adaptiveCard `json:"card"`
}

type adaptiveCard struct {
	Type    string          `json:"type"`
	Schema  string          `json:"$schema"`
	Version string          `json:"version"`
	Body    []any           `json:"body"`
	Actions []openURLAction `json:"actions,omitempty"`
	MsTeams *msTeamsExt     `json:"msteams,omitempty"`
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

	body := []any{
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
	for _, r := range mr.Reviewers {
		if !r.IsApprover {
			continue
		}
		displayName := r.Username
		if name, ok := cfg.UserMappings[r.Username]; ok {
			displayName = name
		}
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

	return adaptiveCard{
		Type:    typeAdaptive,
		Schema:  schemaAdaptive,
		Version: versionAdaptive,
		Body:    body,
		Actions: []openURLAction{
			{Type: typeActionOpen, Title: "Open MR", URL: mr.WebURL},
		},
		MsTeams: msTeams,
	}
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
