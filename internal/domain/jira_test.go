package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	jiraKeyUppercase = "OD-3345"
	jiraKeyLowercase = "od-3345"
)

func TestTicketKeyMatcher_ExtractFromTitle(t *testing.T) {
	tests := []struct {
		name            string
		title           string
		caseInsensitive bool
		want            string
	}{
		{name: "case-sensitive recognizes an uppercase key", title: "feat(OD-3345): x", want: jiraKeyUppercase},
		{name: "case-sensitive does not recognize a lowercase key", title: "fix(od-3345): x", want: ""},
		{
			name:  "case-insensitive upper-cases a lowercase key",
			title: "fix(od-3345): x", caseInsensitive: true, want: jiraKeyUppercase,
		},
		{
			name:  "case-insensitive leaves an uppercase key unchanged",
			title: "feat(OD-3345): x", caseInsensitive: true, want: jiraKeyUppercase,
		},
		{name: "no key found returns empty regardless of mode", title: "chore: bump deps", caseInsensitive: true, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewTicketKeyMatcher(tc.caseInsensitive)
			assert.Equal(t, tc.want, m.ExtractFromTitle(tc.title), "ExtractFromTitle(%q)", tc.title)
		})
	}
}

func TestTicketKeyMatcher_Normalize(t *testing.T) {
	assert.Equal(t, jiraKeyUppercase, NewTicketKeyMatcher(true).Normalize(jiraKeyLowercase),
		"case-insensitive matcher should upper-case an already-known key")
	assert.Equal(t, jiraKeyLowercase, NewTicketKeyMatcher(false).Normalize(jiraKeyLowercase),
		"case-sensitive matcher should leave an already-known key unchanged")
}

func TestTicketKeyMatcher_ZeroValueIsCaseSensitive(t *testing.T) {
	var m TicketKeyMatcher
	assert.Equal(t, jiraKeyUppercase, m.ExtractFromTitle("feat(OD-3345): x"), "zero value should recognize an exact key")
	assert.Empty(t, m.ExtractFromTitle("fix(od-3345): x"), "zero value should not recognize a lowercase key")
}

func TestHasJiraLink(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        bool
	}{
		{name: "marker present", description: "existing body\n---\n🎫 [OD-1](url) <!-- mrboard -->", want: true},
		{name: "marker absent", description: "some description", want: false},
		{name: "empty description", description: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, HasJiraLink(tc.description), "HasJiraLink(%q)", tc.description)
		})
	}
}

func TestAppendJiraLink(t *testing.T) {
	const issueKey = "OD-123"
	const instanceURL = "https://jira.example.com"
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "empty description",
			description: "",
			want: "---\n<!-- mrboard: automated back-link below — do not edit or remove -->\n" +
				"🎫 [OD-123](https://jira.example.com/browse/OD-123) <!-- mrboard -->",
		},
		{
			name:        "non-empty description",
			description: "some body",
			want: "some body\n\n---\n<!-- mrboard: automated back-link below — do not edit or remove -->\n" +
				"🎫 [OD-123](https://jira.example.com/browse/OD-123) <!-- mrboard -->",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AppendJiraLink(tc.description, instanceURL, issueKey)
			assert.Equal(t, tc.want, got, "AppendJiraLink()")
			assert.True(t, HasJiraLink(got), "AppendJiraLink() result does not satisfy HasJiraLink: %q", got)
		})
	}
}
