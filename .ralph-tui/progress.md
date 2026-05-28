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

