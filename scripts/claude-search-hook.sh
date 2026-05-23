#!/usr/bin/env bash
# Claude Code Stop hook — incremental sync into the claude-search index.
# Add to ~/.claude/settings.json:
#   {
#     "hooks": {
#       "Stop": [{
#         "matcher": "",
#         "hooks": [
#           { "type": "command", "command": "/path/to/claude-search-hook.sh" }
#         ]
#       }]
#     }
#   }
#
# Output is suppressed; failures are logged to ~/.claude/claude-search-hook.log.
set -euo pipefail
LOG="${HOME}/.claude/claude-search-hook.log"
BIN="${CLAUDE_SEARCH_BIN:-claude-search}"
{
  date "+%FT%T %z run"
  "$BIN" import --embed-tool-output small || echo "import failed (exit $?)"
  if [ -n "${OPENAI_API_KEY:-}" ]; then
    "$BIN" embed || echo "embed failed (exit $?)"
  else
    echo "skip embed: no OPENAI_API_KEY"
  fi
} >>"$LOG" 2>&1 &
exit 0
