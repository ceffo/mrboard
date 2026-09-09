#!/usr/bin/env bash
# Version arithmetic for mrboard releases. Shared by scripts/release.sh (manual
# release) and .github/workflows/release-on-merge.yml (release on merge), so both
# compute the same version from the same tag list.
#
# Usage:
#   next-version.sh --current                    latest release tag, empty if none
#   next-version.sh --next <patch|minor|major>   next version after the latest tag
#   next-version.sh --level "<commit subject>"   bump level the subject maps to
#   next-version.sh --plan "<commit subject>"    that level plus the version it
#                                                reaches, as level=/version= lines
#   next-version.sh --self-test                  check the subject → level table
#
# Levels (see docs/adr/0011-auto-release-on-merge.md):
#   minor    feat
#   patch    fix, perf, refactor, revert
#   none     a type enumerated below as deliberately not releasing
#   skip     "[skip release]" anywhere in the subject
#   unknown  a type in no list, or a subject that is not a conventional commit
#
# Only minor and patch release. none, skip and unknown all stop the release; they
# are separate levels so a deliberate non-releasing merge can be told apart from
# a mistyped subject, which is a mistake worth reporting.
#
# A "!" breaking marker upgrades a releasing type to minor and never to major:
# while the version is 0.x, v1.0.0 must stay a deliberate act.
set -euo pipefail

# Space-padded so a case glob matches a whole word.
MINOR_TYPES=" feat "
PATCH_TYPES=" fix perf refactor revert "
NONE_TYPES=" build chore ci docs merge release style test wip "

lower() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }

# True for a level that produces a release.
releases() { [[ "$1" == minor || "$1" == patch || "$1" == major ]]; }

# Prints the bump level a conventional-commit subject maps to.
level_from_subject() {
  local subject type breaking level=unknown
  subject=$(lower "$1")

  case "$subject" in
  *'[skip release]'*)
    echo skip
    return
    ;;
  esac

  [[ "$subject" =~ ^([a-z]+)(\([^\)]*\))?(!)?: ]] || {
    echo unknown
    return
  }
  type=${BASH_REMATCH[1]}
  breaking=${BASH_REMATCH[3]}

  case "$MINOR_TYPES" in *" $type "*) level=minor ;; esac
  case "$PATCH_TYPES" in *" $type "*) level=patch ;; esac
  case "$NONE_TYPES" in *" $type "*) level=none ;; esac

  # "!" only upgrades a type that already releases: docs!: still releases nothing.
  if [[ -n "$breaking" ]] && releases "$level"; then
    level=minor
  fi

  echo "$level"
}

latest_tag() {
  git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true
}

# Prints the version reached by applying a bump level to the latest tag.
# Bootstraps to v0.0.1 when the repository has no release tag yet.
compute_next() {
  local level=$1 latest major minor patch
  latest=$(latest_tag)

  if [[ -z "$latest" ]]; then
    echo "v0.0.1"
    return
  fi

  IFS='.' read -r major minor patch <<<"${latest#v}"
  case "$level" in
  major)
    major=$((major + 1))
    minor=0
    patch=0
    ;;
  minor)
    minor=$((minor + 1))
    patch=0
    ;;
  patch) patch=$((patch + 1)) ;;
  *)
    echo "error: unknown bump level '$level'" >&2
    return 1
    ;;
  esac

  echo "v${major}.${minor}.${patch}"
}

SELF_TEST_FAILURES=0

expect_level() {
  local subject=$1 want=$2 got
  got=$(level_from_subject "$subject")
  if [[ "$got" != "$want" ]]; then
    echo "FAIL: '$subject' → $got (want $want)" >&2
    SELF_TEST_FAILURES=$((SELF_TEST_FAILURES + 1))
  fi
}

# Guards the mapping the release pipeline depends on: a wrong level here ships a
# version nobody asked for, or silently ships none at all.
self_test() {
  expect_level 'feat: add a thing' minor
  expect_level 'feat(update): self-update check, --update flag (#10)' minor

  expect_level 'fix(domain): reviewer state' patch
  expect_level 'perf(tui): fewer redraws' patch
  expect_level 'refactor(core): split the boot path' patch
  expect_level 'revert: bring back the old parser' patch

  # Every enumerated non-releasing type.
  expect_level 'build(deps): bump goreleaser' none
  expect_level 'chore: update beads' none
  expect_level 'ci(release): pin the runner image' none
  expect_level 'docs: refresh stale docs' none
  expect_level 'merge: main into feat/x' none
  expect_level 'release: cut the notes' none
  expect_level 'style: reflow comments' none
  expect_level 'test(domain): cover the phase rules' none
  expect_level 'wip: half a thing' none

  # A breaking marker never reaches major while the version is 0.x, and only
  # upgrades a type that already releases.
  expect_level 'feat!: drop the legacy config key' minor
  expect_level 'feat(config)!: drop the legacy config key' minor
  expect_level 'fix!: change the exit code' minor
  expect_level 'docs!: rewrite everything' none

  expect_level 'FEAT(TUI): shouting still releases' minor
  expect_level 'feat: add a thing [skip release]' skip
  expect_level 'fix(tui): a thing [SKIP RELEASE]' skip
  expect_level 'chore: no release either way [skip release]' skip

  # Neither releasing nor enumerated: reported rather than silently dropped.
  expect_level 'Update README' unknown
  expect_level 'Merge pull request #12 from ceffo/feat/x' unknown
  expect_level "Merge branch 'main' into feat/x" unknown
  expect_level 'Revert "feat(tui): add a column"' unknown
  expect_level 'deps: bump goreleaser' unknown
  expect_level 'security: stop logging the token' unknown
  expect_level 'feat add a thing' unknown
  expect_level 'feat/x: add a thing' unknown
  expect_level '' unknown

  if [[ "$SELF_TEST_FAILURES" -gt 0 ]]; then
    echo "$SELF_TEST_FAILURES self-test failure(s)" >&2
    exit 1
  fi
  echo "next-version.sh self-test passed"
}

# Prints this file's comment header: drop the shebang, then everything from the
# first line of code onwards.
usage() {
  sed -e '1d' -e '/^[^#]/,$d' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' >&2
  exit 1
}

case "${1:-}" in
--current)
  latest_tag
  ;;
--next)
  [[ $# -eq 2 ]] || usage
  compute_next "$2"
  ;;
--level)
  [[ $# -eq 2 ]] || usage
  level_from_subject "$2"
  ;;
--plan)
  [[ $# -eq 2 ]] || usage
  level=$(level_from_subject "$2")
  echo "level=$level"
  if releases "$level"; then
    echo "version=$(compute_next "$level")"
  else
    echo "version=none"
  fi
  ;;
--self-test)
  self_test
  ;;
*) usage ;;
esac
