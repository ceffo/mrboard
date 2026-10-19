#!/usr/bin/env bash
# fleet-lib.sh: sourced by every fleet script. Resolves the project root and loads the project's
# fleet configuration, so no script carries a path, a repo name or a team key of its own.
#
# Config file: <project>/.claude/fleet.env (override the location with $FLEET_CONFIG). Plain
# VAR=value lines; an environment variable already set wins over the file.
#
#   FLEET_MAIN             main checkout (default: $CLAUDE_PROJECT_DIR, else the script's git toplevel)
#   FLEET_DEFAULT_BRANCH   default branch (default: origin/HEAD, else main)
#   FLEET_REPO             GitHub owner/name (default: what `gh repo view` says)
#   FLEET_WT_ROOT          worktree root (default: <main>.worktrees, beside the checkout)
#   FLEET_SUPERVISOR_DIR   supervisor state (ledger, user log, watcher pids; default: <wt root>/supervisor)
#   FLEET_BRIEFS_DIR       briefs (default: <wt root>/briefs)
#   FLEET_REPORTS_DIR      reports (default: <wt root>/reports)
#   FLEET_TRACKER          github | linear (default github)
#   FLEET_CI_WORKFLOWS     workflows whose latest run on the default branch the supervisor owns (default "CI")
#   FLEET_RELEASE_WORKFLOW a workflow that a green push run must trigger (default none)
#   FLEET_REVIEW_BOT       regex of a review bot's login whose unanswered PR threads block a merge (default none)
#   LINEAR_TEAM            Linear team key (tracker=linear)
#   LINEAR_PROJECT         optional Linear project name new issues are filed in
#   LINEAR_API_KEY         or the first line of $FLEET_CONFIG_DIR/linear-api-key (default ~/.config/agents-dev-fleet)
# shellcheck shell=bash

_fleet_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FLEET_SCRIPTS="$_fleet_here"
FLEET_MAIN="${FLEET_MAIN:-${CLAUDE_PROJECT_DIR:-$(git -C "$_fleet_here" rev-parse --show-toplevel 2>/dev/null || true)}}"
[ -n "$FLEET_MAIN" ] || { echo "fleet: cannot find the project root (set FLEET_MAIN)" >&2; exit 1; }

_fleet_cfg="${FLEET_CONFIG:-$FLEET_MAIN/.claude/fleet.env}"
if [ -f "$_fleet_cfg" ]; then
  # environment wins: remember what was set, source the file, restore
  _fleet_saved="$(export -p | grep -E '^declare -x (FLEET_|LINEAR_)' || true)"
  # shellcheck disable=SC1090
  . "$_fleet_cfg"
  eval "$_fleet_saved"
  unset _fleet_saved
fi

FLEET_TRACKER="${FLEET_TRACKER:-github}"
FLEET_CI_WORKFLOWS="${FLEET_CI_WORKFLOWS:-CI}"
FLEET_RELEASE_WORKFLOW="${FLEET_RELEASE_WORKFLOW:-}"
FLEET_REVIEW_BOT="${FLEET_REVIEW_BOT:-}"
FLEET_WT_ROOT="${FLEET_WT_ROOT:-$(dirname "$FLEET_MAIN")/$(basename "$FLEET_MAIN").worktrees}"
FLEET_SUPERVISOR_DIR="${FLEET_SUPERVISOR_DIR:-${SUPERVISOR_DIR:-$FLEET_WT_ROOT/supervisor}}"
FLEET_BRIEFS_DIR="${FLEET_BRIEFS_DIR:-$FLEET_WT_ROOT/briefs}"
FLEET_REPORTS_DIR="${FLEET_REPORTS_DIR:-$FLEET_WT_ROOT/reports}"
FLEET_CONFIG_DIR="${FLEET_CONFIG_DIR:-$HOME/.config/agents-dev-fleet}"

if [ -z "${FLEET_DEFAULT_BRANCH:-}" ]; then
  FLEET_DEFAULT_BRANCH="$(git -C "$FLEET_MAIN" symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null | sed 's|^origin/||' || true)"
  FLEET_DEFAULT_BRANCH="${FLEET_DEFAULT_BRANCH:-main}"
fi

# fleet_repo: owner/name of the GitHub repo (FLEET_REPO, else asked of gh once).
fleet_repo() {
  if [ -z "${FLEET_REPO:-}" ]; then
    FLEET_REPO="$(cd "$FLEET_MAIN" && gh repo view --json nameWithOwner --jq .nameWithOwner)" \
      || { echo "fleet: cannot tell the GitHub repo (set FLEET_REPO)" >&2; return 1; }
  fi
  printf '%s\n' "$FLEET_REPO"
}

export FLEET_MAIN FLEET_DEFAULT_BRANCH FLEET_WT_ROOT FLEET_SUPERVISOR_DIR FLEET_BRIEFS_DIR FLEET_REPORTS_DIR \
  FLEET_TRACKER FLEET_CI_WORKFLOWS FLEET_RELEASE_WORKFLOW FLEET_REVIEW_BOT FLEET_CONFIG_DIR FLEET_SCRIPTS
