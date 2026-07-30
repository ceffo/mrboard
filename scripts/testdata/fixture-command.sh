#!/bin/sh
# Permanent test fixture for the external-command-launcher feature
# (docs/adr/0004-external-command-launcher.md). Configured as a real "commands"
# entry in mrboard.yaml.example so the tea.ExecProcess suspend/resume flow can
# be re-verified with agent-tui without depending on tools like tuicr/hunk,
# which are not installed in CI/test environments.
set -eu

echo "=== mrboard fixture command ==="
echo "mrboard has suspended and handed the terminal to this script."
echo "args: $*"
echo "================================"

sleep 5
