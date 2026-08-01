# 0006 — Demo mode

**Status**: Accepted

## Context

mrboard could not be shown to anyone who did not already have a GitLab instance, a
personal access token, and a config file. That blocked two things:

- a README screenshot or GIF. The only board available to record was the maintainer's
  real one, full of company project names, ticket keys, and colleagues' names.
- contributors exercising the UI. Any change to a widget had to be verified against live
  company data, which is slow, non-reproducible, and unavailable to an outside
  contributor.

The board is also the part of the app most likely to regress invisibly: `just check`
validates compilation and unit tests, but the interesting failures are layout and
interaction ones.

## Non-goals

- **No fake for the browser-opening actions.** `o` and `J` shell out to `open`/`xdg-open`
  directly from the TUI with no port to substitute. Introducing one means changing
  `internal/tui`, which this design deliberately avoids. The recording does not press
  them; see Consequences.
- **No runtime-loadable fixture.** A `--demo-fixture <path>` flag would make recordings
  depend on the current directory and would not work for an installed binary. Worth
  adding later for iteration; not now.
- **No in-UI "DEMO" badge.** Honesty is carried by the data (`demo-corp/*` projects,
  `.invalid` hostnames), by `--help`, and by the README. A badge would require threading
  a title suffix through `internal/tui`, breaking the zero-TUI-changes property below.
- **No external command launcher in demo mode.** `Commands` is nil, so no subprocess is
  reachable. A command that suspends the terminal mid-recording looks like a glitch.
- **The GIF is not regenerated in CI.** `vhs` needs specific fonts and a renderer, so a
  CI-produced GIF would not be byte-stable and would churn the repo on every run. What
  does run in CI is the `demoadpt` test suite.

## Decision

### Demo mode is a composition-root feature, with no changes to `internal/tui`

Every optional feature in the TUI is already gated on either a dependency being non-nil
(`Notifier`, `TicketEnricher`) or a config field being set (`CurrentUser`,
`Jira.BoardID`, `Jira.InstanceURL`). Supplying the right set of fakes and the right
synthetic config therefore lights up the whole board without touching a single widget.

This is worth preserving deliberately: it means demo mode cannot drift from the real UI,
because it *is* the real UI. Any future change that requires a `if demoMode` branch
inside `internal/tui` should be treated as a design smell and pushed back into the
adapter or the config.

### Activation is a persistent `--demo` flag

`bootCore` in `internal/cmd/mrboard/root.go` is a closure shared by the root command and
`fetch`, so one branch inside it covers both `mrboard --demo` and `mrboard --demo fetch`.
The flag composes with the existing `--theme`, `--mode`, and `--log-level` overrides,
which the recording uses, and it is discoverable in `--help`.

Rejected alternatives:

- **A config key** (`demo: true`) cannot work: `config.Load` fails when no file is found,
  so a config key cannot bootstrap a user who has no config — which is exactly the
  audience.
- **A `demo` subcommand** would have to duplicate the board's session flags, which live
  on the root command, or lose them.
- **A build tag** would exclude the fixture from the released binary, so
  `brew install mrboard && mrboard --demo` — the whole point — would fail, and the GIF
  would be recorded from a binary that is not the one shipped.

`--demo` and `--config` are mutually exclusive and error out together, rather than
silently ignoring one.

### `config.DemoConfig()` builds a real config in memory

Demo mode bypasses `config.Load`, which means it also bypasses every viper default.
`DemoConfig` therefore sets each of them by hand. The trap this avoids is subtle: a zero
`LifetimeWarnAfter`/`LifetimeErrorAfter` silently disables the board's age colouring,
which is one of the things worth showing. A test asserts the demo config passes the same
`validate` the real path uses, with no warnings — it is a legal config, just never read
from disk.

Hostnames use the reserved `.invalid` TLD (RFC 2606) so nothing can resolve even if a
request escaped the fakes, and the log goes to the OS temp dir rather than the user's
data dir.

### `internal/adapters/demoadpt` implements every driven port

One package implements all six: `mrsvc.MergeRequestSource`, `domain.StateStore`,
`domain.SnapshotStore`, `domain.Notifier`, `ticketsvc.TicketEnricher`, and
`ticketsvc.TicketLinker`. It sits alongside the other driven-port implementations and
imports only `internal/domain`, the two service packages, and stdlib plus YAML.

**These are not mocks.** Generated mockery doubles live in sibling `mocks/` packages and
never link into a binary; `demoadpt` ships and backs a user-facing feature, and its data
is a curated narrative a generated mock cannot express. This is stated explicitly so a
future reader does not "fix" it by deleting it.

Per the no-vendor-bleeding rule, nothing declared in the package is named for a provider,
and the fixture's keys name capabilities (`issue_types`, `merge_status`, `ticket_key`).
It reads and writes the vendor-named fields `internal/domain` already exposes, which is
what every other domain consumer does.

### The fixture is embedded YAML with relative ages

A checked-in `fixture/board.yaml`, embedded with `go:embed` — the pattern the repo
already uses for themes. YAML rather than Go literals because the content is multi-line
markdown and unified diffs, which block scalars make readable and diffable; adding an MR
to show off a new feature should be a data edit, not a code change.

Two rules make it work, both enforced by tests because both are easy to break silently:

1. **Ages are stored as offsets, never absolute timestamps**, and materialised against a
   single boot anchor. A fixture with absolute dates would read as increasingly stale as
   it ages in git.
2. **Every offset is a whole number of minutes.** Ages render truncated to the minute, so
   an offset with a seconds component makes the first frame of a recording differ from
   the next.

The fixture does **not** carry a `phase` field. Phase, `WaitingSince` and
`ReadyToMergeSince` are derived by the real `domain.ClassifyPhase` /
`domain.DeriveWaitingSince`, in the same order `gitlabadpt`'s mapper uses. The demo board
therefore cannot disagree with production about which column an MR belongs in. The
trade-off is that a future change to the phase rules will reshuffle the demo board — which
is why the column-placement test is load-bearing: it turns a silent reshuffle into a red
build.

MRs the fixture does not give an explicit diff get a small generated one, so the diff view
is useful from any card rather than answering "no files changed".

### Writes mutate the dataset; the real cache is never touched

The dataset is the single mutable source of truth for both reads and writes. A fake that
served immutable fixture data would let an edited card visibly revert on the next refresh
tick. Concretely: `SetReviewers` and `SaveApprovers` mutate and then re-derive the phase,
so promoting the last outstanding approver actually moves the card into the Approved
column.

Because Bubble Tea runs each `Cmd` on its own goroutine, fetches race writes: the dataset
takes a mutex and **every read returns a deep copy**. This is not hygiene — the reviewer
write path mutates `mr.Reviewers[i]` in place on whatever slice it was handed, so sharing
the backing array is a real race. The package's tests run under `-race`.

`core.NewDemo` is a separate constructor rather than a branch in `core.New` for one
specific reason: `statestore.New` and `snapshotstore.New` create their directories under
the user's XDG paths *at construction time*. Avoiding `Save` is not sufficient; the
constructors must never run. A test sets both XDG roots to temp dirs, boots demo mode,
writes through both store ports, and asserts both directories are still empty.

The demo snapshot store serves a pre-seeded warm cache, so the board is on screen and
interactive from the first frame instead of showing a spinner — which also demonstrates
the warm-boot behaviour ADR-0005 exists to provide. The demo state store starts from
`domain.DefaultAppState()` so a recording never inherits the maintainer's saved theme,
sort, or filter.

Every ticketed MR's description already ends with the `<!-- mrboard -->` marker. That
makes ADR-0003's idempotency check short-circuit, so no description write fires and the
board does not mutate itself while on screen. Also test-enforced.

### Recording with vhs

`demo/mrboard.tape` is a checked-in, reviewable script; `just demo` re-records
`demo/mrboard.gif`. vhs is Charm's own tool, same house as the TUI libraries here, and a
scripted tape means re-recording after a UI change is a command rather than a
hand-performed screen capture. `agent-tui` keeps its existing role — verification, not
recording.

`scripts/demo-tui.sh` pins `PATH` before launching. The diff view resolves its external
differ once in a package `init()` with no runtime override, so whether one happens to be
installed changes how diffs render; pinning `PATH` keeps a recording reproducible across
machines.

### The published GIF is recorded from a clean worktree, not the working tree

The version string is baked in at build time by `git describe` and is visible in the
recorded footer. That makes recording from the working tree unable to produce a
release-looking frame, for two compounding reasons:

- anything uncommitted stamps `-dirty` — including `demo/mrboard.gif` itself, which the
  recording command rewrites, so the tree is dirty by the time the next run reads it;
- even a clean tree past a tag reads `v0.10.0-2-gabc1234`, since `describe` counts commits
  since the tag.

A bare `v0.10.0` is therefore only reachable from a checkout sitting exactly on the tag.
`scripts/record-demo.sh <ref>` creates a throwaway `git worktree` at that ref, builds
there, records, and copies the GIF back. `just demo` keeps the working-tree behaviour for
iterating; `just demo-release <tag>` produces the committed artefact.

The binary comes from the ref but the **tape comes from the working tree**, so the
recording script can be iterated without tagging first. The version is resolved *before*
the tape is staged: the stamp describes the source the binary is compiled from, and the
tape contributes nothing to the binary — but copying it in does dirty the worktree, which
would otherwise reintroduce the exact problem this exists to solve. The script asserts the
worktree is clean at that point rather than silently emitting a `-dirty` GIF.

Consequence worth noting: the tag cannot contain the GIF recorded from it, since the GIF
is produced after tagging. The GIF documents a release rather than shipping inside it.

### Recording dimensions have a hard floor

The board is four 28-column panes, so it needs ~120 columns; below that the Approved
column is truncated off the right edge and reviewer pills clip mid-word, which reads as a
broken app rather than a small window. The tape is set to the smallest grid that fits
(~120x34). If the GIF needs to be smaller, cut `Framerate` or a beat — not the width.

## Consequences

- `mrboard --demo` runs the full board with no config, credentials, or network. It is the
  answer to "can I see it before I set it up", and the way a contributor exercises a
  widget change.
- Demo mode adds no user-facing config surface: no new YAML key, no change to the config
  search path, nothing in `mrboard.yaml.example`.
- The demo dataset is a second consumer of the domain rules, so it doubles as a
  regression check on them: the column-placement test fails if the phase rules change.
- Adding an MR to the fixture means honouring the whole-minute rule and the back-link
  marker rule. Both are enforced by tests, but the tests are the only thing stopping the
  recording from quietly becoming non-reproducible.
- `o` and `J` remain live in demo mode and will open a browser at a `.invalid` URL. The
  tape avoids them. Giving the TUI a URL-opener port is the follow-up that would close
  this, at the cost of the zero-TUI-changes property.
- The GIF is a committed binary that must be regenerated by hand when the UI changes.
  A stale GIF is now a possible form of documentation rot.
