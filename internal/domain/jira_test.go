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

func TestExtractLinkedTicketKey(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantKey     string
		wantOK      bool
	}{
		{
			name: "current footer format",
			description: "body\n\n---\n<!-- mrboard: automated back-link below — do not edit or remove -->\n" +
				"🎫 [OD-1](url) <!-- mrboard -->",
			wantKey: "OD-1", wantOK: true,
		},
		{
			name:        "pre-fix footer format (single newline, no agent notice)",
			description: "existing body\n---\n🎫 [OD-1](url) <!-- mrboard -->",
			wantKey:     "OD-1", wantOK: true,
		},
		{name: "marker absent", description: "some description", wantOK: false},
		{name: "empty description", description: "", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, ok := ExtractLinkedTicketKey(tc.description)
			assert.Equal(t, tc.wantOK, ok, "ExtractLinkedTicketKey(%q) ok", tc.description)
			assert.Equal(t, tc.wantKey, key, "ExtractLinkedTicketKey(%q) key", tc.description)
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
			key, ok := ExtractLinkedTicketKey(got)
			assert.True(t, ok, "ExtractLinkedTicketKey() could not find a footer in AppendJiraLink() result: %q", got)
			assert.Equal(t, issueKey, key, "ExtractLinkedTicketKey() key")
		})
	}
}

func TestEnsureJiraLink(t *testing.T) {
	const instanceURL = "https://jira.example.com"

	t.Run("no footer appends one", func(t *testing.T) {
		got, changed := EnsureJiraLink("some body", instanceURL, "OD-1")
		assert.True(t, changed, "EnsureJiraLink() changed")
		assert.Equal(t, AppendJiraLink("some body", instanceURL, "OD-1"), got, "EnsureJiraLink() result")
	})

	t.Run("footer already links the current key is left untouched", func(t *testing.T) {
		description := AppendJiraLink("some body", instanceURL, "OD-1")
		got, changed := EnsureJiraLink(description, instanceURL, "OD-1")
		assert.False(t, changed, "EnsureJiraLink() changed")
		assert.Equal(t, description, got, "EnsureJiraLink() result")
	})

	t.Run("footer linking a different key is replaced, body preserved", func(t *testing.T) {
		description := AppendJiraLink("some body", instanceURL, "OD-1")
		got, changed := EnsureJiraLink(description, instanceURL, "OD-2")
		assert.True(t, changed, "EnsureJiraLink() changed")
		assert.Equal(t, AppendJiraLink("some body", instanceURL, "OD-2"), got, "EnsureJiraLink() result")
		key, ok := ExtractLinkedTicketKey(got)
		assert.True(t, ok, "ExtractLinkedTicketKey() could not find a footer in EnsureJiraLink() result: %q", got)
		assert.Equal(t, "OD-2", key, "ExtractLinkedTicketKey() key")
	})

	t.Run("pre-fix footer format is replaced with the current format", func(t *testing.T) {
		description := "some body\n---\n🎫 [OD-1](url) <!-- mrboard -->"
		got, changed := EnsureJiraLink(description, instanceURL, "OD-2")
		assert.True(t, changed, "EnsureJiraLink() changed")
		assert.Equal(t, AppendJiraLink("some body", instanceURL, "OD-2"), got, "EnsureJiraLink() result")
	})
}
