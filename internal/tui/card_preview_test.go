package tui

// Run with:   go test ./internal/tui/ -run TestCardPreview -v 2>/dev/null
// Pipe to a real terminal to see colours.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	lip "charm.land/lipgloss/v2"

	"github.com/ceffo/mrboard/internal/domain"
)

func TestCardPreview(t *testing.T) {
	if testing.Short() {
		t.Skip("preview only")
	}

	cardWidth := 32
	styles := NewStyles(LoadThemeByName("default"), true)
	styles.LifetimeWarn = 3 * 24 * time.Hour
	styles.LifetimeError = 5 * 24 * time.Hour

	now := time.Now()
	mr := domain.MergeRequest{
		IID:          1167,
		Title:        "feat(OD-2245): [GitOps migration] boris-pipeline-elset-service [AI-Assisted]",
		Phase:        domain.PhaseNeedsAuthorAction,
		CreatedAt:    now.Add(-5*24*time.Hour - time.Hour),
		WaitingSince: now.Add(-2 * time.Minute),
		Reviewers: []domain.ReviewerInfo{
			{Username: "bjean", Name: "Benoit Jean", State: domain.ReviewerApproved},
			{Username: "abarde", Name: "A Barde", State: domain.ReviewerCommented, WaitingSince: now.Add(-time.Minute)},
		},
	}
	mr.Author = "moncef"
	mr.AuthorName = "Moncef Naji"

	sep := func(label string) string {
		line := strings.Repeat("─", 50)
		return lip.NewStyle().Foreground(lip.Color("240")).Render("── " + label + " " + line)
	}

	c := newCardWidget(mr, styles, cardWidth)

	fmt.Println(sep("unfocused"))
	fmt.Println(c.render())

	c.SetFocused(true)
	fmt.Println(sep("focused"))
	fmt.Println(c.render())

	c.SetFocusInactive(true)
	fmt.Println(sep("focused inactive"))
	fmt.Println(c.render())
}
