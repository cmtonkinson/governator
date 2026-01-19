#!/usr/bin/env bash
set -euo pipefail

# Ensure we are our own process group so we can kill everything
set -m
PGID=$(ps -o pgid= $$ | tr -d ' ')

cleanup() {
  # Kill the entire process group (including codex children)
  kill -TERM -"$PGID" 2>/dev/null || true
  exit 130
}

trap cleanup INT TERM

INDEX_FILE="${INDEX_FILE:-TODO/index.md}"

WORK_PROMPT_FILE="${WORK_PROMPT_FILE:-/Users/chris/vault/vibe/ralph-work.md}"
REVIEW_PROMPT_FILE="${REVIEW_PROMPT_FILE:-/Users/chris/vault/vibe/ralph-review.md}"

CODEX_BIN="${CODEX_BIN:-codex}"
CODEX_ARGS=(exec --full-auto --sandbox workspace-write)

REVIEW_MAX_ATTEMPTS="${REVIEW_MAX_ATTEMPTS:-10}"
SLEEP_BETWEEN_ATTEMPTS_SEC="${SLEEP_BETWEEN_ATTEMPTS_SEC:-0}"

has_word() {
  local word="$1"
  grep -qE "(^|[^A-Za-z0-9_])${word}([^A-Za-z0-9_]|$)" "$INDEX_FILE"
}

run_codex_prompt_file() {
  local prompt_file="$1"
  echo "=================================================================="
  echo "=================================================================="
  echo "=================================================================="
  echo "=================================================================="
  echo "Running codex prompt file: $prompt_file"
  "$CODEX_BIN" "${CODEX_ARGS[@]}" "$(cat "$prompt_file")"
}

while has_word "open" || has_word "worked"; do
  # If there's any 'open', do one work pass.
  if has_word "open"; then
    run_codex_prompt_file "$WORK_PROMPT_FILE"
  fi

  # If there's any 'worked', keep reviewing until it prints "task complete"
  if has_word "worked"; then
    attempt=1
    while has_word "worked"; do
      # Capture stdout for the single check, but do not suppress it from the terminal:
      # - we tee it to stderr (so you still see it)
      # - we grep the tee stream to detect "task complete"
      if run_codex_prompt_file "$REVIEW_PROMPT_FILE" \
          2> >(tee /dev/stderr) \
          | tee /dev/stderr \
          | grep -qFx "task complete"; then
        break
      fi

      if (( attempt >= REVIEW_MAX_ATTEMPTS )); then
        echo "Review did not reach 'task complete' after ${REVIEW_MAX_ATTEMPTS} attempts." >&2
        exit 2
      fi
      attempt=$((attempt + 1))

      if (( SLEEP_BETWEEN_ATTEMPTS_SEC > 0 )); then
        sleep "$SLEEP_BETWEEN_ATTEMPTS_SEC"
      fi
    done
  fi
done
