#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

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
