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

// jiraLinkAgentNotice is an HTML comment warning coding agents editing the MR
// description not to touch the automated back-link below it. HTML comments
// are invisible in rendered markdown, so this has no visual effect.
const jiraLinkAgentNotice = "<!-- mrboard: automated back-link below — do not edit or remove -->"

// jiraFooterPattern matches the mrboard-authored back-link footer built by
// AppendJiraLink, anchored to the end of the description. The agent-notice
// line and the blank line before "---" are both optional in the pattern so a
// footer written before either was introduced is still recognized.
var jiraFooterPattern = regexp.MustCompile(
	`\n*---\n(?:<!--[^\n]*-->\n)?🎫 \[([A-Za-z]+-\d+)\]\([^)]*\) <!-- mrboard -->\s*$`)

// ExtractLinkedTicketKey returns the ticket key already recorded in an MR
// description's mrboard back-link footer, and whether a footer was found.
func ExtractLinkedTicketKey(description string) (key string, ok bool) {
	match := jiraFooterPattern.FindStringSubmatch(description)
	if match == nil {
		return "", false
	}
	return match[jiraIDSubmatch], true
}

// AppendJiraLink returns description with a JIRA back-link footer appended:
// existing body + "\n\n---\n<agent notice>\n🎫 [KEY](url) <!-- mrboard -->".
// The blank line before "---" is load-bearing: without it, GFM parses "---"
// as a setext heading underline for the description's last line instead of
// a thematic break, turning that line into a heading.
func AppendJiraLink(description, instanceURL, issueKey string) string {
	suffix := fmt.Sprintf("---\n%s\n🎫 [%s](%s) %s",
		jiraLinkAgentNotice, issueKey, JiraIssueURL(instanceURL, issueKey), jiraLinkMarker)
	if description == "" {
		return suffix
	}
	return description + "\n\n" + suffix
}

// EnsureJiraLink returns description with its mrboard back-link footer set to
// issueKey, and whether the result differs from description. A missing
// footer is appended; a footer already linking issueKey is returned
// untouched; a footer linking a different key — e.g. the MR was retitled
// onto a different ticket — is replaced rather than duplicated.
func EnsureJiraLink(description, instanceURL, issueKey string) (string, bool) {
	loc := jiraFooterPattern.FindStringSubmatchIndex(description)
	if loc == nil {
		return AppendJiraLink(description, instanceURL, issueKey), true
	}
	if description[loc[2]:loc[3]] == issueKey {
		return description, false
	}
	return AppendJiraLink(description[:loc[0]], instanceURL, issueKey), true
}
