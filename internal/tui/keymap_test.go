package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
)

// TestNoKeyConflicts asserts the registry invariant: within one context every
// key maps to exactly one action. (NewContext already panics on violation at
// init; this test makes `just check` fail even when the TUI never launches.)
// Cross-context duplicates are legal shadowing — logged so they stay visible.
func TestNoKeyConflicts(t *testing.T) {
	require.NotEmpty(t, AllContexts(), "no contexts registered")
	type owner struct{ ctx, field string }
	seen := map[string]owner{}
	for _, ctx := range AllContexts() {
		keys := map[string]string{}
		for _, a := range ctx.actions {
			for _, k := range a.Keys() {
				prev, dup := keys[k]
				assert.False(t, dup, "context %q: key %q bound to both %q and %q",
					ctx.Name(), k, prev, a.Help().Desc)
				keys[k] = a.Help().Desc
				if prev, ok := seen[k]; ok && prev.ctx != ctx.Name() {
					t.Logf("shadowing: key %q in %q (was %q in %q)", k, ctx.Name(), prev.field, prev.ctx)
				}
				seen[k] = owner{ctx: ctx.Name(), field: a.Help().Desc}
			}
		}
	}
}

// TestBindingsDefinedOnlyInKeys enforces the single-source-of-truth rule:
// Action construction (Act / key.NewBinding) may only appear in keys.go
// (definitions) and keymap.go (the Act constructor itself).
func TestBindingsDefinedOnlyInKeys(t *testing.T) {
	newBinding := regexp.MustCompile(`key\.NewBinding\(`)
	actCall := regexp.MustCompile(`\bAct\(`)

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "keys.go" || f == "keymap.go" {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		for i, line := range strings.Split(string(src), "\n") {
			assert.False(t, newBinding.MatchString(line),
				"%s:%d: key.NewBinding outside keys.go — define the Action in keys.go instead", f, i+1)
			assert.False(t, actCall.MatchString(line),
				"%s:%d: Act() outside keys.go — define the Action in keys.go instead", f, i+1)
		}
	}
}

// TestFooterItemsRespectPriorityAndGroups covers the footer candidate logic:
// grouped nav collapses to one item, modal-only actions are excluded, and
// items come out sorted by priority.
func TestFooterItemsRespectPriorityAndGroups(t *testing.T) {
	items := BoardCtx.footerItems()
	require.NotEmpty(t, items, "board context has no footer items")
	assert.False(t, items[0].key != "↑↓←→" || items[0].label != "move",
		"first board footer item = %q %q, want grouped nav ↑↓←→ move", items[0].key, items[0].label)
	for _, it := range items {
		assert.False(t, it.label == "up" || it.label == "down" || it.label == "left" || it.label == "right",
			"group member %q leaked into footer items", it.label)
		assert.False(t, it.label == "settings" || it.label == "notify",
			"modal-only action %q leaked into footer items", it.label)
	}
	for i := 1; i < len(items); i++ {
		assert.False(t, items[i].priority < items[i-1].priority,
			"footer items not sorted by priority: %v before %v", items[i-1], items[i])
	}
}

// TestFooterPinnedNeverDropped verifies that on a narrow terminal the pinned
// items ("? help", "q quit") survive while lower-priority items are dropped,
// and that the version stays within the line.
func TestFooterPinnedNeverDropped(t *testing.T) {
	f := newFooterWidget(NewStyles(LoadThemeByName(""), true), "v0.0.0")
	f.SetWidth(40)
	line := f.render([]*Context{BaseCtx, BoardCtx})
	for _, pinned := range []string{"help", "quit", "v0.0.0"} {
		assert.True(t, strings.Contains(line, pinned), "narrow footer dropped pinned element %q: %q", pinned, line)
	}
	assert.False(t, strings.Contains(line, "refresh"), "narrow footer kept low-priority item refresh: %q", line)
}

// TestHelpSectionsShadowing verifies stack semantics: a key claimed by a
// higher context hides the lower context's action, and a text-capturing top
// context hides lower contexts entirely.
func TestHelpSectionsShadowing(t *testing.T) {
	// Diff view binds "d" (close); base binds "q"/"?" — both visible, but the
	// board is not part of this stack.
	sections := helpSections([]*Context{BaseCtx, DiffViewCtx})
	var labels []string
	for _, s := range sections {
		for _, e := range s.entries {
			labels = append(labels, e.label)
		}
	}
	joined := strings.Join(labels, ",")
	assert.True(t, strings.Contains(joined, "close") && strings.Contains(joined, "quit"),
		"diff+base sections missing expected entries: %v", labels)

	// Text capture on top: base's help/quit must disappear.
	sections = helpSections([]*Context{BaseCtx, ReviewerSearchCtx})
	for _, s := range sections {
		for _, e := range s.entries {
			assert.False(t, e.label == "help" || e.label == "quit",
				"base action %q visible under text-capturing context", e.label)
		}
	}
}

// TestBuildCustomCommandsContext covers the configured-commands context from
// docs/adr/0004-external-command-launcher.md: commands are modal-only (never
// footer items) and a configured key colliding with a board default shadows
// it once stacked above BoardCtx — the existing stacking rule, no new
// override logic.
func TestBuildCustomCommandsContext(t *testing.T) {
	cmds := []config.Command{
		{Name: "code review", Key: "R", Binary: "tuicr"},
		{Name: "refresh via tool", Key: "r", Binary: "hunk"}, // collides with Board's "r" refresh
	}
	ctx := BuildCustomCommandsContext(cmds)

	assert.Empty(t, ctx.footerItems(), "configured commands must never appear in the footer")

	sections := helpSections([]*Context{BaseCtx, BoardCtx, ctx})
	var labels []string
	for _, s := range sections {
		for _, e := range s.entries {
			labels = append(labels, e.label)
		}
	}
	joined := strings.Join(labels, ",")
	assert.True(t, strings.Contains(joined, "code review"), "custom command missing from help modal: %v", labels)
	assert.True(t, strings.Contains(joined, "refresh via tool"),
		"shadowing custom command missing from help modal: %v", labels)
	assert.False(t, strings.Contains(joined, "refresh"+","),
		"board default not shadowed by colliding custom key: %v", labels)
}

// TestNewDynamicContextPanicsOnDuplicateKey mirrors NewContext's intra-context
// conflict panic. Configured-command duplicate keys are expected to be
// rejected earlier, at config load time — this stays a defensive invariant.
func TestNewDynamicContextPanicsOnDuplicateKey(t *testing.T) {
	a1 := Act("x", "one", PriorityModal, CategoryAct)
	a2 := Act("x", "two", PriorityModal, CategoryAct)
	assert.Panics(t, func() {
		NewDynamicContext("dup-test", "Dup", []*Action{&a1, &a2})
	})
}
