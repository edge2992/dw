#!/usr/bin/env bash
# dw: Stop hook that checkpoints the session into .dw/last-session.md.
#
# Deterministic extraction only — no LLM calls. Claude Code hands us the turn's
# final message on stdin, so there is nothing to summarize and nothing to wait
# for. A Stop hook that exits non-zero tells Claude to keep going, so every
# failure path here has to fall through to a silent `exit 0`.
set -u

checkpoint() {
  set -e
  # Parsing JSON without jq is not worth attempting: the message is free-form
  # text with newlines and quotes in it. No jq, no checkpoint.
  command -v jq >/dev/null 2>&1 || return 0

  # CLAUDE_PROJECT_DIR, not the payload's cwd: cwd moves when the session cd's
  # somewhere else, while this stays pinned to the workspace we belong to.
  local project_dir="${CLAUDE_PROJECT_DIR:-}"
  [ -n "$project_dir" ] || return 0

  local input
  input="$(cat)"

  local session_id message ended_at
  session_id="$(printf '%s' "$input" | jq -r '.session_id // empty')"
  message="$(printf '%s' "$input" | jq -r '.last_assistant_message // empty')"
  [ -n "$message" ] || return 0
  # Formatting the current time only — no date parsing, so BSD and GNU date
  # take the same arguments.
  ended_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  local dw_dir="$project_dir/.dw"
  mkdir -p "$dw_dir"

  # Write-then-rename, so a reader never catches a half-written checkpoint.
  local tmp
  tmp="$(mktemp "$dw_dir/.last-session.XXXXXX")"
  {
    printf -- '---\n'
    printf 'session_id: %s\n' "$session_id"
    printf 'ended_at: %s\n' "$ended_at"
    printf -- '---\n\n'
    printf '%s\n' "$message"
  } >"$tmp"
  mv -f "$tmp" "$dw_dir/last-session.md"
}

# The subshell keeps `set -e` from escaping into the exit status below.
( checkpoint ) >/dev/null 2>&1 || true
exit 0
