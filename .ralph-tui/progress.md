# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

### board.SetMRs is the right filter boundary for display-only MR exclusions
`board.SetMRs` (board.go) is called after all service-layer filtering. Add display-specific exclusions (e.g. hide no-reviewer MRs) here — it runs before column layout and card creation.

### Two-line title pattern for card.go
Use a `wrapTitleLines(title string, width int) []string` helper that breaks at word boundaries and hard-truncates line 2 with `truncateWidth`. Append styled title lines to rawLines before the empty separator. The per-line padding loop in render() handles variable card height automatically.

### Minute tick pattern (bubbletea v2)
`tea.Tick(time.Minute, fn)` fires once; re-schedule it on receipt to loop:
```go
case tickMsg:
    return m, tickCmd()
```
Since card.render() calls `time.Now()` fresh on every render, no extra state is needed — just receiving the tick triggers a re-render.

### test fixture someMRs() must have reviewers
After the no-reviewer filter was added to board.SetMRs, test MRs without reviewers are excluded from the board. Always add at least one ReviewerInfo to fixtures that need a focused card (e.g. detail panel tests).

---

## 2026-05-12 - mrr-erc.2
- Implemented username consistency and filter UX overhaul:
  1. **UserMap (Task A)**: `service.BuildUserMap` + `service.DisplayName` in service/filter.go; builds username→full-name map from reviewer info; model rebuilds it in `applyMRFilter` and stores as `m.userMap`
  2. **Card full names (Task B)**: `card.go` pills use `r.Name` with fallback to `r.Username`; author line was already full name, no change needed
  3. **Filter modal full names (Task C)**: `newFilterPopupWidget` accepts `userMap`; author labels use display name without `@`; reviewer labels use `lookupName(userMap, r)` (no `@` prefix)
  4. **@ prefix audit (Task D)**: removed all `"@" +` constructs from filter popup; no `@` on full names anywhere
  5. **Immediate-apply + close keybindings (Task E)**: Toggle emits `FilterAppliedMsg` immediately; model no longer closes popup on `FilterAppliedMsg`; added `FilterClosedMsg`; `f`/`Esc` close the popup; `Enter` does nothing inside popup; hint text updated
- Files changed: `internal/service/filter.go`, `internal/tui/keys.go`, `internal/tui/filter_popup.go`, `internal/tui/card.go`, `internal/tui/model.go`
- **Learnings:**
  - `domain.MergeRequest.Author` already stores the full name (set by mapper from `mr.Author.Name`); no username field on author — so UserMap only covers reviewers
  - Separating "filter changed" (`FilterAppliedMsg`, keeps popup open) from "filter closed" (`FilterClosedMsg`) is the clean immediate-apply pattern
  - `f` closes filter because popup's Update handles it before the board's `keys.Filter` binding sees it
  - `lookupName` helper in filter_popup.go avoids adding a service import to that file
---

## 2026-05-12 - mrr-erc.1
- Implemented three display fixes:
  1. **No-reviewer filter**: `hasAssignedReviewer()` helper in board.go; `SetMRs` skips MRs with no non-empty reviewer username
  2. **Two-line titles**: `wrapTitleLines()` helper in card.go; card render uses 1-2 title lines; card height is naturally variable
  3. **Minute tick**: `tickMsg` + `tickCmd()` in model.go using `tea.Tick(time.Minute, ...)`; re-scheduled on each receipt
- Files changed: `internal/tui/board.go`, `internal/tui/card.go`, `internal/tui/model.go`, `internal/tui/model_test.go`
- **Learnings:**
  - `domain.Reviewer` does not exist — the type is `domain.ReviewerInfo`
  - `replace_all` in Edit tool replaces ALL occurrences including substring matches like `ReviewerNotStarted` → `ReviewerInfoNotStarted`; be precise with the search string
  - `just check` caught the test breakage from adding the no-reviewer filter immediately
---

