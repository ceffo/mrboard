#!/usr/bin/env bash
# Permanent test fixture for the terminal handoff (docs/adr/0004-external-command-launcher.md,
# "Terminal handoff"): a child that owns the alternate screen the way a full-screen review tool
# does. It enters the alt screen, draws, redraws on resize, reports the key it receives, and
# leaves the alt screen on exit. Press `f` to exit 1 (exercises the error toast); any other key,
# or 10 seconds without one, exits 0.
set -u

draw() {
	printf '\033[2J\033[H'
	echo "=== mrboard alt-screen fixture ==="
	echo "args: $*"
	echo "size: $(stty size)"
	echo "last key: ${key:-none}"
	echo "press f to fail, any other key to exit"
}

key=""
trap 'draw "$@"' WINCH
printf '\033[?1049h'
draw "$@"
IFS= read -r -s -n 1 -t 10 key || true
printf '\033[?1049l'
[ "$key" = "f" ] && exit 1
exit 0
