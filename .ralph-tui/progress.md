# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

*Add reusable patterns discovered during development here.*

- **Promoting a private key type to an exported domain type**: when a package-private
  identity struct (e.g. `mrKey{projectID, iid int}`) needs to become the canonical
  exported type (`domain.MRKey{ProjectID, IID int}`), use a type alias
  (`type mrKey = domain.MRKey`) rather than a distinct type — it lets every existing
  `mrKey{...}` construction site keep compiling with only field-name casing fixes
  (map keys, composite literals), instead of a full rewrite to `domain.MRKey`.

---

## [2026-07-31] - mrr-incremental-fetch-3sl.1
- Implemented the pure type/port foundation for the incremental-fetch epic (docs/adr/0005):
  `domain.MRKey{ProjectID, IID int}` (internal/domain/mr.go), `MergeRequest.UpdatedAt time.Time`,
  `domain.SnapshotStore` port (internal/domain/state.go, sibling to `StateStore`),
  `mrsvc.FetchOptions.Previous []domain.MergeRequest`, `config.XDGCacheDir()` (mirrors
  `XDGConfigDir`/`XDGDataDir`, backed by `$XDG_CACHE_HOME` else `~/.cache/mrboard`).
- Promoted the private `mrKey` in `internal/adapters/gitlabadpt` to a type alias of
  `domain.MRKey` (`type mrKey = domain.MRKey`) so dedup.go/gitlabadpt.go keep compiling
  with only field-name casing fixes (`projectID`→`ProjectID`, `iid`→`IID`) at every
  `mrKey{...}` construction site — no behavior change.
- `UpdatedAt` now populated in both mapper paths: `MapMR` reads `mr.UpdatedAt *time.Time`
  (REST, same nil-guard pattern as `CreatedAt`); `MapMRFromGraphQL` parses
  `mr.UpdatedAt string` via `time.Parse(time.RFC3339, ...)` (same pattern as `CreatedAt`).
  Added `TestMapMR_UpdatedAt_Stored` / `TestMapMRFromGraphQL_UpdatedAt_Stored` in
  mapper_test.go, following the existing `SourceTargetBranch_Stored` test pair style.
- Added `SnapshotStore:` entry to `.mockery.yml` under the `internal/domain` package
  (same package block as `Notifier`, since `SnapshotStore` lives in `internal/domain/state.go`).
  `just generate` produced `internal/domain/mocks/mock_SnapshotStore.go` cleanly and is a
  no-op diff on a second run.
- Files changed: internal/domain/mr.go, internal/domain/state.go,
  internal/domain/service/mrsvc/mrsvc.go, internal/config/config.go,
  internal/adapters/gitlabadpt/{dedup.go,gitlabadpt.go,mapper.go,mapper_test.go},
  .mockery.yml, internal/domain/mocks/mock_SnapshotStore.go (new).
- **Learnings:**
  - `just check` passed clean: fmt + lint + build + 185 tests, no changes needed beyond
    the scoped additions.
  - No behavior change: `mrsvc.FetchOptions.Previous` is added but unused by any adapter
    yet — that's phase 2 (two-phase conditional fetch), a separate ticket in this epic.
  - `internal/domain/state.go` has zero imports at all (not even stdlib) — adding
    `SnapshotStore` referencing only `[]MergeRequest` (same package) kept it that way.
---

