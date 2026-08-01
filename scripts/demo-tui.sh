#!/bin/zsh
# Entry point for demo mode: launches the board against the built-in dataset.
# Used both by `just demo` (via demo/mrboard.tape) and by agent-tui.
#
# PATH is scrubbed deliberately. The diff view resolves an external differ once,
# in a package init(), with no runtime override — so whether one is installed
# changes how diffs render. Pinning PATH keeps a recording reproducible on any
# machine. Running `mrboard --demo` directly with a differ installed is still
# correct, just a different (nicer) rendering than the recorded GIF.
cd "$(dirname "$0")/.." || exit 1
export PATH=/usr/bin:/bin
exec ./bin/mrboard --demo
