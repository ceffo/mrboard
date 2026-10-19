#!/usr/bin/env bash
# PreToolUse(Bash): a session started in the main checkout (the supervisor) must not cd into a
# worktree; developer sessions start inside their worktree and are not affected.
set -euo pipefail
input="$(cat)"
cwd="$(jq -r '.cwd // empty' <<<"$input")"
cmd="$(jq -r '.tool_input.command // empty' <<<"$input")"
main="${CLAUDE_PROJECT_DIR:-}"
[ -n "$main" ] && [ "$cwd" = "$main" ] || exit 0
wt_root="$(dirname "$main")/$(basename "$main").worktrees"
if grep -Eq "(^|[;&|({[:space:]])(cd|pushd)[[:space:]]+[\"']?(${wt_root}|~/[^[:space:]]*\.worktrees|\.\./[^[:space:]]*\.worktrees)" <<<"$cmd"; then
  echo "Blocked: the supervisor stays in the main checkout. Use 'git -C <worktree> …' or absolute paths instead of cd into ${wt_root}." >&2
  exit 2
fi
exit 0
