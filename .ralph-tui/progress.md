# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

*Add reusable patterns discovered during development here.*

- **Measuring real fetch timing**: `internal/adapters/gitlabadpt/gitlabadpt.go` logs
  `gitlab: phase-2 diff` (unchanged/changed counts) and `gitlab: fetch done` (`total_duration`)
  at Info level to whatever `log.path` the active `mrboard.yaml` points at (`tail` that file after
  a run). `mrboard fetch` always passes a nil `Previous`, so it's a ready-made cold/full-fetch
  baseline; the TUI's `Init()` fetch passes the snapshot-store-loaded `m.allMRs` as `Previous`, so
  booting the real TUI (via `agent-tui run scripts/run-tui.sh`) against an existing
  `~/.cache/mrboard/snapshot.json` is a ready-made warm-path measurement — no test harness needed.

---

## [2026-07-31] - mrr-incremental-fetch-3sl.4
- Found the phase-2 implementation already complete and committed (bundled into commit `216bc64`
  from a prior ralph-tui iteration): `internal/adapters/gitlabadpt/phase2.go`
  (`diffGQLStage`/`enrichGQLMRsBatch`), `mapper.go`'s `MergeMRFromGraphQL` (the cache-merge rule —
  reuses only Reviewers/OpenThreads/RoundTripCount on a hit, always overwrites
  DetailedMergeStatus/Draft/approver-derived fields), `mrsvc.FetchOptions.ForceStale`, and the full
  table-driven test suite in `merge_test.go` plus the zero-request and nil-Previous tests in
  `gitlabadpt_test.go`. No code changes needed this session — verified the implementation against
  every acceptance line and closed the remaining open item (timing measurement).
- Files touched this session: `.ralph-tui/progress.md` only (this entry).
- **Measured end-to-end timing** (real GitLab instance, 7 MRs, `mrboard.yaml` config,
  `/tmp/mrboard.log`):
  - Cold / full fetch (`./bin/mrboard fetch`, always nil `Previous` → all 7 MRs treated as
    changed): `total_duration: 3s` (wall clock `time`: 3.49s) — inside the pre-epic 2-9s baseline,
    confirms nil-Previous full-fetch parity.
  - Warm, 0 changed (`agent-tui` boot of the real TUI against an existing warm
    `~/.cache/mrboard/snapshot.json`, all 7 MRs unchanged since the snapshot): `phase-2 diff:
    unchanged=7 changed=0`, phase 2 issued **zero** discussion requests, `total_duration: 836ms`
    (phase-1 listing + team-resolve overhead only, no discussion round trip at all).
  - Warm, 1 changed (captured in `/tmp/mrboard.log` from an earlier session's TUI boot today):
    `phase-2 batch discussions done: changed=1, duration=256ms`, `total_duration: 678ms` — matches
    the ADR's predicted steady state (~0.6s, phase 2 ≈ 400ms for 1-3 changed MRs) almost exactly.
- **Learnings:**
  - `mrboard fetch` (the CLI JSON dump) is a convenient, already-correct cold-baseline probe since
    `execFetch` in `internal/cmd/mrboard/fetch.go` always builds `mrsvc.FetchOptions{}` with a nil
    `Previous` — no extra flag or code path needed to exercise the "full fetch" branch.
  - The TUI's very first `Init()` fetch (`internal/tui/model.go` `makeFetchCmd`) already passes
    `m.allMRs` (loaded from `SnapshotStore` before `Init` runs, per `.6`) as `Previous` — so a plain
    `agent-tui` boot against a warm cache is a legitimate, zero-setup warm-path timing probe; no
    synthetic benchmark test was needed to satisfy the "measured, warm and cold" acceptance line.
---

