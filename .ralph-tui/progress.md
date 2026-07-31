# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

*Add reusable patterns discovered during development here.*

- A ralph-tui bead can arrive already substantially (or fully) implemented in the working tree —
  `git status`/`git diff --stat` before reading the bead's acceptance criteria in detail. Read every
  diff against the acceptance list line by line rather than assuming "modified files exist" means
  "done"; the gaps that remain are usually narrow (a lint regression, a missing measurement) rather
  than missing features.
- `just check`'s `golangci-lint` `goconst` check operates on total literal-string occurrences across
  an entire package (all files, `_test.go` included), not per-file. Adding a new test file that
  reuses an existing fixture string (a test username, a status literal) can push a *pre-existing,
  untouched* file over the threshold and fail lint on a line you never edited. Fix by hoisting shared
  literals (test usernames, status strings that already have a package const like
  `detailedMergeStatusMergeable`) into named constants and reusing them across all files in the
  package — not just the new one. Verify by stashing your changes (including untracked files — move
  them aside with `mv`, plain `git stash` skips untracked) and re-running lint on the clean tree to
  confirm the failures are actually new.
- For a "measure end-to-end timing against real GitLab" acceptance criterion with no CI-safe way to
  hit production: write a throwaway same-package `_test.go` gated behind an env var
  (`if os.Getenv("X") == "" { t.Skip(...) }`) that loads the real `~/.config/<app>/<config>.yaml` and
  builds the real client, run it once with `go test -run <name> -count=1 -v` to get numbers, record
  the numbers in the ADR, then delete the file — don't leave a live-credentials-dependent test
  sitting in the repo even if it's skipped by default.

---

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
  - A ralph-tui bead's code can be fully implemented AND committed, yet `br` still reports it
    `in_progress` — the commit landed under a *different* ticket's auto-commit message (e.g. an
    `-A`-style add swept up several tickets' work into one `feat: ...3sl.2` commit that also
    contains `.3`'s and `.5`'s files). `git log --oneline -- <file>` / `git show --stat <sha>`
    settles this fast: if the files are already committed and `git status` is clean, don't
    re-implement — verify against acceptance criteria, run `just check`, and just close the bead.

---

## [2026-07-31] - mrr-incremental-fetch-3sl.3
- Verified (no new implementation needed): thin GraphQL queries `gqlUserMRsThinQuery` /
  `gqlReviewerMRsThinQuery` in `pkg/gitlab/graphql.go` (fat query minus `discussions`, keeps
  `approvalState.rules`, adds `resolvedDiscussionsCount`/`resolvableDiscussionsCount`); dedup
  moved before enrichment in `internal/adapters/gitlabadpt/gitlabadpt.go` (`listStage` ->
  `dedupStage` -> `enrichStage`); `TestFetchAll_DedupBeforeEnrichment` in `gitlabadpt_test.go`
  asserts a duplicate-source MR is enriched exactly once. ADR `docs/adr/0005` section "Phase-1
  thin query: implementation findings" records both open questions resolved: the two
  discussion-count scalars exist and are non-deprecated, and `approvalState` measured 579ms–1.3s
  either way (no systematic cost) — phase-1 wall-clock ~0.6–1.3s vs the 2–9s fat baseline.
- Files touched this session: none (verification only) — all code was already committed (see
  learning above) under commit `c256149`. Closed the bead and flushed beads state.
- **Learnings:** see the reusable pattern added at the top of this file about auto-commits
  sweeping multiple tickets' work into one commit message.
---

## [2026-07-31] - mrr-incremental-fetch-3sl.4
- Found the implementation already present and substantially complete in the working tree (not
  committed): `internal/adapters/gitlabadpt/phase2.go` (`diffGQLStage`, `enrichGQLMRsBatch`),
  `mapper.go`'s `MergeMRFromGraphQL` (the cache-hit merge rule: `Reviewers`/`OpenThreads`/
  `RoundTripCount` from cache, everything else — including `DetailedMergeStatus` — always fresh from
  phase 1, then `ClassifyPhase`/`DeriveWaitingSince`/`deriveReadyToMergeSince` re-run), `mrsvc.
  FetchOptions.ForceStale []domain.MRKey`, and `pkg/gitlab`'s aliased `FetchMRsDiscussionsGraphQL`
  (`buildAliasedMRDiscussionsQuery`, one `mrN: project(fullPath:$pN){mergeRequest(iid:$iN){...}}`
  block per MR, one round trip for N MRs). Test coverage for every acceptance item already existed:
  `merge_test.go` (cache-hit reuse, always-fresh `DetailedMergeStatus`/`Draft`/`IsApprover`, no
  cached-slice mutation), `gitlabadpt_test.go` (`TestFetchAll_Phase2SkipsUnchangedMRs` — zero
  requests when nothing changed; `TestFetchAll_NilPrevious_TreatsEveryMRAsChanged`; `ForceStale`;
  batch-error-per-MR), `pkg/gitlab/graphql_aliased_test.go` (request shape, response alignment,
  not-found, GraphQL error).
- What was actually missing: `just check` failed on `golangci-lint`'s `goconst` (see the reusable
  pattern above — added earlier in this session) plus one `mnd` (magic number `2` in
  `graphql.go`'s `variables` map sizing → named `varsPerMR` const) and two `revive` unused-`r`-param
  hits in the new `graphql_aliased_test.go` httptest handlers. Fixed all of those. The other missing
  piece — "measured end-to-end timing recorded, warm and cold" — had never been done: added a
  throwaway env-var-gated test (see the reusable pattern above), ran it twice against the real
  `~/.config/mrboard/mrboard.yaml` (`gl.nsesi.io`, 5 user sources, 11 MRs), got cold 3.80s/4.10s and
  warm 651ms/667ms, recorded both in `docs/adr/0005` under a new "Phase-2 aliased query: measured
  timing" subsection, then deleted the test file.
- Files touched this session: `docs/adr/0005-incremental-fetch-and-selection-identity.md` (new
  timing subsection), `pkg/gitlab/graphql.go` (`varsPerMR` const), `pkg/gitlab/graphql_aliased_test.go`
  (unused-param fix), `internal/adapters/gitlabadpt/mapper_test.go` (added shared `testUserAlice`/
  `testUserAliceName`/`testUserBob`/`testUserBobName`/`testUserPriya` consts, replaced literals),
  `internal/adapters/gitlabadpt/gitlabadpt_test.go` (same consts; dropped `gqlMR`'s unused `author`
  param — every call site passed `"priya"`). No production-logic changes — the implementation itself
  needed nothing.
- **Learnings:** the two reusable patterns added at the top of this file this session (goconst is
  package-wide; the throwaway-env-gated-test approach for a real-credentials timing measurement).
---

## [2026-07-31] - mrr-incremental-fetch-3sl.5
- Found the implementation already present and complete in the working tree (not committed): same
  uncommitted-work-survives-across-sessions pattern as `.3`/`.4`. `Model.selected domain.MRKey`
  (`model.go:254`) is the single source of truth; `prevFocusMR`/`TryRestoreFocus` are gone entirely.
  `boardWidget.SetMRs(mrs []domain.MergeRequest, selected domain.MRKey) domain.MRKey` (`board.go:98`)
  resolves focus by scanning for `selected` across all columns (follows phase changes), falling back
  to the same column/row-index clamped to the new length, then to the first non-empty column if that
  column is now empty — and returns the key of wherever it landed. All 8 `applyMRFilter()` call sites
  funnel through one `board.SetMRs` call (`model.go:1506`), so they're fixed by construction as the
  scope demanded. Arrow-key handlers (`Up`/`Down`/`Left`/`Right`) call `m.updateTicketKey()` after
  every `board.Move*()`, which writes `m.selected = mr.Key()` — the write-back path. Test coverage in
  `board_test.go` already covered every acceptance item: same-IID-different-project no-collision,
  follows-across-columns, absent-MR-same-index fallback, empty-column-falls-back-to-first-non-empty.
- Files touched this session: none (verification only) — closed the bead and flushed beads state.
- agent-tui gate: launched the TUI, selected `!534 boris-od-workflow` mid-column (Draft), opened the
  detail panel (Enter) to read its identity as text since focus highlighting is color-only (lipgloss
  `CardFocusedBg`/`CardFocused` styles) and this agent-tui build's screenshot capture carries no ANSI
  bytes at all (verified with `od -c` / `grep -c $'\x1b'` — zero escape sequences, both `text` and
  `json` formats). Pressed `s` to re-sort (order visibly reversed, sort indicator flipped ↑→↓) and
  reopened detail: still `!534`. Toggled view (`Tab`) to the `@moncef`-only filter: `!534` isn't
  authored/reviewed by moncef, so it legitimately disappeared and Draft went to 0 cards — this
  exercised the empty-column fallback, and detail confirmed focus landed on `!270` (first non-empty
  column, Needs Review). Toggled back (`Tab`): screenshot showed `Total:7`/`Draft (3)` immediately
  restored with no navigation needed, and detail confirmed focus was still `!270` — selection survived
  the round trip, matching the same key both times.
- **Learnings:**
  - This agent-tui installation's `screenshot` (both `text` and `json` format) strips all ANSI/color
    entirely — confirmed with `od -c` and `grep -c $'\x1b'` on the raw output (zero escape bytes).
    Since this TUI signals focus/selection purely via lipgloss background/border color with no
    fallback text glyph, plain screenshots cannot show which card is focused. Workaround: open the
    detail panel (`Enter`) — it renders the focused MR's identity (title, IID, project path) as text,
    giving a color-independent way to confirm selection identity across an action. Use this whenever
    an agent-tui gate needs to prove "the same item is still selected" and the widget's focus
    indicator is color-only.
  - `boardWidget.MoveRight()`/`MoveLeft()` (pre-existing, not part of this bead) unconditionally
    advance to the adjacent column index even if it has zero cards — landing focus on an empty
    column, where `FocusedMR()` returns `nil` and any focus-dependent keybinding (e.g. `Detail`)
    silently no-ops. Don't mistake this for a broken keybinding when driving the board with arrow
    keys during agent-tui testing near columns with 0 items; move one more step to reach a
    non-empty column instead.
---

