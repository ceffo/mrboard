#!/bin/zsh
# Entry point for agent-tui: cd to project root and launch the TUI.
cd "$(dirname "$0")/.." || exit 1
exec ./bin/mrboard run
