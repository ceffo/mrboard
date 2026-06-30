# Changelog

## [0.7.0] - 2026-06-30

### Added
- JIRA Remote Issue Links: mrboard now writes a backlink on each JIRA issue pointing to its GitLab MR, complementing the existing MR-to-JIRA link. Links carry a descriptive title and a GitLab favicon icon, and writes are logged at Info level for visibility.
- Remote link icon URL is configurable via `jira.remote_link_icon_url` in `mrboard.yaml` instead of being hardcoded.

### Changed
- Remote link writes use a 3-layer dedup strategy (session cache, disk cache, GET-before-write) to avoid redundant JIRA API calls when the link content hasn't changed.

## [0.6.0] - 2026-06-30

### Added
- JIRA issue line on cards: each card now shows a dedicated third line with an issue-type icon and the JIRA key (e.g. `🐛 OD-3345`). Icons are fetched asynchronously in the background with a 🎫 placeholder while loading, backed by a 24-hour disk cache to minimise API traffic. Icon-to-type mapping is configurable per issue type in `mrboard.yaml`.
- Auto-inject JIRA backlink into MR descriptions: when fetching MRs, the adapter appends a JIRA link to the MR body if the `<!-- mrboard -->` marker is absent — keeping GitLab MR descriptions in sync with their linked tickets automatically.
- Sprint filter (`S` key): toggles the board to show only MRs whose JIRA issue is part of the active sprint. The board header displays a sprint indicator when the filter is active. Requires `jira.board_id` in config.
- Batch reviewer editor (`E` key): opens a full-screen editor pre-filled from the focused card's current reviewers, listing all sibling MRs under the same JIRA ticket. A preview diff screen shows per-MR reviewer changes before committing; writes are skipped for MRs where nothing changed, making the operation idempotent.

### Fixed
- JIRA icon rendering: replaced ambiguous-width emoji (e.g. `☑️`) with terminal-safe alternatives to eliminate one-column layout gaps caused by width-detection mismatches between the renderer and terminal emulator.
- Eliminated a TOCTOU race and redundant GitLab API calls in the background JIRA description injection path.

## [0.5.0] - 2026-06-17

### Added
- New unified reviewer editor overlay: a full-screen modal that replaces the former approver editor, allowing reviewers to be added or removed inline without leaving the board.
- Team username resolution at startup — the roster is fetched once and cached, so reviewer names are always shown as human-readable handles rather than raw user IDs.
- `User` domain type with dedicated reviewer ports and corresponding GitLab client methods, giving the reviewer workflow a clean domain model separate from MR state.

### Changed
- Remapped `r`/`v` keys and redesigned the reviewer editor title bar for a clearer, more consistent keyboard affordance.

## [0.4.5] - 2026-06-09

### Fixed
- Successive toast notifications now each display for their full configured duration; previously the timer began at enqueue time rather than display time, so back-to-back notifications would expire prematurely.

### Changed
- Refactored internal architecture: extracted overlay state machine, GitLab client sub-interfaces (MRLister/MREnricher/MRWriter), fetch pipeline stages, and domain event types into dedicated packages.

## [0.4.4] - 2026-06-09

### Changed
- Redesigned card and detail pane layout for improved readability.

## [0.4.3] - 2026-06-05

### Fixed
- Settings panel navigation: `hjkl` now moves within a section; `h`/`l` switches between sections. Tab is tab-only again. Age sort direction corrected.

## [0.4.2] - 2026-06-05

### Added
- MR author name is now included in Teams notification cards.

### Changed
- Replaced the internal BubbleUp toast implementation with `ceffo/toast` for alert overlay rendering.

## [0.4.1] - 2026-06-04

### Fixed
- All team MRs are now shown regardless of whether the current user is assigned as a reviewer.

## [0.4.0] - 2026-06-03

### Added
- Teams notifications via Power Automate webhook — press `n` on a focused card to notify the reviewer/approver.
- Jira integration — press `J` on a card to open the linked Jira ticket in the browser.
- Toast alert overlay for in-app feedback (approver save confirmation, notification status).
- Approver saves now automatically fire a Teams notification when a webhook is configured.

## [0.3.4] - 2026-05-28

### Added
- New unified settings panel (press `,`) with four tabs: General, Filters, Sorting, and Theme — replaces the separate filter popup and theme picker overlays.
- Reviewer MRs are now fetched lazily once per session when "include reviewer MRs" is enabled; filter and reviewer preferences are persisted across restarts.

### Fixed
- All reviewers are shown on cards; reviewer MRs are no longer re-fetched on every refresh.

## [0.3.3] - 2026-05-27

### Added
- Full-screen diff view: press `d` on any card to open a diff rendered by `difft` (side-by-side ≥ 180 cols, inline otherwise) with `go-gitdiff` fallback. Navigate files with `n`/`p`, scroll with `j`/`k`/`ctrl-d`/`ctrl-u`, jump to top/bottom with `g`/`G`. Files are fetched lazily and cached per session.

## [0.3.2] - 2026-05-27

### Changed
- Added structured info-level logging to `FetchMR`, `GetProjectMembers`, `SaveApprovers`, and `GetDetail` for improved observability.

## [0.3.1] - 2026-05-27

### Added
- The "Ready to Merge" column is renamed "Approved"; the MR title is now coloured by merge-readiness (green when mergeable).
- Reviewer pills restyled: neutral brackets, unified `@` colour, designated approvers are always shown.

### Fixed
- GitLab GraphQL `detailedMergeStatus` is now normalised to lowercase to match REST API values.
- Fixed wrong GraphQL field (`approvalRules` → `approvalState.rules`) that caused approval data to be missing.
- Fixed a race condition in the GQL approval-rules fallback that silently dropped parallel user fetches.

## [0.3.0] - 2026-05-27

### Added
- Approver editor overlay: press `a` on a focused card to assign approvers from project members. Changes are saved back to GitLab and the card re-fetches immediately.
- Approval state is now displayed on cards and in the detail pane using a dedicated colour token.
- GitLab adapter fetches approval rules and `detailedMergeStatus` via GraphQL, and supports writing approval rules.

### Fixed
- `SaveApprovers` was hardcoding `approvals_required` to 1 regardless of how many users were selected.
- Approver editor correctly resolves user IDs before saving.

### Changed
- `ApprovalCount`/`RequiredApprovals` fields removed from domain; replaced with per-reviewer `IsApprover` flag.

## [0.2.6] - 2026-05-25

### Changed
- Refactored internal card rendering (separated measure from render pass), introduced `FilterCriteria`, `MRDeduplicator`, and `domain.AppState` to consolidate state and filtering logic.

## [0.2.5] - 2026-05-21

### Added
- Cards in the "Ready to Merge" column now show how long ago the MR was fully approved.

## [0.2.4] - 2026-05-20

### Fixed
- Resolved threads no longer contribute to reviewer state derivation, preventing false "needs attention" signals.

## [0.2.3] - 2026-05-20

### Fixed
- Focused card background is now restricted to title lines only.
- Detail pane scroll offset is clamped correctly.
- Theme propagation and focus refresh corrected; MR lifetime thresholds applied.

## [0.2.2] - 2026-05-15

### Changed
- GitLab data is now fetched per user via GraphQL and sources are fetched in parallel, significantly reducing load time on large teams.
- A centred spinner overlay is shown during background refresh.

### Fixed
- Header shows only the total MR count (removed duplicate count display).
- Logger is wired throughout the fetch pipeline so the log file contains meaningful output at info level.

## [0.2.1] - 2026-05-15

### Added
- Current user is always visible in the filter popup; auto-pin removed.
- Filter popup uses theme colours.

### Fixed
- Header background now spans the full terminal width; columns fill available width.
- Header MR counts now match the number of cards actually displayed.

### Changed
- Source config shape updated: `ids` list with explicit `SourceType` enum replaces the previous mixed format.

## [0.2.0] - 2026-05-14

### Added
- Live theme picker overlay (`t`) with state persistence and `--theme`/`--mode` CLI flags.
- Semantic colour system with five bundled themes (supported: light and dark mode variants).
- `--config` flag and `$MRBOARD_CONFIG` environment variable for explicit config path.
- Composition root (`internal/core`) wires config, adapters, and stores; signal handling and root context propagation added.

## [0.1.3] - 2026-05-12

### Changed
- CLI commands slimmed to a four-step pattern; CLI wiring moved to `internal/cmd/mrboard` following clean-architecture boundaries.

## [0.1.2] - 2026-05-12

### Added
- Shell completions (bash, zsh, fish) packaged in the Homebrew cask.
- CLI entrypoint migrated to Cobra with a proper command hierarchy.

## [0.1.1] - 2026-05-12

### Changed
- Distribution switched to Homebrew cask for correct CLI binary installation.

## [0.1.0] - 2026-05-12

### Changed
- Release workflow now prompts for confirmation before tagging and pushing.

## [0.0.5] - 2026-05-12

### Fixed
- Homebrew formula written to the correct `Formula/` directory per tap conventions.

## [0.0.4] - 2026-05-12

### Fixed
- Reverted to `brews` stanza for a proper CLI formula (not a cask).

## [0.0.3] - 2026-05-12

### Fixed
- GoReleaser Homebrew schema: use `binaries` field, drop `install`/`test` blocks.

## [0.0.2] - 2026-05-12

### Added
- Kanban board TUI displaying GitLab MR review status across four phases: In Review, Changes Requested, Approved, Ready to Merge.
- Per-lane scrolling layout with header (live stats) and docked footer (keybinding bar).
- Card layout: author, age, stale indicator, reviewer pills with approval state.
- MR detail panel: description rendered with Glamour, discussion threads, `!IID` reference links.
- Filter popup: filter by phase, author, and reviewer; filter state persists across restarts.
- Sort cycling and "my view" toggle (show only MRs relevant to the current user).
- Reviewer state machine: derives pending/commented/approved/changes-requested per reviewer from discussion events.
- MR phase classification based on reviewer states and approval requirements.
- Archived GitLab projects are automatically excluded.
- GitLab API client with group MR listing, user MR listing, and discussion fetching.
- Config loading from `mrboard.yaml` / `$MRBOARD_CONFIG` with XDG search path; PAT overridable via `$GITLAB_TOKEN`.
- File logging with configurable path; `mrboard fetch` subcommand for headless data inspection.
- Homebrew distribution via GoReleaser.
