#!/usr/bin/env bash
set -euo pipefail

# Loop until index.md contains no "open" tasks. Each codex run should:
# - pick the first open task
# - implement + test + verify
# - mark it closed in the index
# per the combined Ralph prompt.  [oai_citation:0‡ralph.md](sediment://file_00000000c7bc71fd931a79ab783661c4)

INDEX_FILE="${INDEX_FILE:-index.md}"
PROMPT_FILE="${PROMPT_FILE:-/Users/chris/vault/vibe/ralph.md}"

CODEX_BIN="${CODEX_BIN:-codex}"
CODEX_ARGS=(${CODEX_ARGS:-exec --dangerously-bypass-approvals-and-sandbox})

SLEEP_BETWEEN_RUNS_SEC="${SLEEP_BETWEEN_RUNS_SEC:-5}"
MAX_RUNS="${MAX_RUNS:-60}" # 0 = unlimited

# Ctrl-C should kill the entire script and any in-flight codex subprocesses.
set -m
PGID="$(ps -o pgid= $$ | tr -d ' ')"
cleanup() {
  kill -TERM -"${PGID}" 2>/dev/null || true
  exit 130
}
trap cleanup INT TERM

has_open() {
  grep -qE "(^|[^A-Za-z0-9_])open([^A-Za-z0-9_]|$)" "$INDEX_FILE"
}

[[ -f "$INDEX_FILE" ]] || { echo "Missing $INDEX_FILE" >&2; exit 1; }
[[ -f "$PROMPT_FILE" ]] || { echo "Missing $PROMPT_FILE" >&2; exit 1; }
command -v "$CODEX_BIN" >/dev/null 2>&1 || { echo "codex not found: $CODEX_BIN" >&2; exit 1; }

runs=0
while has_open; do
  runs=$((runs + 1))
  if (( MAX_RUNS > 0 && runs > MAX_RUNS )); then
    echo "MAX_RUNS reached (${MAX_RUNS}); exiting." >&2
    exit 2
  fi

  "$CODEX_BIN" "${CODEX_ARGS[@]}" "$(cat "$PROMPT_FILE")"

  if (( SLEEP_BETWEEN_RUNS_SEC > 0 )); then
    echo "Sleeping ${SLEEP_BETWEEN_RUNS_SEC} seconds before next run."
    sleep "$SLEEP_BETWEEN_RUNS_SEC"
  fi
done
