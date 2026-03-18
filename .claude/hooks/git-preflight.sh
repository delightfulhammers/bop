#!/bin/bash
# Telesis preflight hook for Claude Code
# Gates git commit on orchestrator preflight checks.

if ! command -v jq &>/dev/null; then
  echo "Warning: jq not found, skipping telesis preflight" >&2
  exit 0
fi

INPUT=$(cat)
COMMAND=$(printf '%s' "$INPUT" | jq -r '.tool_input.command // empty')
FIRST_LINE=$(printf '%s' "$COMMAND" | head -1)

if [[ "$FIRST_LINE" =~ (^|[[:space:]]|&&|;)git[[:space:]]+commit([[:space:]]|$) ]]; then
  cd "$CLAUDE_PROJECT_DIR" || exit 0

  if [[ "$FIRST_LINE" == *"--amend"* ]]; then
    CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
    if [ -n "$CURRENT_BRANCH" ]; then
      REMOTE_SHA=$(git rev-parse "origin/$CURRENT_BRANCH" 2>/dev/null)
      LOCAL_SHA=$(git rev-parse HEAD 2>/dev/null)
      if [ "$REMOTE_SHA" = "$LOCAL_SHA" ]; then
        echo "Blocked: git commit --amend on a pushed commit rewrites history." >&2
        echo "Create a new commit instead." >&2
        exit 2
      fi
    fi
  fi

  if command -v telesis &>/dev/null; then
    telesis orchestrator preflight 2>&1
    if [ $? -ne 0 ]; then
      echo "Telesis preflight checks failed. The commit has been blocked." >&2
      exit 2
    fi
  fi
fi

exit 0
