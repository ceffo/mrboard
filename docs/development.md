# Development

Everything a contributor needs. The [README](../README.md) is for people *using* mrboard.

## Documentation map

| | |
| --- | --- |
| [configuration.md](configuration.md) | Every config key, defaults, env vars, troubleshooting |
| [theme-format.md](theme-format.md) | Writing a custom theme |
| [keybindings.md](keybindings.md) | How bindings are declared and registered |
| [architecture.md](architecture.md) | Package boundaries, data flow, dependency rules |
| [domain-model.md](domain-model.md) | Phase rules and the reviewer state machine |
| [tui-conventions.md](tui-conventions.md) | TUI file structure and widget rules |
| [clean_architecture.md](clean_architecture.md) | The ports-and-adapters principles this follows |
| [adr/](adr/) | Why things are built the way they are, one record per decision |

`AGENTS.md` at the repo root (symlinked as `CLAUDE.md`) is the working agreement: quality
gates, test conventions, and the non-negotiable architecture rules.

## Commands

```bash
just check                    # fmt + lint + build + test + release-script self-test
just generate                 # regenerate mocks from .mockery.yml
just demo-run                 # launch the board against the demo dataset
just demo                     # re-record the GIF from the working tree
just demo-release v0.13.0     # re-record it from a clean checkout of a tag
```

CI runs `just check-ci` — the same gate, except it fails on unformatted code instead of
reformatting it. golangci-lint and mockery are `go.mod` tool directives, so there is
nothing to install beyond the Go toolchain; `just tools-update` takes the newest release
of each.

Use `demo-release` for the committed GIF: the version in the footer is stamped at build
time, so recording from the working tree labels the frame `-dirty`. Both need
[vhs](https://github.com/charmbracelet/vhs).

## Branching

`main` is protected. Start from a branch named `type/short-kebab-description`, where
`type` is the conventional-commit type of the work.

## Releasing

Merging a PR into `main` releases it. The squashed commit subject — the PR title — picks
the bump:

| PR title | Bump | Release |
| --- | --- | --- |
| `feat(tui): …` | minor | yes |
| `fix`, `perf`, `refactor`, `revert` | patch | yes |
| `build`, `chore`, `ci`, `docs`, `merge`, `release`, `style`, `test`, `wip` | none | no |
| `… [skip release]` | skip | no |
| any other type, or a subject that is not a conventional commit | unknown | no, with a warning on the run |

A `!` breaking marker bumps the minor, not the major — `v1.0.0` is only reachable through
`just release major`.

The last two rows are separate on purpose: `none` is a deliberate non-releasing merge,
`unknown` is usually a mistyped title. Both stop the release, but `unknown` annotates the
workflow run so an unreleased merge does not pass unnoticed. Check a title before merging
with:

```bash
just release-preview "feat(tui): add a column"
# level=minor
# version=v0.14.0
```

See [adr/0011](adr/0011-auto-release-on-merge.md) for the full rationale.
