package teamsnotify

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/domain"
)

const (
	approverUser        = "doc"
	approverDisplayName = "Emmett Brown"
)

func baseMR() domain.MergeRequest {
	return domain.MergeRequest{
		IID:         42,
		Title:       "Fix the flux capacitor",
		Author:      "mmcfly",
		ProjectPath: "group/delorean",
		WebURL:      "https://gitlab.example.com/group/delorean/-/merge_requests/42",
		Reviewers: []domain.ReviewerInfo{
			{Username: approverUser, IsApprover: true},
			{Username: "biff", IsApprover: false},
		},
	}
}

func TestBuildCard_FallbackText_FrontLoadsTitleAndApprovers(t *testing.T) {
	cfg := Config{UserMappings: map[string]string{approverUser: approverDisplayName}}

	card := buildCard(baseMR(), cfg)

	// The push-notification preview must be human-legible on its own: title,
	// then !IID + project, then the approver(s) being pinged.
	want := "Fix the flux capacitor · !42 delorean — 👌 Emmett Brown"
	assert.Equal(t, want, card.FallbackText)
}

func TestBuildCard_FallbackText_NoApprovers_OmitsPingSuffix(t *testing.T) {
	mr := baseMR()
	for i := range mr.Reviewers {
		mr.Reviewers[i].IsApprover = false
	}

	card := buildCard(mr, Config{})

	assert.NotContains(t, card.FallbackText, "👌", "FallbackText should omit approver suffix, got %q", card.FallbackText)
	want := "Fix the flux capacitor · !42 delorean"
	assert.Equal(t, want, card.FallbackText)
}

func TestBuildPayload_SummaryUsesEmailMentionTags(t *testing.T) {
	cfg := Config{
		UserMappings: map[string]string{approverUser: approverDisplayName},
		UserIDs:      map[string]string{approverUser: "doc@example.com"},
	}
	p := buildPayload(baseMR(), cfg)

	// Summary must be non-empty so PA step 1 has content to post.
	assert.NotEmpty(t, p.Summary, "Summary must not be empty")
	// Summary uses <at>email</at> for approvers with a configured UserID so
	// Teams resolves them as @mentions and fires push notifications.
	assert.Contains(t, p.Summary, "<at>doc@example.com</at>",
		"Summary should contain email mention tag, got %q", p.Summary)
	// FallbackText uses plain display names (for email/non-card clients) and
	// must NOT contain <at> tags.
	assert.NotContains(t, p.Card.FallbackText, "<at>", "FallbackText should use plain names, got %q", p.Card.FallbackText)
}

func TestBuildPayload_SummaryFallsBackToDisplayName(t *testing.T) {
	// No UserIDs configured — approver has no email mapping.
	cfg := Config{UserMappings: map[string]string{approverUser: approverDisplayName}}
	p := buildPayload(baseMR(), cfg)

	assert.NotContains(t, p.Summary, "<at>",
		"Summary should not contain mention tags when UserIDs is empty, got %q", p.Summary)
	assert.Contains(t, p.Summary, approverDisplayName, "Summary should contain display name fallback, got %q", p.Summary)
}

func TestBuildCard_MentionsOnlyApprovers(t *testing.T) {
	card := buildCard(baseMR(), Config{})

	require.NotNil(t, card.MsTeams, "expected exactly one mention entity, got %+v", card.MsTeams)
	require.Len(t, card.MsTeams.Entities, 1, "expected exactly one mention entity, got %+v", card.MsTeams)
	got := card.MsTeams.Entities[0].Mentioned.Name
	assert.Equal(t, approverUser, got)
}
