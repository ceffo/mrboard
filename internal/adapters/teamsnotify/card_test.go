package teamsnotify

import (
	"strings"
	"testing"

	"github.com/ceffo/mrboard/internal/domain"
)

const approverUser = "doc"

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
	cfg := Config{UserMappings: map[string]string{"doc": "Emmett Brown"}}

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

func TestBuildCard_MentionsOnlyApprovers(t *testing.T) {
	card := buildCard(baseMR(), Config{})

	if card.MsTeams == nil || len(card.MsTeams.Entities) != 1 {
		t.Fatalf("expected exactly one mention entity, got %+v", card.MsTeams)
	}
	if got := card.MsTeams.Entities[0].Mentioned.Name; got != approverUser {
		t.Errorf("mentioned approver = %q, want %q", got, approverUser)
	}
}
