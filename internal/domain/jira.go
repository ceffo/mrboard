package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// jiraIDPatternExact and jiraIDPatternAnyCase match the JIRA ticket ID
// embedded in conventional commit-style MR titles, e.g.
// "feat(OD-3345): add something" → "OD-3345". Submatch index 1 holds the
// captured ID. jiraIDPatternExact only recognizes an already-uppercase key;
// jiraIDPatternAnyCase also recognizes "fix(od-3345): ..." so a
// TicketKeyMatcher built case-insensitive can normalize it to "OD-3345".
var (
	jiraIDPatternExact   = regexp.MustCompile(`\(([A-Z]+-\d+)\)`)
	jiraIDPatternAnyCase = regexp.MustCompile(`\(([A-Za-z]+-\d+)\)`)
)

const jiraIDSubmatch = 1 // index of the captured group in the jiraIDPattern* vars

// TicketKeyMatcher extracts and normalizes ticket keys, built once with the
// desired case-sensitivity and then shared by every caller that parses or
// compares a ticket key, so the whole system agrees on one canonical form —
// whether the key came from an MR title, a JIRA API response, or a
// previously-extracted key being matched against another. The zero value is
// case-sensitive (equivalent to matching only exact-uppercase keys).
type TicketKeyMatcher struct {
	caseInsensitive bool
}

// NewTicketKeyMatcher returns a matcher. When caseInsensitive is true, the
// matcher recognizes a ticket key regardless of the case it's written in and
// upper-cases every key it returns; when false, only an already-uppercase
// key is recognized, and returned exactly as written.
func NewTicketKeyMatcher(caseInsensitive bool) TicketKeyMatcher {
	return TicketKeyMatcher{caseInsensitive: caseInsensitive}
}

// ExtractFromTitle returns the canonical ticket key embedded in title, or ""
// if none is found under this matcher's case rule.
func (m TicketKeyMatcher) ExtractFromTitle(title string) string {
	pattern := jiraIDPatternExact
	if m.caseInsensitive {
		pattern = jiraIDPatternAnyCase
	}
	match := pattern.FindStringSubmatch(title)
	if len(match) <= jiraIDSubmatch {
		return ""
	}
	return m.Normalize(match[jiraIDSubmatch])
}

// Normalize applies this matcher's case rule to an already-known key (e.g.
// one returned directly by the JIRA API) without parsing a title.
func (m TicketKeyMatcher) Normalize(key string) string {
	if m.caseInsensitive {
		return strings.ToUpper(key)
	}
	return key
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
