package teamsnotify

import (
	"strings"
	"testing"

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
	if card.FallbackText != want {
		t.Errorf("FallbackText = %q, want %q", card.FallbackText, want)
	}
}

func TestBuildCard_FallbackText_NoApprovers_OmitsPingSuffix(t *testing.T) {
	mr := baseMR()
	for i := range mr.Reviewers {
		mr.Reviewers[i].IsApprover = false
	}

	card := buildCard(mr, Config{})

	if strings.Contains(card.FallbackText, "👌") {
		t.Errorf("FallbackText should omit approver suffix, got %q", card.FallbackText)
	}
	want := "Fix the flux capacitor · !42 delorean"
	if card.FallbackText != want {
		t.Errorf("FallbackText = %q, want %q", card.FallbackText, want)
	}
}

func TestBuildPayload_SummaryUsesEmailMentionTags(t *testing.T) {
	cfg := Config{
		UserMappings: map[string]string{approverUser: approverDisplayName},
		UserIDs:      map[string]string{approverUser: "doc@example.com"},
	}
	p := buildPayload(baseMR(), cfg)

	// Summary must be non-empty so PA step 1 has content to post.
	if p.Summary == "" {
		t.Fatal("Summary must not be empty")
	}
	// Summary uses <at>email</at> for approvers with a configured UserID so
	// Teams resolves them as @mentions and fires push notifications.
	if !strings.Contains(p.Summary, "<at>doc@example.com</at>") {
		t.Errorf("Summary should contain email mention tag, got %q", p.Summary)
	}
	// FallbackText uses plain display names (for email/non-card clients) and
	// must NOT contain <at> tags.
	if strings.Contains(p.Card.FallbackText, "<at>") {
		t.Errorf("FallbackText should use plain names, got %q", p.Card.FallbackText)
	}
}

func TestBuildPayload_SummaryFallsBackToDisplayName(t *testing.T) {
	// No UserIDs configured — approver has no email mapping.
	cfg := Config{UserMappings: map[string]string{approverUser: approverDisplayName}}
	p := buildPayload(baseMR(), cfg)

	if strings.Contains(p.Summary, "<at>") {
		t.Errorf("Summary should not contain mention tags when UserIDs is empty, got %q", p.Summary)
	}
	if !strings.Contains(p.Summary, approverDisplayName) {
		t.Errorf("Summary should contain display name fallback, got %q", p.Summary)
	}
}

func TestBuildCard_MentionsOnlyApprovers(t *testing.T) {
	card := buildCard(baseMR(), Config{})

	if card.MsTeams == nil || len(card.MsTeams.Entities) != 1 {
		t.Fatalf("expected exactly one mention entity, got %+v", card.MsTeams)
	}
	if got := card.MsTeams.Entities[0].Mentioned.Name; got != approverUser {
		t.Errorf("mentioned approver = %q, want %q", got, approverUser)
	}
}
