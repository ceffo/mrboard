package tui

import "strings"

// Canonical (lowercased) keys for the default issue type icon map.
const (
	issueTypeBug     = "bug"
	issueTypeStory   = "story"
	issueTypeTask    = "task"
	issueTypeEpic    = "epic"
	issueTypeSubtask = "subtask"
)

// defaultIssueTypeIcons maps canonical (lowercased) JIRA issue type names to
// their emoji icons. Lookup is case-insensitive.
var defaultIssueTypeIcons = map[string]string{
	issueTypeBug:     "🐛",
	issueTypeStory:   "📖",
	issueTypeTask:    "☑️",
	issueTypeEpic:    "⚡",
	issueTypeSubtask: "↩️",
}

// unknownIssueTypeIcon is returned when no match is found in the resolver map.
const unknownIssueTypeIcon = "🎫"

// IssueTypeIconResolver resolves a JIRA issue type string to an emoji icon.
// It merges a user-supplied override map (from config) on top of the defaults.
// Lookups are case-insensitive.
type IssueTypeIconResolver struct {
	icons map[string]string // all keys are lowercased
}

// NewIssueTypeIconResolver builds a resolver from the default icon map merged
// with any overrides from jira.issue_type_icons in the config. Override keys
// take precedence; lookups are case-insensitive.
func NewIssueTypeIconResolver(overrides map[string]string) IssueTypeIconResolver {
	merged := make(map[string]string, len(defaultIssueTypeIcons)+len(overrides))
	for k, v := range defaultIssueTypeIcons {
		merged[strings.ToLower(k)] = v
	}
	for k, v := range overrides {
		merged[strings.ToLower(k)] = v
	}
	return IssueTypeIconResolver{icons: merged}
}

// Resolve returns the emoji icon for the given JIRA issue type.
// Returns unknownIssueTypeIcon ("🎫") when issueType is empty or not found.
func (r IssueTypeIconResolver) Resolve(issueType string) string {
	if issueType == "" {
		return unknownIssueTypeIcon
	}
	if icon, ok := r.icons[strings.ToLower(issueType)]; ok {
		return icon
	}
	return unknownIssueTypeIcon
}
