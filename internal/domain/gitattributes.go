package domain

import (
	"bufio"
	"strings"
)

// GeneratedAttr names a .gitattributes attribute that marks a path as generated.
type GeneratedAttr string

// GeneratedAttr values. GitLab collapses a path marked gitlab-generated; a path with
// gitlab-generated unspecified falls back to linguist-generated.
const (
	AttrGitLabGenerated   GeneratedAttr = "gitlab-generated"
	AttrLinguistGenerated GeneratedAttr = "linguist-generated"
)

// AttrState is the state one .gitattributes line gives an attribute.
type AttrState string

// AttrState values: "attr" or "attr=<value>" sets it (except "attr=false"), "-attr" or
// "attr=false" unsets it, and "!attr" returns it to unspecified.
const (
	AttrSet         AttrState = "set"
	AttrUnset       AttrState = "unset"
	AttrUnspecified AttrState = "unspecified"
)

// GeneratedRule is one generated-file marking from a repository's root .gitattributes.
type GeneratedRule struct {
	Glob  string        `json:"glob"`
	Attr  GeneratedAttr `json:"attr"`
	State AttrState     `json:"state"`
}

// ParseGeneratedRules extracts the gitlab-generated and linguist-generated markings of a
// root .gitattributes file, in file order. Comments, macro definitions ("[attr]..."),
// quoted patterns, patterns ending in "/" (which match no file in .gitattributes) and
// malformed globs are skipped.
func ParseGeneratedRules(content string) []GeneratedRule {
	var rules []GeneratedRule
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		const minFields = 2 // a pattern and at least one attribute
		if len(fields) < minFields {
			continue
		}
		glob := fields[0]
		if strings.HasPrefix(glob, "#") || strings.HasPrefix(glob, "[attr]") ||
			strings.HasPrefix(glob, `"`) || strings.HasSuffix(glob, "/") || ValidateSizeGlob(glob) != nil {
			continue
		}
		for _, tok := range fields[1:] {
			if attr, state, ok := parseGeneratedAttr(tok); ok {
				rules = append(rules, GeneratedRule{Glob: glob, Attr: attr, State: state})
			}
		}
	}
	return rules
}

func parseGeneratedAttr(tok string) (GeneratedAttr, AttrState, bool) {
	state := AttrSet
	switch {
	case strings.HasPrefix(tok, "-"):
		state, tok = AttrUnset, tok[1:]
	case strings.HasPrefix(tok, "!"):
		state, tok = AttrUnspecified, tok[1:]
	}
	name, value, hasValue := strings.Cut(tok, "=")
	if hasValue {
		if state != AttrSet {
			return "", "", false
		}
		if value == "false" {
			state = AttrUnset
		}
	}
	attr := GeneratedAttr(name)
	if attr != AttrGitLabGenerated && attr != AttrLinguistGenerated {
		return "", "", false
	}
	return attr, state, true
}

// markedGenerated reports whether rules mark filePath generated. As in .gitattributes,
// the last matching rule decides each attribute's state; gitlab-generated decides when it
// is set or unset, and linguist-generated decides when it is unspecified.
func markedGenerated(rules []GeneratedRule, filePath string) bool {
	states := map[GeneratedAttr]AttrState{}
	for _, r := range rules {
		if matchSizeGlob(r.Glob, filePath) {
			states[r.Attr] = r.State
		}
	}
	switch states[AttrGitLabGenerated] {
	case AttrSet:
		return true
	case AttrUnset:
		return false
	}
	return states[AttrLinguistGenerated] == AttrSet
}
