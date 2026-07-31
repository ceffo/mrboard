# Ralph Progress Log

This file tracks progress across iterations. Agents update this file
after each iteration and it's included in prompts for context.

## Codebase Patterns (Study These First)

- **Before implementing a bead, verify it isn't already done.** Prior ralph-tui iterations sometimes
  bundle a later bead's work into an earlier bead's commit message (e.g. `git log -S "<phrase from
  the bead's scope>" -- <package>` against the actual acceptance criteria, not just `git status`).
  A bead can show `in_progress` in `br show` and have zero uncommitted diffs yet be fully
  implemented and committed under a different bead's commit — read the real code against every
  acceptance-criteria line before writing anything.
- **agent-tui gate against a warm snapshot cache**: `internal/adapters/snapshotstore.JSONStore`
  writes `{XDG_CACHE_HOME|~/.cache}/mrboard/snapshot.json` as `{version, written_at, mrs}`.
  `Save()` always stamps `written_at` with `time.Now()`, so to test an *aged* label you must hand-edit
  the JSON's `written_at` after seeding (a small `go run` scratch tool under a temp `tools/` dir,
  using the real `snapshotstore`/`domain` packages, is easier than hand-crafting `domain.MergeRequest`
  JSON — internal-package import rules mean the tool must live inside this module tree). Point
  `mrboard.yaml`'s `gitlab.url` at a black-hole IP (e.g. `192.0.2.1`, RFC 5737 TEST-NET-1) with a
  short `gitlab.timeout` (e.g. `8s`) instead of an `.invalid` domain — DNS failure on `.invalid` is
  near-instant and the fetch-failure swap overwrites the seeded cache before you can screenshot the
  warm-boot state; a black-hole IP gives a real multi-second window to navigate mid-fetch.
  **Always restore/delete the real `~/.cache/mrboard/snapshot.json`** afterward — it's the user's
  actual daily-standup cache, not repo state, and `mrboard.yaml`/scratch tools must be deleted too
  (the former is gitignored but must not linger to accidentally break a future real launch).
- A totally failed fetch (network unreachable, all sources error) currently replaces `m.allMRs`
  with the empty/nil result — the board goes blank rather than keeping the last-known snapshot on
  screen. This is a pre-existing behavior unrelated to non-blocking refresh; noted here in case a
  future bead wants "never blank the board on total failure" as explicit scope.
- **agent-tui gate config-precedence trap — CRITICAL**: viper's search order is `--config` flag >
  `$XDG_CONFIG_HOME/mrboard/mrboard.yaml` (default `~/.config/mrboard/mrboard.yaml`) > `./mrboard.yaml`.
  If a real user config already exists at `~/.config/mrboard/mrboard.yaml` (it does, on this machine,
  with a live GitLab PAT and JIRA token for `gl.nsesi.io`), it silently wins over a scratch
  project-root `./mrboard.yaml` meant to point at a black-hole IP — the scratch black-hole-IP trick
  from the `.6` entry above does **not** actually isolate the TUI from the real GitLab instance unless
  you launch with an explicit `--config <abs-path-to-scratch-yaml>`. Discovered when a "Team
  resolution failed" toast showed a real `https://gl.nsesi.io/...` URL instead of the intended
  `192.0.2.1`, meaning a live agent-tui gate could have fired a real write (e.g. `SaveApprovers`)
  against production GitLab data. **Always launch the scratch config via a temp wrapper script that
  execs `./bin/mrboard --config <absolute path>`** (not the shared `scripts/run-tui.sh`, which takes
  no args) — never rely on `./mrboard.yaml` cwd-resolution alone when a real XDG config may exist.
  Before pressing any key that could trigger a real write (Enter/save in an editor overlay), verify
  the error/toast text references the intended fake target, not a real domain.
- **Live reproduction of a network-timing race is often not worth attempting.** For
  `mrr-incremental-fetch-3sl.7` (dirty-set guard), the exact "stale snapshot lands after a local
  write" race is deterministically covered by mocked-clock unit tests (`dirty_test.go`) that control
  `FetchStartedAt`/`writeAt` precisely — reproducing the same race live would require either a real
  GitLab write (unsafe/unauthorized against production data) or a protocol-accurate local mock GitLab
  server (REST + GraphQL, disproportionate effort for a race the unit tests already nail exactly).
  The proportionate agent-tui gate in that situation: prove the *reachable* live surface (editor
  opens and stays interactive during an in-flight refresh, a failed write closes the overlay
  gracefully with a toast, no crash) and lean on the unit tests for the timing-precise assertion —
  document the scoping decision explicitly rather than silently skipping or fabricating the live
  race.

---


## 2026-07-31 - mrr-incremental-fetch-3sl.6
- Verified this bead's full scope was already implemented and committed (bundled into commit
  `9491963`, labeled as .4's phase-2 commit): `baseStack()`/`handleKey()` in `internal/tui/model.go`
  no longer gate on `isRefreshing` (only on `m.state != stateBoard`); `New()` boots straight into
  `stateBoard` from `snapStore.Load()` at any age, leaving `stateLoading` only for a genuinely empty
  cache; `renderBoard()` calls `header.SetSnapshotAge(m.snapshotWrittenAt, m.isRefreshing,
  spinnerFrame)` instead of drawing a full-screen spinner overlay; `saveSnapshot()` runs on every
  landed `FetchResultMsg`; `handleRefreshTick` does not check overlay/detail state before firing.
  `internal/tui/refresh_test.go` already had full unit coverage for every acceptance-criteria line
  (`TestModel_BaseStack_IncludesBoardWhileRefreshing`, `TestModel_HandleKey_MovesSelectionWhileRefreshing`,
  `TestNew_ColdCache_StaysInLoadingState`, `TestNew_WarmCache_BootsInteractiveAtAnyAge`,
  `TestModel_FetchResultMsg_SavesSnapshot`).
- No code changes made — this session's only artifacts were scratch (a temporary
  `tools/seed_snapshot` go program, a temporary `mrboard.yaml` pointed at `192.0.2.1`), all deleted
  before finishing; the real `~/.cache/mrboard/snapshot.json` was restored to its prior (empty)
  contents.
- Ran `just check` (clean) and the mandatory agent-tui gate: seeded a warm 3-MR snapshot backdated
  to 14 minutes, launched via `scripts/run-tui.sh`, screenshotted immediately (board rendered, header
  `⣯ 14m ago`, no spinner overlay), pressed `ArrowRight` + `Enter` while the background fetch was
  still in flight and confirmed the detail panel opened on the newly-selected MR (proves keybindings
  work mid-refresh), then waited for the fetch to land and confirmed the header flipped to `just now`.
- **Learnings:** see the new "Codebase Patterns" entries above (verify-before-implementing via
  `git log -S`, seeding a warm snapshot for agent-tui, and the pre-existing total-failure-blanks-board
  behavior that's out of scope here).
---

## 2026-07-31 - mrr-incremental-fetch-3sl.7
- Verified this bead's full scope was already implemented and committed (bundled into commit
  `9491963`, same commit that bundled `.5`/`.6`/`.8` work under the `.4` label). `Model.dirty
  map[domain.MRKey]time.Time` (model.go:292) is populated by `handleReviewersSaved` (covers both
  plain reviewer edits and `SaveApprovers`/`SetReviewers`, since both funnel through
  `ReviewersSavedMsg` via `mrsvc.ApplyReviewerChanges`) and by `handleTicketDescriptionLinkResult`.
  `startFetch()` forces every dirty key stale via `FetchOptions.ForceStale` so the next fetch's
  phase-2 pass re-fetches it fresh. `applyFetchResult()` keeps the local entry in place for any dirty
  key whose write happened after the landing fetch started (including keys the landing snapshot
  dropped entirely), and `clearResolvedDirty()` drops entries confirmed by a later-started fetch.
  `internal/tui/dirty_test.go` already had the exact two tests the acceptance criteria describe
  (`TestFetchResultMsg_DirtyGuard_PreservesLocalWriteAndIssuesTargetedRefetch`,
  `TestFetchResultMsg_DirtyGuard_ClearsWhenConfirmingFetchLands`), both driving `Model.Update` with
  hand-stamped `FetchResultMsg.FetchStartedAt` values via mocked `FetchAll`.
- No code changes made. Scratch artifacts this session: `tools/seed_dirty_snapshot/main.go` (seeded
  one warm MR with a reviewer via the real `snapshotstore`/`domain` packages), a scratch
  `./mrboard.yaml` pointed at black-hole IP `192.0.2.1`, and a temp `/tmp/mrboard-gate-run.sh`
  wrapper — all deleted before finishing; `~/.cache/mrboard/snapshot.json` restored to its prior
  (empty) contents.
- Ran `just check` (clean) and an agent-tui gate: seeded the warm MR, launched with an explicit
  `--config` pointing at the scratch black-hole-IP config (see the new "Codebase Patterns" entry
  above — the default `./mrboard.yaml` cwd-resolution is NOT safe here because a real
  `~/.config/mrboard/mrboard.yaml` with live GitLab/JIRA credentials exists and wins by viper's
  search order), pressed `r` to start a refresh, immediately pressed `v` to open the reviewer editor
  (proved the editor stays reachable mid-refresh), toggled an approver with `space`, and pressed
  `Enter` to save. The write timed out against the black-hole IP (as expected/safe) and the overlay
  closed gracefully with no crash. Did not attempt to reproduce the exact millisecond-timed
  stale-landing race live (see the new "Codebase Patterns" entry on why that's disproportionate here)
  — that assertion is owned by `dirty_test.go`, which passed under `just check`.
- **Learnings:** see the two new "Codebase Patterns" entries above — the config-precedence trap is
  the important one; it could otherwise cause a scratch agent-tui gate to silently fire a real write
  against production GitLab data.
---

## 2026-07-31 - mrr-incremental-fetch-3sl.8
- Verified already implemented (same "bundled under an earlier bead's commit" pattern as .3/.4/.5,
  bundled into commit 9491963, labeled as .4's phase-2 commit). No code changes made this session.
- Confirmed against every acceptance-criteria line: `config.AppConfig.RefreshInterval time.Duration`
  (`internal/config/config.go`, `mapstructure:"refresh_interval"`, `v.SetDefault("refresh_interval",
  "60s")`), `refreshTickCmd`/`handleRefreshTick`/`refreshGen` in `internal/tui/model.go` (skip-in-
  flight via `isRefreshing` check, generation counter bumped by manual `r` so a stale scheduled tick
  is dropped instead of acting), `mrboard.yaml.example:22` documents the key, and
  `internal/tui/refresh_test.go` + `internal/config/config_test.go` already cover every acceptance
  line (zero-disables, skip-while-in-flight, stale-generation-dropped, unchanged-data-causes-no-
  mutation-or-selection-change).
- Ran `just check` clean (fmt/lint/build/test all pass). No agent-tui gate needed — no TUI-visible
  change was made (nothing to screenshot), consistent with the ticket being a no-op this session.
- **Learnings:** this is the fourth bead in the epic found fully pre-implemented and bundled into an
  earlier commit (after .3, .4, .5) — reinforces the top "Codebase Patterns" entry: always read the
  actual code and tests against the acceptance criteria before writing anything, especially when
  `br show` reports `in_progress`.
---
