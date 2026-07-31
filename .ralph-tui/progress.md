# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

*Add reusable patterns discovered during development here.*

- New JSON/YAML-backed persistence adapters (statestore, snapshotstore) share one shape:
  `Config{Dir string}` -> `New(cfg)` does `os.MkdirAll(cfg.Dir, 0o700)` and stores just the
  file path -> `Load()`/`Save()` use `dirMode 0o700`/`fileMode 0o600`. For caches (as opposed
  to durable state), make `Load()` swallow every failure mode (absent file, corrupt JSON,
  version mismatch) into a zero-value/empty result with a nil error — the port's doc comment
  should say why (losing the cache costs one slow fetch, not a user-visible error).

---

## [2026-07-31] - mrr-incremental-fetch-3sl.2
- Verified `internal/adapters/snapshotstore.JSONStore` (already implemented, likely by a prior
  session whose `br close` never landed — the bead was found `in_progress` despite engram memory
  recording it as closed). Confirmed it satisfies every acceptance criterion: absent/corrupt/
  version-mismatch files all yield `(nil, time.Time{}, nil)`; round-trip test asserts equality
  including `UpdatedAt`; `WrittenAt` is exposed to callers. Wiring into `internal/core/core.go`
  (`Core.SnapshotStore`, built via `snapshotstore.New(snapshotstore.Config{Dir: config.XDGCacheDir()})`
  alongside the existing `StateStore`) was also already present.
- Files touched this session: none (verification only) — closed the bead and flushed beads state.
- **Learnings:**
  - When a ralph-tui bead shows up `in_progress` but engram memory says it was already closed in a
    prior session, check the actual files first — the implementation may be complete and only the
    `br close`/`git commit` step got lost (e.g. session ended before the automatic commit ran).
    Re-verify against the acceptance criteria and `just check` rather than re-implementing.
---

