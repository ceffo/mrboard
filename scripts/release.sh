#!/usr/bin/env bash
# Usage: scripts/release.sh [patch|minor|major]
# Reads the latest git tag, bumps the requested component, tags, and pushes.
# Bootstraps to v0.0.1 if no tags exist.
set -euo pipefail

bump=${1:-patch}

# Abort if local main has unpushed commits.
unpushed=$(git log origin/main..main --oneline 2>/dev/null | wc -l | tr -d ' ')
if [[ "$unpushed" -gt 0 ]]; then
  echo "error: $unpushed unpushed commit(s) on main — push first, then release" >&2
  exit 1
fi

latest=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)

if [[ -z "$latest" ]]; then
  next="v0.0.1"
else
  IFS='.' read -r major minor patch <<< "${latest#v}"
  case "$bump" in
    major) major=$((major + 1)); minor=0; patch=0 ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    patch) patch=$((patch + 1)) ;;
    *) echo "usage: release.sh [patch|minor|major]" >&2; exit 1 ;;
  esac
  next="v${major}.${minor}.${patch}"
fi

gum confirm --default=false "Tag and push $next?" || { echo "Aborted."; exit 1; }
git tag "$next"
git push origin "$next"
