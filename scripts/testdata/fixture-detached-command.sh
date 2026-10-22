#!/bin/sh
# Fixture for detached commands (docs/adr/0004-external-command-launcher.md,
# "Detached commands"). Sleeps, then writes a marker file, so a check can show the
# process outlives mrboard: the marker appears after mrboard has exited.
# Usage: fixture-detached-command.sh <marker-file> [exit-code] [sleep-seconds]
set -eu

marker="${1:?usage: fixture-detached-command.sh <marker-file> [exit-code] [sleep-seconds]}"
code="${2:-0}"

sleep "${3:-3}"
echo "detached fixture ran: $$" > "$marker"
if [ "$code" != 0 ]; then
	echo "fixture failing on purpose" >&2
fi
exit "$code"
