# Keybinding & Help System

Design for mrboard's keybinding registry, contextual footer, and `?` help modal.
Replaces the legacy per-widget `KeyMap` structs and manual `footer.SetKeyMap` swaps.

## Goals

1. **Single source of truth** — an action and its key binding are defined in exactly
   one place. Widgets, footer, and help modal all consume that one definition.
2. **Minimal, prioritized status line** — the footer never overflows: it shows
   `? help` plus as many bindings as fit (by priority), and the version is always
   visible, pinned to the right edge.
3. **Full contextual help on demand** — `?` toggles a centered modal listing every
   binding reachable *right now*, grouped by category.
4. **Self-consistent** — duplicate keys within a context are detected at init
   (panic) and in tests (`just check` fails).
5. **Composable contexts** — what is active is a *stack* of contexts (base →
   board → detail → overlay), computed from model state, never synced imperatively.

## Concepts

### Action

An `Action` wraps a `bubbles/key.Binding` with display metadata:

```go
type Action struct {
    key.Binding          // keys + help text (verb label, e.g. "sort" — never state)
    Priority Priority    // footer eligibility: P0 always, P1 core, P2 common, P3 modal-only
    Category Category    // help-modal grouping: Navigate, Act, View, General
}
```

- **Labels are static verbs.** State that used to be smuggled into labels
  (`sort:repo·id↑`, `my view`/`team view`) is rendered as a header indicator
  instead — it is state, not help.
- `Priority` decides footer inclusion order; `Category` decides modal grouping.

### Context

A context is a typed struct whose fields are `Action`s, plus metadata:

```go
type BoardKeyMap struct {
    Up, Down, Left, Right Action
    Detail, Refresh, Sort ... Action
}

var Board = NewContext("board", "Board", &DefaultBoardKeyMap)
```

`NewContext` uses reflection **once at init** to enumerate `Action` fields in
declaration order. That enumeration drives everything downstream (footer order
within a priority tier, modal rows, conflict check) — adding a field to the
struct is the *only* step needed to register a new binding.

Contexts that own a focused text input (e.g. the reviewer-editor search field)
are marked `capturesText`: all printable keys go to the input, so the base
context's `?`/`q` are suppressed and the footer shows only the context's own
escape hatches (`esc cancel • ↵ confirm`).

### Context stack (derived, not pushed)

The active stack is computed by one pure function on the model — there is no
imperative push/pop and therefore no way for the footer to drift out of sync
(the legacy design had 8 scattered `SetKeyMap` call sites; forgetting one was a
recurring bug class):

```go
// contextStack returns bottom→top: base is always present; the top context
// wins on key shadowing and owns the footer/help content.
func (m Model) contextStack() []*Context {
    stack := []*Context{Base}             // ? help, q quit — always on
    switch m.overlay.active() {
    case overlayKindDiffView:       return append(stack, DiffView)
    case overlayKindSettings:       return append(stack, SettingsCtx)
    case overlayKindReviewerEditor: return append(stack, m.reviewerEditor.Context()) // covers siblings too
    case overlayKindBatchPreview:   return append(stack, BatchPreview)
    }
    if m.showDetail {
        return append(stack, Detail)
    }
    return append(stack, Board)
}
```

Widgets with internal modes (reviewer editor searching vs. list mode) expose
`Context()` returning the context matching their current mode.

**Shadowing rule:** a key bound in a higher context wins over the same key
lower in the stack (diff view's `d` = close shadows board's `d` = diff). This
is legal and intentional. *Within* one context a duplicate key is always a bug.

### Dispatch

Widgets keep their existing `key.Matches` switches — but they match against the
`Action`s of the shared context definitions, never against locally re-declared
bindings. `key.NewBinding` may only be called inside `keys.go` context
definitions; a lint-style test enforces this (grep-test over the package).

### Configured commands (external launcher)

User-defined commands from `mrboard.yaml`'s `commands:` list (see
`mrboard.yaml.example` and [ADR-0004](adr/0004-external-command-launcher.md))
become a keybinding context at runtime, built by
`BuildCustomCommandsContext(cfg.Commands)` in `keys.go`. Unlike every other
context, its `Action`s are not declared as struct fields — the set of commands
is only known once config is loaded — so it is built via
`NewDynamicContext(name, title string, actions []*Action, opts ...ContextOpt)`
in `keymap.go`, a slice-based sibling to the reflection-based `NewContext`.
Downstream machinery (footer fill, help modal, shadowing) is unchanged, since
none of it depends on how a context's `actions` were populated.

The resulting context is pushed only in the `[Base, Board]` stack —
`[Base, Board, Commands]` — never in Detail or an overlay. Sitting above
`Board` means a configured key colliding with a board default (e.g. `r`)
shadows it via the ordinary shadowing rule, with no override logic of its own.
Every configured command is `PriorityModal` (footer never shows it,
regardless of count) and `CategoryAct` (grouped with `refresh`/`open
MR`/`reviewers`/`diff` in the `?` help modal).

A duplicate key across two configured commands is rejected by `internal/config`
at load time (mrboard refuses to start) — distinct from `NewContext`'s
init-time panic, which is reserved for bugs in `keys.go` itself, not user
config mistakes.

## Footer (status line)

```
 ? help • ↑↓←→ move • ↵ details • r refresh • s sort • q quit          v0.7.2
```

Render algorithm, given terminal width `W`:

1. Render the version, pinned right. Reserve its width + 2. **Never dropped.**
2. Start with the `? help` hint (from Base, P0).
3. Walk the top context's actions ordered by `(Priority, declaration order)`,
   then remaining Base actions (`q quit`).
4. Append each `key label` item if it fits *entirely* in the remaining width;
   stop at the first item that doesn't fit (no mid-item truncation).
5. Arrow-pair compression: Up/Down/Left/Right collapse to one `↑↓←→ move` item
   when all four are present (declared via a `Merge` group on the context).

Narrow terminal degradation is therefore automatic and deterministic:

```
 ? help • ↑↓←→ move • q quit                                           v0.7.2
```

## Help modal

`?` toggles; `esc` or `?` closes. Rendered as a top layer above whatever is
open (board, detail, or any overlay). Content = top context + visible Base
actions, grouped by `Category` into aligned columns; keys right-aligned in the
accent color, labels in muted foreground. Sized to content, capped at ~70% of
the terminal, centered.

```
        ╭─ Help · Board ──────────────────╮
        │                                 │
        │  Navigate         Act           │
        │  ↑/k   up         r  refresh    │
        │  ↓/j   down       o  open MR    │
        │  ←/h   left       v  reviewers  │
        │  →/l   right      d  diff       │
        │  ↵     details                  │
        │                   n  notify     │
        │  View             J  jira       │
        │  tab   toggle                   │
        │  s     sort       General       │
        │  S     sprint     ,  settings   │
        │                   q  quit       │
        │                                 │
        ╰─ ? · esc close ─────────────────╯
```

The modal is a self-contained widget (`help_modal.go`) with its own
`Init/Update/View`, styled exclusively from `styles.go` (`Help*` styles).
While open it owns key input (any key other than close is ignored).

## Conflict detection

Two enforcement layers, same predicate:

1. **Init panic** — `NewContext` builds a `key → action` index; a duplicate key
   within the context panics with both action names. Development builds crash
   instantly on a bad edit.
2. **Unit test** — `TestNoKeyConflicts` walks `AllContexts()` (the registry
   populated by `NewContext`) and asserts the same invariant, so `just check`
   catches it even if the TUI is never launched.

Cross-context duplicates are *not* errors (shadowing), but the test emits a
log of shadowed pairs so intentional shadowing stays visible in review.

## Header state indicators

Sort mode and view mode move out of key labels into the header stats area:

```
                    mrboard                    sort repo·id↑ • team • Total:12
```

## File layout

| File | Role |
|---|---|
| `internal/tui/keymap.go` | `Action`, `Category`, `Priority`, `Context`, `NewContext` (reflection + conflict panic), `NewDynamicContext` (slice-based, for configured commands), registry |
| `internal/tui/keys.go` | **Only** place where `key.NewBinding` appears: all context definitions, plus `BuildCustomCommandsContext` |
| `internal/tui/footer.go` | Priority-fill renderer; version pinned right |
| `internal/tui/help_modal.go` | `?` modal widget |
| `internal/tui/keymap_test.go` | conflict test + single-definition-site test |

## Legacy → new mapping

| Legacy | Replacement |
|---|---|
| `KeyMap` / `DetailKeyMap` / `DiffViewKeyMap` / `SettingsKeyMap` / `ReviewerEditorKeyMap` / `BatchReviewerEditorKeyMap` / `BatchPreviewKeyMap` | Context structs of `Action` fields (same names, `-Ctx` values) |
| `ShortHelp` / `FullHelp` + `bubbles/help.Model` | footer priority-fill + help modal (bubbles/help dropped) |
| `footer.SetKeyMap(...)` ×8 | `m.contextStack()` (derived) |
| `m.keys.Sort = key.NewBinding(...)` rebuilds | header indicator, static label |
| `CloseDetail` in board KeyMap | `Close` in Detail context |
