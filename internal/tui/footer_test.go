package tui

import (
	"strings"
	"testing"

	lip "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestFooterWidget_Render_NoUpdate_OmitsBadge(t *testing.T) {
	f := newFooterWidget(NewStyles(LoadThemeByName("default"), true), "1.2.3")
	f.SetWidth(120)

	line := f.render([]*Context{BaseCtx})

	assert.Contains(t, line, "1.2.3")
	assert.False(t, strings.Contains(line, "↑"), "no badge expected when no update is available")
}

func TestFooterWidget_Render_UpdateAvailable_ShowsBadge(t *testing.T) {
	f := newFooterWidget(NewStyles(LoadThemeByName("default"), true), "1.2.3")
	f.SetUpdateAvailable(true)
	f.SetWidth(120)

	line := f.render([]*Context{BaseCtx})

	assert.Contains(t, line, "1.2.3")
	assert.Contains(t, line, "↑")
}

// TestFooterWidget_Render_NarrowWidth_NeverSquashesVersionOrBadge pins the
// widget's core guarantee: the version+badge block is reserved before any
// keybinding items are fit, so it survives even a terminal too narrow to
// show any binding items at all.
func TestFooterWidget_Render_NarrowWidth_NeverSquashesVersionOrBadge(t *testing.T) {
	f := newFooterWidget(NewStyles(LoadThemeByName("default"), true), "1.2.3")
	f.SetUpdateAvailable(true)
	f.SetWidth(lip.Width("1.2.3 ↑") + 1)

	line := f.render([]*Context{BaseCtx})

	assert.Contains(t, line, "1.2.3")
	assert.Contains(t, line, "↑")
}
