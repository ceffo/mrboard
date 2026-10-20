package tui

import (
	"testing"

	lip "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ceffo/mrboard/internal/config"
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

// TestFooterWidget_Render_CustomCommandsContextKeepsBoardHints pins that the
// configured-commands context, always stacked above BoardCtx, never hides the
// board's own hints, whether or not any command is configured.
func TestFooterWidget_Render_CustomCommandsContextKeepsBoardHints(t *testing.T) {
	boardItems := BoardCtx.footerItems()
	require.NotEmpty(t, boardItems)

	cases := map[string][]config.Command{
		"no commands": nil,
		"configured":  {{Name: "deploy tool", Key: "R", Binary: "deploy"}},
	}
	for name, cmds := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFooterWidget(NewStyles(LoadThemeByName("default"), true), newTestVersionWidget("1.2.3"))
			f.SetWidth(200)

			line := f.render([]*Context{BaseCtx, BoardCtx, BuildCustomCommands(cmds).Context()})

			assert.Contains(t, line, boardItems[0].label)
			assert.Contains(t, line, "help")
			assert.Contains(t, line, "quit")
			assert.NotContains(t, line, "deploy tool", "configured commands stay modal-only")
		})
	}
}
