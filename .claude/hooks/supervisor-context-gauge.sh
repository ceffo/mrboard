#!/usr/bin/env bash
# PostToolUse: in a supervisor session, once the context passes 180k tokens, remind it every tool
# call to hand off with .claude/scripts/supervisor-handoff.sh. Other agents are not affected.
set -euo pipefail
t="$(jq -r '.transcript_path // empty')"
[ -f "$t" ] && head -c 20000 "$t" | grep -q '"agentSetting":"supervisor"' || exit 0
# The newest assistant turn with usage is in the transcript's tail (no tac: macOS lacks it).
used="$(tail -n 400 "$t" | jq -r 'select(.type=="assistant" and .message.usage)|.message.usage
  |(.input_tokens+(.cache_read_input_tokens//0)+(.cache_creation_input_tokens//0))' 2>/dev/null | tail -1)"
[ "${used:-0}" -ge 180000 ] || exit 0
msg="Context at $((used / 1000))k (>=180k). At the next quiet point (no rework mid-commit): rewrite the ledger with a 'Notes for the next supervisor (sup-N handoff' section and a Retro, then run .claude/scripts/supervisor-handoff.sh (background). Do not wait for the user."
jq -n --arg m "$msg" '{hookSpecificOutput:{hookEventName:"PostToolUse",additionalContext:$m}}'
