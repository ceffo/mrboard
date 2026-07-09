package domain

import "testing"

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
			if got := HasJiraLink(tc.description); got != tc.want {
				t.Errorf("HasJiraLink(%q) = %v, want %v", tc.description, got, tc.want)
			}
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
			want:        "---\n🎫 [OD-123](https://jira.example.com/browse/OD-123) <!-- mrboard -->",
		},
		{
			name:        "non-empty description",
			description: "some body",
			want:        "some body\n---\n🎫 [OD-123](https://jira.example.com/browse/OD-123) <!-- mrboard -->",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AppendJiraLink(tc.description, instanceURL, issueKey)
			if got != tc.want {
				t.Errorf("AppendJiraLink() = %q, want %q", got, tc.want)
			}
			if !HasJiraLink(got) {
				t.Errorf("AppendJiraLink() result does not satisfy HasJiraLink: %q", got)
			}
		})
	}
}
