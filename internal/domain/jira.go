package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// jiraIDPattern matches the JIRA ticket ID embedded in conventional commit-style
// MR titles, e.g. "feat(OD-3345): add something" → "OD-3345".
// Submatch index 1 holds the captured ID.
var jiraIDPattern = regexp.MustCompile(`\(([A-Z]+-\d+)\)`)

const jiraIDSubmatch = 1 // index of the captured group in jiraIDPattern

// ExtractJiraID returns the JIRA issue ID from an MR title, or "" if none found.
func ExtractJiraID(title string) string {
	m := jiraIDPattern.FindStringSubmatch(title)
	if len(m) <= jiraIDSubmatch {
		return ""
	}
	return m[jiraIDSubmatch]
}

// JiraIssueURL builds the browse URL for a JIRA issue given the instance base URL.
// Returns "" if either argument is empty.
func JiraIssueURL(instanceURL, issueID string) string {
	if instanceURL == "" || issueID == "" {
		return ""
	}
	return fmt.Sprintf("%s/browse/%s", instanceURL, issueID)
}

// jiraLinkMarker is the HTML comment sentinel appended alongside the back-link
// injected into an MR description. Its presence means the link was already written.
const jiraLinkMarker = "<!-- mrboard -->"

// HasJiraLink reports whether an MR description already carries the mrboard
// back-link marker, per ADR-0003's idempotency rule.
func HasJiraLink(description string) bool {
	return strings.Contains(description, jiraLinkMarker)
}

// AppendJiraLink returns description with a JIRA back-link line appended:
// existing body + "\n---\n🎫 [KEY](url) <!-- mrboard -->".
func AppendJiraLink(description, instanceURL, issueKey string) string {
	suffix := fmt.Sprintf("---\n🎫 [%s](%s) %s", issueKey, JiraIssueURL(instanceURL, issueKey), jiraLinkMarker)
	if description == "" {
		return suffix
	}
	return description + "\n" + suffix
}
