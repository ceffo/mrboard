package tui

import (
	"testing"
)

// Test-local string constants to avoid goconst lint violations.
const (
	testBugTitle  = "Bug"
	testTechDebt  = "Tech Debt"
	testUnknownFB = "Incident" // unrecognized type → falls back to unknown icon
)

func TestIssueTypeIconResolver_Defaults(t *testing.T) {
	r := NewIssueTypeIconResolver(nil)

	cases := []struct {
		issueType string
		want      string
	}{
		{testBugTitle, "🐛"},
		{issueTypeBug, "🐛"}, // lowercase key
		{"BUG", "🐛"},        // upper-case
		{"Story", "🔖"},
		{issueTypeStory, "🔖"},
		{"Task", "📝"},
		{issueTypeTask, "📝"},
		{"Epic", "⚡"},
		{issueTypeEpic, "⚡"},
		{"Subtask", "📎"},
		{issueTypeSubtask, "📎"},
		{testUnknownFB, unknownIssueTypeIcon},
		{"", unknownIssueTypeIcon},
		{testTechDebt, unknownIssueTypeIcon},
	}

	for _, tc := range cases {
		got := r.Resolve(tc.issueType)
		if got != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.issueType, got, tc.want)
		}
	}
}

func TestIssueTypeIconResolver_Overrides(t *testing.T) {
	overrides := map[string]string{
		testTechDebt: "🔧",
		testBugTitle: "🔴", // override a default
	}
	r := NewIssueTypeIconResolver(overrides)

	cases := []struct {
		issueType string
		want      string
	}{
		{testTechDebt, "🔧"},
		{"tech debt", "🔧"},  // case-insensitive
		{testBugTitle, "🔴"}, // override wins over default
		{issueTypeBug, "🔴"}, // same override, lowercase input
		{"Story", "🔖"},      // unchanged default
		{testUnknownFB, unknownIssueTypeIcon},
		{"", unknownIssueTypeIcon},
	}

	for _, tc := range cases {
		got := r.Resolve(tc.issueType)
		if got != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.issueType, got, tc.want)
		}
	}
}
