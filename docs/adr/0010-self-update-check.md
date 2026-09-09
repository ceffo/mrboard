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
confirmation dialog offering to run `brew update && brew upgrade ceffo/tap/mrboard` itself. The
badge carries the `u` hint beside it, since a badge nobody knows how to act on is only half a
feature.

While designing this, "update" was found to already name something unrelated in this codebase —
`mrboard update` was the CLI subcommand for auto-assigning reviewers (ADR-0009). That command was
renamed to `mrboard auto` in the same change, freeing "update" for this feature.

## Non-goals

- No general-purpose GitHub integration. The check target (`ceffo/mrboard`) is a fixed constant,
  not a user-configurable repo — this is mrboard checking its own upstream, not a GitHub client
  feature.
- No update mechanism other than Homebrew. mrboard does not attempt to detect how it was
  installed; the offered command assumes the documented install path.
- The check never runs for a build that isn't a published release, regardless of
  `update_check.enabled` — a local build has no meaningful "latest release" to compare against, and
  nagging a developer mid-session would be actively unhelpful. That covers both `dev` and the
  `git describe` version `just build` stamps (`v0.11.0-3-gabc1234-dirty`): goreleaser publishes
  only plain `vX.Y.Z`, so any suffix means locally built.

## Decision

**Version source.** `GET /repos/ceffo/mrboard/releases/latest` (`pkg/github`), unauthenticated —
goreleaser already guarantees a GitHub Release exists per tag, so it's authoritative regardless of
how the user installed mrboard. Comparison uses `github.com/Masterminds/semver/v3`, in
`githubadpt`, not a hand-rolled parse: version precedence has enough edge cases (prerelease
ranking especially) that owning the implementation buys nothing. It sits in the adapter rather
than the port because `internal/domain` — `updatesvc` included — takes no non-stdlib
dependencies; the port declares only that an implementation must never report an update available
for a version it cannot parse, which is what the running `"dev"` build relies on.

**Check cadence.** A forced live check on launch from the version widget's `Init()`, then a
recurring check every `update_check.cache_ttl`. Launch forces past the cache because a cache entry
written by a previous run describes what was true then, and launch is exactly when a stale or
missing badge is most visible. The recurring cadence matches the cache TTL deliberately: a shorter
period would only ever be answered from that cache, and a longer one would leave the cache expired
between checks. It is separate from the `RefreshInterval` ticker, which exists for MR-data
staleness — an unrelated concern.

**Dialog is a real confirmation, not a two-step reveal.** It shows the current/latest version and
the exact command, and a single keypress decides the outcome: Enter/y runs it, Esc/n dismisses.
There is no separate "show the command" state requiring a second key to actually run it. The
dialog itself is the generic `confirmWidget` (`internal/tui/confirm.go`), which knows nothing
about updates: bubbles ships no yes/no dialog, so mrboard owns one, parameterized by title, body,
and the message to emit on yes.

**The version widget owns the whole feature.** `internal/tui/version.go` holds the running
version, the checker, the check cadence, the badge, the enablement of the `u` binding, and the
self-update run. `model.go` routes messages to it and opens the overlay it builds; it stores no
update state of its own. Widgets cannot reach the root model's screen-wide resources directly, so
the widget emits `dismissOverlayMsg` and `toastMsg` and the root serves them.

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
the target repository isn't user-configurable (see Non-goals). `cache_ttl` sets both the adapter's
cache lifetime and the widget's re-check period, so one key describes the whole cadence.

**Demo mode never checks for real**, regardless of `update_check.enabled`: `demoadpt.UpdateChecker`
always reports no update available, the same "every driven port has a fake here" invariant already
covering `Notifier`.

## Consequences

- `internal/domain/service/updatesvc` and `internal/adapters/githubadpt` are new vendor-neutral
  port/adapter packages, following the same shape as `ticketsvc`/`jiraadpt`.
- `pkg/github` is a new raw API client package, alongside `pkg/gitlab`/`pkg/jira`.
- `confirmWidget` and `ConfirmCtx` are generic: the next feature needing a yes/no decision reuses
  them rather than adding another bespoke modal.
- `github.com/Masterminds/semver/v3` is a new direct dependency, used only by `githubadpt`.
- `tui.New`'s signature grows a `updatesvc.UpdateChecker` parameter; every call site (production
  and tests) passes it explicitly.
- Verifying the actual confirm-and-run path cannot be automated or driven through `agent-tui`: it
  runs a real `brew update && brew upgrade` against whichever machine executes it. Automated tests
  cover command *construction* only (`newSelfUpdateExecCmd`); the real end-to-end run is a
  deliberate, manually-triggered, one-time check by a human, not a scripted step.
- `mrboard update` (auto-assign reviewers) is renamed to `mrboard auto` in the same change — a
  breaking CLI change, called out in `CHANGELOG.md`, not left silently undocumented.
