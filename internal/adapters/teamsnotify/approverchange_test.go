package teamsnotify

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

const lostUser = "biff"

func cardTexts(card adaptiveCard) []string {
	var texts []string
	for _, b := range card.Body {
		if tb, ok := b.(textBlock); ok {
			texts = append(texts, tb.Text)
		}
	}
	return texts
}

func TestBuildApproverChangeCard_MentionsOnlyWhoGained(t *testing.T) {
	cfg := Config{
		UserMappings: map[string]string{approverUser: approverDisplayName},
		UserIDs:      map[string]string{approverUser: approverEmail},
	}
	change := domain.ApproverChange{
		Gained: []string{approverUser}, Lost: []string{lostUser}, Source: domain.ApproverChangedElsewhere,
	}

	card := buildApproverChangeCard(baseMR(), change, cfg)

	texts := cardTexts(card)
	assert.Contains(t, texts, gainedHeading)
	assert.Contains(t, texts, "<at>Emmett Brown</at>")
	assert.Contains(t, texts, lostHeading)
	assert.Contains(t, texts, lostUser, "who lost the status is named, not pinged")
	assert.Contains(t, texts, sourceText(domain.ApproverChangedElsewhere))
	require.NotNil(t, card.MsTeams)
	require.Len(t, card.MsTeams.Entities, 1)
	assert.Equal(t, approverEmail, card.MsTeams.Entities[0].Mentioned.ID)
	assert.Equal(t, "Fix the flux capacitor · !42 delorean — 👌 Emmett Brown — no longer required: biff",
		card.FallbackText)
}

func TestBuildApproverChangeCard_LossOnly_HasNoMentions(t *testing.T) {
	change := domain.ApproverChange{Lost: []string{approverUser}, Source: domain.ApproverChangedByMrboard}

	card := buildApproverChangeCard(baseMR(), change, Config{})

	texts := cardTexts(card)
	assert.NotContains(t, texts, gainedHeading)
	assert.Contains(t, texts, approverUser)
	assert.Contains(t, texts, sourceText(domain.ApproverChangedByMrboard))
	assert.Nil(t, card.MsTeams)
}

func TestBuildApproverChangePayload_LossOnly_SummaryNamesNoOneAsGained(t *testing.T) {
	cfg := Config{UserIDs: map[string]string{approverUser: approverEmail}}
	change := domain.ApproverChange{Lost: []string{approverUser}, Source: domain.ApproverChangedByMrboard}

	p := buildApproverChangePayload(baseMR(), change, cfg)

	assert.Equal(t, "Fix the flux capacitor · !42 delorean — no longer required: doc", p.Summary)
	assert.NotContains(t, p.Summary, "<at>", "who lost the status is not pinged")
}

func TestSourceText_TellsTheSourcesApart(t *testing.T) {
	assert.Contains(t, sourceText(domain.ApproverChangedByMrboard), "through mrboard")
	assert.Contains(t, sourceText(domain.ApproverChangedElsewhere), "outside mrboard")
}

func TestBuildApproverChangePayload_SummaryPingsWhoGained(t *testing.T) {
	cfg := Config{UserIDs: map[string]string{approverUser: approverEmail}}
	change := domain.ApproverChange{Gained: []string{approverUser}, Lost: []string{lostUser}}

	p := buildApproverChangePayload(baseMR(), change, cfg)

	assert.Equal(t, "Fix the flux capacitor · !42 delorean — 👌 <at>doc@example.com</at> — no longer required: biff",
		p.Summary)
}
