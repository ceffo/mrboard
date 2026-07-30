# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

This repo does not use a `CONTEXT.md` convention, but it does keep a numbered ADR log. The
canonical domain context lives in:

- **`docs/architecture.md`** — package boundaries, data flow, dependency rules
- **`docs/domain-model.md`** — domain types, reviewer state machine, phase rules
- **`docs/tui-conventions.md`** — TUI file structure, widget rules, keybinding conventions
- **`docs/clean_architecture.md`** — ports-and-adapters principles used when redesigning significant architectural areas
- **`docs/adr/`** — numbered Architecture Decision Records; one per feature area, recording why a
  design was chosen, not just what it does. See "Recording decisions" below.

Read the files relevant to the area you're working in before proposing changes.

## File structure

Single-context repo:

```
/
├── docs/
│   ├── architecture.md        ← package boundaries + dependency rules
│   ├── domain-model.md        ← domain types + state machine
│   ├── tui-conventions.md     ← TUI widget rules + keybinding conventions
│   ├── clean_architecture.md  ← ports-and-adapters reference
│   └── adr/                   ← numbered ADRs, one per feature area
└── internal/
    └── domain/                ← source of truth for Go domain types
```

## Recording decisions

Decisions and conclusions live in **documents**, not tickets. This is about *location*, not
phrasing — a one-line gist in a ticket is still decision content in the wrong place. A beads (`br`)
ticket, including a `/wayfinder` map's own epic issue, only ever states what it's about and links
to the document that holds the real content; it never restates destination, context, out-of-scope
reasoning, or a resolution's substance, however briefly.

A resolved architectural or design decision is recorded in `docs/adr/` as a numbered ADR:
`**Status**` line, `## Context`, `## Non-goals`, `## Decision`, `## Consequences` (see
`docs/adr/0003-jira-remote-links.md` for the model). One ADR covers a whole feature area and
accumulates `## Decision` subsections as sub-decisions resolve — it is not one ADR per ticket.

Beads tickets close with a short `--reason` and nothing more. Engram memory is likewise not the
place for a development decision — it's useful for session continuity, not for what a future
contributor should find when asking "why is it built this way."

## Use the domain vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the terms as defined in `docs/domain-model.md`. Don't drift to synonyms the model explicitly avoids.

## Respect architectural boundaries

Before proposing a change that crosses package boundaries, check `docs/architecture.md`. If your proposal contradicts the stated dependency rules, surface the conflict explicitly rather than silently overriding:

> _Contradicts the architecture rule that `internal/domain` may only import stdlib — but worth revisiting because…_
