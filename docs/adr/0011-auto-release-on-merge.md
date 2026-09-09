# 0011 — Auto-release on merge

**Status**: Accepted

## Context

Releasing was a two-step ritual performed by hand: merge the PR, then remember to run
`just release patch|minor|major` locally, which tags and pushes, which fires
`release.yml`, which runs goreleaser and updates the homebrew cask.

Nothing enforced the second step, and nothing tied the bump level to what actually
changed. Since ADR-0010 added an in-app update check, a version that never gets tagged is
no longer just a missing GitHub release — every installed binary keeps reporting itself
as current, so the fix a user is waiting for is invisible to them.

The PR title already carries the answer. `main` is protected and every change lands as a
squash merge, so the merged commit subject *is* the PR title, and the repo's commit
convention already puts a conventional-commit type at the front of it:

```
feat(update): self-update check, --update flag, fang CLI (#10)
fix(domain): reviewer state, fetch parity, GraphQL batch limit (#4)
docs: refresh stale docs and require branch workflow (#7)
```

## Non-goals

- **No `semantic-release`.** The sibling `nova` project runs npm `semantic-release` with
  `@semantic-release/gitlab`. It would mean a Node toolchain in a Go repo, a plugin chain
  to configure, and a second thing that decides versions — while the interesting logic
  here is a dozen lines of arithmetic already present in `scripts/release.sh`. goreleaser
  already produces the release and its notes; only the tag was missing.
- **No automatic major bump.** See the decision below: `v1.0.0` stays a deliberate act.
- **No CHANGELOG.md commit from CI.** Writing it would mean CI pushing a commit to
  protected `main`, which needs a privileged token and risks re-triggering itself.
  goreleaser's generated release notes cover the per-release record; `CHANGELOG.md` stays
  curated by hand.
- **`BREAKING CHANGE:` footers are not read.** Only the `!` marker in the subject is,
  because the subject is all a squash merge reliably surfaces. This costs nothing today:
  no subject can produce a major, so the only thing a footer could change is `fix!` →
  minor instead of patch.
- **The PR API is not consulted.** No labels, no `pull_request: closed` event. Everything
  the decision needs is in the commit subject, which keeps the whole thing reproducible
  from a local checkout.

## Decision

### The merged commit subject decides the bump

| Level | Types | Releases |
| --- | --- | --- |
| `minor` | `feat` | yes |
| `patch` | `fix`, `perf`, `refactor`, `revert` | yes |
| `none` | `build`, `chore`, `ci`, `docs`, `merge`, `release`, `style`, `test`, `wip` | no |
| `skip` | any type, with `[skip release]` in the subject | no |
| `unknown` | any other type, and any subject that is not a conventional commit | no |

Defaulting an unrecognised type to *no release* rather than *patch* is the load-bearing
choice. It means the failure mode of a mistyped or non-conventional title is a release
that has to be triggered by hand — recoverable with `just release patch` — instead of a
published version nobody intended. `docs:` and `chore:` merges already land without a
release today; this keeps that true.

### The non-releasing types are enumerated, not left to the fallback

`none`, `skip` and `unknown` all stop the release, so collapsing them into one answer
would be simpler. They are kept apart because they mean opposite things: `none` and
`skip` are a merge that was never meant to ship, and `unknown` is almost always a
mistyped or unconventional title — a mistake, and one whose symptom is *nothing
happening*, which is the hardest kind to notice.

So `unknown` additionally annotates the workflow run with a `::warning::` naming the
subject and how to recover. It does **not** fail the run: GitHub generates merge subjects
of its own (`Merge pull request #12 from …`, `Revert "feat(tui): …"`) that are legitimate
and unconventional, and a red `main` on every one of those would train everybody to
ignore the signal.

The consequence to accept is that the enumeration is now a list to maintain. A type used
for the first time — `deps`, `security` — lands as `unknown` and releases nothing until
it is added to one of the three lists. The warning is what makes that visible on the first
try rather than the third.

### A breaking change bumps the minor, never the major

While the version is `0.x`, `feat!:` and `fix!:` bump the minor. `v1.0.0` is reachable
only through `just release major` from a maintainer's machine.

The alternative — strict semver, where `!` means major — was rejected because at `0.13.0`
a single mistyped PR title would publish `v1.0.0`, and the homebrew cask, the update
prompt, and the tag history are all effectively unrecallable once that happens. This also
happens to be the ordinary reading of semver for `0.x`, where the minor is the breaking
position.

Note that `!` only *upgrades* a type that already releases: `docs!:` still releases
nothing.

### `scripts/next-version.sh` is the single source of truth

Both release paths call it, so they cannot disagree about what version follows
`v0.13.0`:

- `scripts/release.sh` (manual) calls `--next <level>`, replacing its own copy of the
  arithmetic.
- `release-on-merge.yml` (automatic) calls `--plan "<subject>"`, which prints `level=` and
  `version=` lines that go straight into the step outputs. The workflow never restates
  which levels release; it reads `version=none` and stops.

The subject → level mapping is pure string parsing with no git dependency, which makes it
cheaply testable: `--self-test` runs every enumerated type plus the edge cases through
`--level`, and the workflow runs it as a pre-flight step before it is allowed to tag
anything. A wrong level here either ships a version nobody asked for or silently ships
none, and neither is visible in a build log, so the table is the guard.

`--plan` also answers "what will merging this release?" from a local checkout, exposed as
`just release-preview "<title>"`.

### The tag is pushed, then `release.yml` is dispatched explicitly

`release-on-merge.yml` computes the version, pushes a lightweight tag, and then runs
`gh workflow run release.yml --ref main -f tag=<version>`.

The dispatch is not redundant: **a tag pushed with the default `GITHUB_TOKEN` does not
fire another workflow's `push: tags` trigger.** GitHub suppresses that to prevent
recursive runs. Without the explicit dispatch, the tag would appear and the release would
silently never happen — the exact failure this ADR exists to remove, just moved later in
the pipeline.

Two alternatives were rejected:

- **Push the tag with a PAT or GitHub App token**, so `push: tags` fires naturally. Adds
  a long-lived privileged secret to store and rotate, in exchange for deleting one line.
- **Run goreleaser in the same workflow.** Then two workflows know how to publish a
  release, or `release.yml` is deleted and the manual tag path loses its own trigger.

Dispatching keeps `release.yml` the only thing that knows how to publish, and keeps all
three routes into it (pushed tag, merge dispatch, manual dispatch) on the same steps. The
dispatch is on `--ref main` rather than the new tag because `release.yml` already checks
out `inputs.tag` itself.

### Serialized, and idempotent on the tag

`concurrency: release-on-merge` with `cancel-in-progress: false` queues runs instead of
cancelling them. Both halves matter: two merges landing seconds apart would otherwise
read the same latest tag and compute the same next version, and a *cancelled* run is a
release that never happens.

Before tagging, the workflow checks whether the computed tag already exists and exits
cleanly if so, so a re-run cannot fail on a duplicate tag.

### The workflow has a side-effect-free dry run

`release-on-merge.yml` also accepts a `workflow_dispatch` with an optional `subject`. It
computes and reports, and the tagging step is gated on `github.event_name == 'push'`, so a
dispatched run can never tag or release.

This exists because a `push: main` trigger cannot be exercised without merging something,
which would make the first real verification of the pipeline also its first real release.
The dispatch checks out the repo, reads the actual tag list, and runs the actual mapping
on the runner. `just release-preview` answers the same question locally; this answers it
where it will actually run.

The subject reaches the script through the environment rather than being interpolated
into the `run:` block, because a dispatch input is arbitrary attacker-supplied text and
`${{ inputs.subject }}` inside a shell script is a command-injection hole.

## Consequences

- Merging a `feat` or `fix` PR publishes a release with no further action. The bump is
  chosen by the PR title, which means **the PR title is now a release decision** — it is
  worth reviewing as one, and `[skip release]` is the way out for a `fix` that should not
  ship on its own.
- `just release` keeps working unchanged and stays the only route to a major, so
  reaching `v1.0.0` remains an explicit, local act.
- Only the head commit's subject is read. A push that lands several commits at once
  (an admin push, or a rebase merge if the merge strategy ever changes) releases based on
  its tip alone and ignores the rest. Squash merges are what makes this safe; changing
  the repo's merge strategy invalidates this design.
- `CHANGELOG.md` can now fall behind the released versions, since nothing updates it
  automatically. The `/write-changelog` workflow remains the way to catch it up.
- Version numbers advance faster than before, because every `fix` merge ships one instead
  of being batched into the next manual release.
- Two repository settings this now depends on. The default workflow token is read-only, so
  both `contents: write` and `actions: write` are granted in the workflow itself; and the
  "No direct commit" ruleset targets branches, with no tag protection — adding a tag
  ruleset means granting `github-actions[bot]` a bypass or the tag push starts failing.
