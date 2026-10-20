#!/usr/bin/env bash
# gate-check.sh <worktree> <key> : exit 0 when reports/<key>-gate (written by `just gate <key>`) holds the
# worktree's HEAD sha, i.e. the full gate ran on the commit about to ship. Exit 1 otherwise.
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
wt=$1 key=$2
stamp="$FLEET_REPORTS_DIR/$key-gate"
head=$(git -C "$wt" rev-parse HEAD)
if [ ! -f "$stamp" ]; then echo "gate-check: no $stamp: the gate has not run" >&2; exit 1; fi
ran=$(tr -d '[:space:]' < "$stamp")
if [ "$ran" != "$head" ]; then echo "gate-check: gate ran on $ran, HEAD is $head" >&2; exit 1; fi
echo "gate-check: ok $head"
