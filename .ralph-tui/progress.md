# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

*Add reusable patterns discovered during development here.*

---

## 2026-05-28 - mrr-ypr.8
- Removed `f` from `DefaultFilterPopupKeyMap.Close` (now `esc`-only)
- Removed `t` from `DefaultThemePickerKeyMap.Close` (now `esc`-only)
- Updated popup hint text in `filter_popup.go` and `theme_picker.go` to drop `f/` and `t/` prefixes
- Added missing `newThemePickerWidget` constructor to `theme_picker.go` (was referenced in model.go but undefined)
- SettingsKeyMap and `,` for settings were already present from a prior iteration
- Added `//nolint:unused` to `newThemePickerWidget` and `openThemePicker` (will be wired in mrr-ypr.9)
- Files changed: `internal/tui/keys.go`, `internal/tui/filter_popup.go`, `internal/tui/theme_picker.go`, `internal/tui/model.go`
- **Learnings:**
  - The `//nolint:unused` directive must go on the line *before* the function, not inline on the signature line, when the signature is near the 120-char lll limit
  - Functions marked `//nolint:unused` can still cause lll violations if placed inline — break the directive onto its own line
---

## 2026-05-28 - mrr-ypr.10
- Deleted `internal/tui/filter_popup.go` and `internal/tui/theme_picker.go`
- Moved shared sub-widget helpers into `settings_widget.go`: `filterStatusWidget`, `filterSelectWidget`, `filterSelectItem`, `filterFocus`/constants, `renderSectionHeader`, phase/marker constants, `modeOptions`, `pickerRadioPrefixLen`
- Removed `filterStatusWidget.moveCursor` (was only used by deleted `filterPopupWidget`)
- Cleaned up `model.go`: removed `filterPopup`/`showFilter`/`themePicker`/`showThemePicker` fields, `filterKeys`/`themePickerKeys` fields, dead `Update` cases (`FilterAppliedMsg`, `FilterClosedMsg`, `ThemePickerClosedMsg`, `ThemeChangedMsg`), dead key/render branches, `openThemePicker` and `handleThemeChanged` functions
- Cleaned up `keys.go`: removed `FilterPopupKeyMap`/`DefaultFilterPopupKeyMap` and `ThemePickerKeyMap`/`DefaultThemePickerKeyMap`
- Files changed: `internal/tui/settings_widget.go` (expanded), `internal/tui/model.go`, `internal/tui/keys.go`; deleted `filter_popup.go`, `theme_picker.go`
- **Learnings:**
  - When deleting a file whose types are still used by another file in the same package, move shared types/helpers to the surviving file rather than creating a new helper file — keeps the dependency explicit and avoids unnecessary indirection
  - Unused method lint (`unused`) will catch pointer-receiver methods not called anywhere — check for methods that were only called from the deleted code
---

## 2026-05-28 - mrr-ypr.9
- Created `internal/tui/settings_widget.go` — 4-tab settings panel (General / Filters / Sorting / Theme)
- `SettingsAppliedMsg` and `SettingsClosedMsg` defined in the new file
- Wired into `model.go`: `showSettings`/`settings` fields, `openSettings()`, `handleSettingsApplied()`, `handleMembersLoaded()` helper (extracted to keep cyclomatic complexity ≤ 30), key handler in `handleKey` and `handleKeyBoard`, overlay in `renderContent`, style propagation in `applyTheme`
- Files changed: `internal/tui/settings_widget.go` (new), `internal/tui/model.go`
- **Learnings:**
  - `Update()` had cyclomatic complexity exactly 30; adding 2 new cases pushed it to 31. Fix: extract one existing inline case (`MembersLoadedMsg`) into `handleMembersLoaded()` to bring it back to 30.
  - The `SettingsKeyMap.PrevTab/NextTab` uses `tab/right/l` and `shift+tab/left/h` — this conflicts with within-tab section navigation if reusing the filter popup's tab/shift+tab pattern. Solution: boundary-crossing up/down auto-switches sections instead.
  - `settingsPickerMaxVisible` reuses the same two-pane layout as `themePickerWidget`; `pickerRadioPrefixLen` constant from `theme_picker.go` is reused for the mode column width calculation.
---

