# ADR-0010: Self-Update Check

**Status**: Accepted

## Context

mrboard is distributed as a Homebrew cask (`ceffo/tap/mrboard`), built and published by
goreleaser on every `v*.*.*` tag (`.goreleaser.yml`, `.github/workflows/release.yml`), which
creates a real GitHub Release alongside the cask update. Nothing today tells a user running an
older build that a newer one exists — they only find out by remembering to run
`brew upgrade` themselves.

The destination: a small `↑` badge next to the version number in the footer when a newer release
exists, and a `u` keybinding (enabled only while a badge is showing) that opens a Yes/No
confirmation modal offering to run `brew update && brew upgrade ceffo/tap/mrboard` itself.

While designing this, "update" was found to already name something unrelated in this codebase —
`mrboard update` was the CLI subcommand for auto-assigning reviewers (ADR-0009). That command was
renamed to `mrboard auto` in the same change, freeing "update" for this feature.

## Non-goals

- No general-purpose GitHub integration. The check target (`ceffo/mrboard`) is a fixed constant,
  not a user-configurable repo — this is mrboard checking its own upstream, not a GitHub client
  feature.
- No update mechanism other than Homebrew. mrboard does not attempt to detect how it was
  installed; the offered command assumes the documented install path.
- The check never runs for a `dev` build (one not built via goreleaser), regardless of
  `update_check.enabled` — a local development build has no meaningful "latest release" to compare
  against, and nagging a developer mid-session would be actively unhelpful.

## Decision

**Version source.** `GET /repos/ceffo/mrboard/releases/latest` (`pkg/github`), unauthenticated —
goreleaser already guarantees a GitHub Release exists per tag, so it's authoritative regardless of
how the user installed mrboard. Comparison is a hand-rolled `major.minor.patch` parse
(`updatesvc.ParseVersion`/`IsNewer`) rather than a semver dependency: the tag scheme is guaranteed
simple (`v*.*.*` only, no prerelease suffixes), and a parse failure — including the running
`"dev"` version — is treated as "can't safely compare," never as "always/never newer."

**Check cadence.** One-shot per launch, fired from `Model.Init()`, not the existing
`RefreshInterval` ticker — that ticker exists for MR-data staleness, an unrelated cadence concern.
`githubadpt` layers its own 24h disk cache (mirroring `jiraadpt`'s cache shape) on top, so the
one-shot-per-launch call still amounts to roughly one real GitHub request per day across however
many times mrboard is started, comfortably inside GitHub's unauthenticated rate limit.

**Modal is a real confirmation, not a two-step reveal.** The modal shows the current/latest
version and the exact command, and a single keypress decides the outcome: Enter/y runs it, Esc/n
dismisses. There is no separate "show the command" state requiring a second key to actually run
it.

**The confirm handler shells out, unlike the custom-command launcher.** ADR-0004's external
command launcher deliberately never passes a shell — its argv is templated from user config, and
a shell there would let a config-supplied argument smuggle in shell metacharacters. This command
has no template expansion and no external input at all: `brew update && brew upgrade
ceffo/tap/mrboard` is a fixed string baked into mrboard itself, identical on every machine. A shell
(`exec.Command("sh", "-c", ...)`) is actually required here, to get `&&` sequencing between the two
logical brew invocations — `exec.Command` alone cannot express that.

**No timeout on the run.** Matches the existing custom-command precedent (`execCommandCmd` is also
unbounded): `brew upgrade` can legitimately take minutes, and killing it partway through would
leave Homebrew in a worse state than waiting.

**Success still toasts, unlike a configured command.** `handleCommandResult` shows no toast on
success because the resumed, redrawn board is itself the success signal. That doesn't hold here:
the mrboard process still in memory is the *old* binary even after `brew upgrade` finishes on disk.
The success toast says so explicitly ("updated — restart mrboard to use the new version") rather
than implying the running session is now current.

**Config.** `update_check.enabled` (default `true`) and `update_check.cache_ttl` (default `24h`),
following the `Jira`/`AutoAssignReviewers` struct-per-feature convention. No `owner`/`repo` keys —
the target repository isn't user-configurable (see Non-goals).

**Demo mode never checks for real**, regardless of `update_check.enabled`: `demoadpt.UpdateChecker`
always reports no update available, the same "every driven port has a fake here" invariant already
covering `Notifier`.

## Consequences

- `internal/domain/service/updatesvc` and `internal/adapters/githubadpt` are new vendor-neutral
  port/adapter packages, following the same shape as `ticketsvc`/`jiraadpt`.
- `pkg/github` is a new raw API client package, alongside `pkg/gitlab`/`pkg/jira`.
- `tui.New`'s signature grows a `updatesvc.UpdateChecker` parameter; every call site (production
  and tests) passes it explicitly.
- Verifying the actual confirm-and-run path cannot be automated or driven through `agent-tui`: it
  runs a real `brew update && brew upgrade` against whichever machine executes it. Automated tests
  cover command *construction* only (`newSelfUpdateExecCmd`); the real end-to-end run is a
  deliberate, manually-triggered, one-time check by a human, not a scripted step.
- `mrboard update` (auto-assign reviewers) is renamed to `mrboard auto` in the same change — a
  breaking CLI change, called out in `CHANGELOG.md`, not left silently undocumented.
