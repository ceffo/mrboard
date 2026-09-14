package gitlab

import "regexp"

// draftPrefixPattern matches a GitLab draft/WIP title marker, case-insensitive,
// ordered most-specific first so alternation picks the right branch:
// "[Draft]", "(Draft)", "Draft:", "Draft", "[WIP]", "WIP:", "WIP".
var draftPrefixPattern = regexp.MustCompile(`(?i)^\s*(\[draft\]|\(draft\)|draft:|draft|\[wip\]|wip:|wip)\s*`)

// stripDraftPrefix removes a leading GitLab draft/WIP marker from title. GitLab
// detects the marker's absence server-side and flips the MR's draft state —
// there is no separate API field for it.
func stripDraftPrefix(title string) string {
	return draftPrefixPattern.ReplaceAllString(title, "")
}
