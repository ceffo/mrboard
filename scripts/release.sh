#!/usr/bin/env bash
# Usage: scripts/release.sh [patch|minor|major] [--force] [--dry-run]
# Reads the latest git tag, bumps the requested component, tags, and pushes.
# The version arithmetic itself lives in next-version.sh, shared with CI.
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

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

latest=$(bash "$script_dir/next-version.sh" --current)

compute_next() {
  bash "$script_dir/next-version.sh" --next "$1"
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
