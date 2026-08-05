#!/usr/bin/env bash
# Usage: scripts/release.sh [patch|minor|major] [--force] [--dry-run]
# Reads the latest git tag, bumps the requested component, tags, and pushes.
# Bootstraps to v0.0.1 if no tags exist.
# Called with no bump argument, prompts interactively (via gum) with a live
# preview of the resulting version for each choice.
# --force   skips the gum confirmation prompt (for non-interactive environments).
# --dry-run prints the next version without tagging or pushing.
set -euo pipefail

bump=""
force=false
dry_run=false
for arg in "$@"; do
  case "$arg" in
    --force) force=true ;;
    --dry-run) dry_run=true ;;
    *) bump="$arg" ;;
  esac
done

# Abort if local main has unpushed commits.
unpushed=$(git log origin/main..main --oneline 2>/dev/null | wc -l | tr -d ' ')
if [[ "$unpushed" -gt 0 ]]; then
  echo "error: $unpushed unpushed commit(s) on main — push first, then release" >&2
  exit 1
fi

latest=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)

compute_next() {
  local level=$1
  if [[ -z "$latest" ]]; then
    echo "v0.0.1"
    return
  fi
  local major minor patch
  IFS='.' read -r major minor patch <<< "${latest#v}"
  case "$level" in
    major) major=$((major + 1)); minor=0; patch=0 ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    patch) patch=$((patch + 1)) ;;
  esac
  echo "v${major}.${minor}.${patch}"
}

if [[ -z "$bump" ]]; then
  command -v gum >/dev/null 2>&1 || { echo "usage: release.sh [patch|minor|major]" >&2; exit 1; }
  choice=$(printf 'patch  →  %s\nminor  →  %s\nmajor  →  %s\n' \
    "$(compute_next patch)" "$(compute_next minor)" "$(compute_next major)" \
    | gum choose --header "Select version bump (current: ${latest:-none})")
  [[ -z "$choice" ]] && { echo "Aborted."; exit 1; }
  bump=${choice%%  *}
fi

case "$bump" in
  patch | minor | major) ;;
  *) echo "usage: release.sh [patch|minor|major]" >&2; exit 1 ;;
esac

next=$(compute_next "$bump")

if [[ "$dry_run" == true ]]; then
  echo "dry-run: would tag and push $next"
  exit 0
fi
if [[ "$force" == false ]]; then
  gum confirm --default=false "Tag and push $next?" || { echo "Aborted."; exit 1; }
fi
echo "Tagging $next" >&2
git tag "$next"
git push origin "$next"
