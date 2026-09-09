package tui

import (
	"testing"

	lip "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

// TestFooterWidget_Render_NarrowWidth_NeverSquashesVersion pins the widget's
// core guarantee: the version segment is reserved before any keybinding items
// are fit, so it survives even a terminal too narrow to show any binding
// items at all.
func TestFooterWidget_Render_NarrowWidth_NeverSquashesVersion(t *testing.T) {
	v := newTestVersionWidget("1.2.3")
	v.available, v.latest = true, "1.3.0"
	f := newFooterWidget(NewStyles(LoadThemeByName("default"), true), v)
	f.SetWidth(lip.Width(v.render()) + 1)

	line := f.render([]*Context{BaseCtx})

	assert.Contains(t, line, "1.2.3")
	assert.Contains(t, line, "↑")
}

func TestFooterWidget_Render_IncludesVersionSegment(t *testing.T) {
	f := newFooterWidget(NewStyles(LoadThemeByName("default"), true), newTestVersionWidget("1.2.3"))
	f.SetWidth(120)

	line := f.render([]*Context{BaseCtx})

	assert.Contains(t, line, "1.2.3")
}
