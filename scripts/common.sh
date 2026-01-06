#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# require_deps
# Purpose: Ensure named development tool binaries are available before running scripts.
# Args:
#   $@: List of command names to validate (strings).
# Output: Writes missing dependency list to stderr on failure.
# Returns: 0 when all dependencies exist; exits 1 if any are missing.
require_deps() {
  local missing=()
  local dep
  for dep in "$@"; do
    if ! command -v "${dep}" >/dev/null 2>&1; then
      missing+=("${dep}")
    fi
  done

  if [[ "${#missing[@]}" -gt 0 ]]; then
    printf 'Missing dev dependencies: %s\n' "${missing[*]}" >&2
    exit 1
  fi
}
