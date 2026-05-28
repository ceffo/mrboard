package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lip "charm.land/lipgloss/v2"
)

// ThemeChangedMsg is sent on every navigation move in the theme picker.
type ThemeChangedMsg struct {
	Name string
	Mode string
}

// ThemePickerClosedMsg is sent when the user closes the theme picker.
type ThemePickerClosedMsg struct{}

var modeOptions = []string{themeModeAuto, themeModeDark, themeModeLight}

const (
	pickerFocusList      = 0
	pickerFocusMode      = 1
	pickerNumPanes       = 2
	pickerMaxVisible     = 12
	pickerListWidth      = 22
	pickerModeWidth      = 10
	pickerRadioPrefixLen = 2 // "● " or "○ "
)

// newThemePickerWidget creates a themePickerWidget with initial cursor positions matching
// the currently selected theme name and mode.
//
//nolint:unused
func newThemePickerWidget( //nolint:deadcode
	themes []string, currentTheme, currentMode string, styles Styles, keys ThemePickerKeyMap,
) themePickerWidget {
	cursor := 0
	for i, name := range themes {
		if name == currentTheme {
			cursor = i
			break
		}
	}
	modeCursor := 0
	for i, m := range modeOptions {
		if m == currentMode {
			modeCursor = i
			break
		}
	}
	p := themePickerWidget{
		themes:     themes,
		cursor:     cursor,
		modeCursor: modeCursor,
		styles:     styles,
		keys:       keys,
	}
	if cursor >= pickerMaxVisible {
		p.scrollOff = cursor - pickerMaxVisible + 1
	}
	return p
}

// themePickerWidget is the overlay popup for live theme and mode selection.
type themePickerWidget struct {
	themes     []string
	cursor     int
	modeCursor int
	focus      int
	scrollOff  int
	styles     Styles
	keys       ThemePickerKeyMap
}

// Init implements tea.Model.
func (p themePickerWidget) Init() tea.Cmd { return nil }

// View implements tea.Model.
func (p themePickerWidget) View() tea.View { return tea.NewView(p.render()) }

// Update implements tea.Model.
func (p themePickerWidget) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn
	kMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}

	switch {
	case key.Matches(kMsg, p.keys.Close):
		return p, func() tea.Msg { return ThemePickerClosedMsg{} }

	case key.Matches(kMsg, p.keys.FocusNext), key.Matches(kMsg, p.keys.FocusPrev):
		p.focus = (p.focus + 1) % pickerNumPanes
		return p, nil

	case key.Matches(kMsg, p.keys.Up):
		if p.focus == pickerFocusList {
			if p.cursor > 0 {
				p.cursor--
				p.adjustScroll()
				return p, p.emitChange()
			}
		} else {
			if p.modeCursor > 0 {
				p.modeCursor--
				return p, p.emitChange()
			}
		}

	case key.Matches(kMsg, p.keys.Down):
		if p.focus == pickerFocusList {
			if p.cursor < len(p.themes)-1 {
				p.cursor++
				p.adjustScroll()
				return p, p.emitChange()
			}
		} else {
			if p.modeCursor < len(modeOptions)-1 {
				p.modeCursor++
				return p, p.emitChange()
			}
		}

	case key.Matches(kMsg, p.keys.Confirm):
		return p, p.emitChange()
	}

	return p, nil
}

func (p themePickerWidget) emitChange() tea.Cmd {
	name := ""
	if p.cursor < len(p.themes) {
		name = p.themes[p.cursor]
	}
	mode := modeOptions[p.modeCursor]
	return func() tea.Msg {
		return ThemeChangedMsg{Name: name, Mode: mode}
	}
}

func (p *themePickerWidget) adjustScroll() {
	if p.cursor < p.scrollOff {
		p.scrollOff = p.cursor
	} else if p.cursor >= p.scrollOff+pickerMaxVisible {
		p.scrollOff = p.cursor - pickerMaxVisible + 1
	}
}

func (p themePickerWidget) render() string {
	// --- Left pane: theme list ---
	end := p.scrollOff + pickerMaxVisible
	if end > len(p.themes) {
		end = len(p.themes)
	}
	visible := p.themes[p.scrollOff:end]

	var listLines []string
	for i, name := range visible {
		idx := p.scrollOff + i
		selected := idx == p.cursor
		focused := p.focus == pickerFocusList && selected
		prefix := "  "
		if selected {
			prefix = "▶ "
		}
		padWidth := pickerListWidth - len([]rune(prefix))
		raw := fmt.Sprintf("%s%-*s", prefix, padWidth, name)
		var line string
		if focused {
			line = p.styles.PopupItemFocused.Render(raw)
		} else {
			line = p.styles.PopupItem.Render(raw)
		}
		listLines = append(listLines, line)
	}
	emptyRaw := fmt.Sprintf("%-*s", pickerListWidth, "")
	for len(listLines) < pickerMaxVisible {
		listLines = append(listLines, p.styles.PopupItem.Render(emptyRaw))
	}
	listPane := strings.Join(listLines, "\n")

	// --- Vertical divider ---
	var divLines []string
	for range pickerMaxVisible {
		divLines = append(divLines, p.styles.PopupDivider.Render("│"))
	}
	divider := strings.Join(divLines, "\n")

	// --- Right pane: mode radio ---
	var modeLines []string
	for i, m := range modeOptions {
		selected := i == p.modeCursor
		focused := p.focus == pickerFocusMode && selected
		radio := "○"
		if selected {
			radio = "●"
		}
		padWidth := pickerModeWidth - pickerRadioPrefixLen
		raw := fmt.Sprintf("%s %-*s", radio, padWidth, m)
		var line string
		if focused {
			line = p.styles.PopupItemFocused.Render(raw)
		} else {
			line = p.styles.PopupItem.Render(raw)
		}
		modeLines = append(modeLines, line)
	}
	emptyModeRaw := fmt.Sprintf("%-*s", pickerModeWidth, "")
	for len(modeLines) < pickerMaxVisible {
		modeLines = append(modeLines, p.styles.PopupItem.Render(emptyModeRaw))
	}
	modePane := strings.Join(modeLines, "\n")

	// --- Compose ---
	body := lip.JoinHorizontal(lip.Top, listPane, divider, modePane)
	hint := p.styles.PopupHint.Render("  ↑/↓ move  tab switch pane  esc close")
	content := p.styles.PopupTitle.Render("Themes") + "\n" + body + "\n" + hint
	return p.styles.PopupBorder.Render(content)
}
