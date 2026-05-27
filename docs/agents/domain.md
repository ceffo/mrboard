# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

This repo does not use a `CONTEXT.md` or `docs/adr/` convention. The canonical domain context lives in:

- **`docs/architecture.md`** — package boundaries, data flow, dependency rules
- **`docs/domain-model.md`** — domain types, reviewer state machine, phase rules
- **`docs/tui-conventions.md`** — TUI file structure, widget rules, keybinding conventions
- **`docs/clean_architecture.md`** — ports-and-adapters principles used when redesigning significant architectural areas

Read the files relevant to the area you're working in before proposing changes.

## File structure

Single-context repo:

```
/
├── docs/
│   ├── architecture.md        ← package boundaries + dependency rules
│   ├── domain-model.md        ← domain types + state machine
│   ├── tui-conventions.md     ← TUI widget rules + keybinding conventions
│   └── clean_architecture.md  ← ports-and-adapters reference
└── internal/
    └── domain/                ← source of truth for Go domain types
```

## Use the domain vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the terms as defined in `docs/domain-model.md`. Don't drift to synonyms the model explicitly avoids.

## Respect architectural boundaries

Before proposing a change that crosses package boundaries, check `docs/architecture.md`. If your proposal contradicts the stated dependency rules, surface the conflict explicitly rather than silently overriding:

> _Contradicts the architecture rule that `internal/domain` may only import stdlib — but worth revisiting because…_
