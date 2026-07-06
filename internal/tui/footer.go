package tui

import (
	"strings"

	lip "charm.land/lipgloss/v2"
)

// footerWidget renders the one-line status bar: a prioritized selection of
// the active context's keybindings on the left and the app version pinned to
// the right edge. The version is never sacrificed; binding items are dropped
// whole (lowest priority first) when the terminal is too narrow.
type footerWidget struct {
	styles  Styles
	version string
	width   int
}

func newFooterWidget(styles Styles, version string) footerWidget {
	return footerWidget{styles: styles, version: version}
}

// SetStyles updates the footer's style set.
func (f *footerWidget) SetStyles(s Styles) { f.styles = s }

// SetWidth updates available width so the version is pinned to the right edge.
func (f *footerWidget) SetWidth(w int) { f.width = w }

// render builds the status line for the given context stack (bottom → top).
// Layout: `? help • <top-context items by priority> • q quit …… version`.
// While the top context captures text, only its own items are shown (base
// help/quit keys go to the text input instead).
func (f footerWidget) render(stack []*Context) string {
	top := stack[len(stack)-1]

	var items []footerItem
	switch {
	case top == BaseCtx || top.CapturesText():
		items = top.footerItems()
	default:
		if !top.binds("?") {
			items = append(items, baseItem(DefaultBaseKeyMap.Help))
		}
		items = append(items, top.footerItems()...)
		if !top.binds("q") {
			items = append(items, baseItem(DefaultBaseKeyMap.Quit))
		}
	}

	ver := f.styles.FooterVersion.Render(f.version)
	if f.width <= 0 {
		return f.styles.Footer.Render(f.renderItems(items) + " " + ver)
	}

	avail := f.width - lip.Width(ver) - 1
	line := f.fitItems(items, avail)
	pad := f.width - lip.Width(line) - lip.Width(ver)
	if pad < 1 {
		pad = 1
	}
	return f.styles.Footer.Render(line + strings.Repeat(" ", pad) + ver)
}

// baseItem converts a base-context action into a pinned footer item.
func baseItem(a Action) footerItem {
	return footerItem{key: a.Help().Key, label: a.Help().Desc, priority: a.Priority}
}

// fitItems selects which items fit within avail columns: pinned items are
// reserved first and never dropped, then the remaining items fill leftover
// space in slice order (already priority-sorted), stopping at the first that
// would overflow. Display preserves the original order; items are never
// truncated mid-entry.
func (f footerWidget) fitItems(items []footerItem, avail int) string {
	sep := f.styles.FooterSep.Render(" • ")
	sepW := lip.Width(sep)

	widths := make([]int, len(items))
	kept := make([]bool, len(items))
	used, count := 0, 0
	take := func(i int) {
		used += widths[i]
		if count > 0 {
			used += sepW
		}
		kept[i] = true
		count++
	}
	for i, it := range items {
		widths[i] = lip.Width(f.renderItem(it))
		if it.priority == PriorityPinned {
			take(i)
		}
	}
	for i := range items {
		if kept[i] {
			continue
		}
		need := widths[i]
		if count > 0 {
			need += sepW
		}
		if used+need > avail {
			break
		}
		take(i)
	}

	var line string
	for i := range items {
		if !kept[i] {
			continue
		}
		if line != "" {
			line += sep
		}
		line += f.renderItem(items[i])
	}
	return line
}

func (f footerWidget) renderItems(items []footerItem) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = f.renderItem(it)
	}
	return strings.Join(parts, f.styles.FooterSep.Render(" • "))
}

func (f footerWidget) renderItem(it footerItem) string {
	return f.styles.FooterKey.Render(it.key) + " " + f.styles.Footer.Render(it.label)
}
