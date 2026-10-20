#!/usr/bin/env bash
# fleet-ack.sh <KEY>... : mark the item's report as handled (touches supervisor/.ack-<KEY>), so a
# re-armed fleet-watch.sh doesn't report it again until the agent writes a newer report.
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=fleet-lib.sh
. "$here/fleet-lib.sh"
[ $# -gt 0 ] || { echo "usage: fleet-ack.sh <KEY>..." >&2; exit 2; }
mkdir -p "$FLEET_SUPERVISOR_DIR"
for k in "$@"; do touch "$FLEET_SUPERVISOR_DIR/.ack-${k^^}"; done
