#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=./common.sh
source "${ROOT_DIR}/scripts/common.sh"
require_deps bats

# detect_logical_cores
# Purpose: Determine logical CPU core count for parallel test runs.
# Args: None.
# Output: Prints core count to stdout.
# Returns: 0 when a value is printed.
detect_logical_cores() {
  local cores=""
  if command -v nproc >/dev/null 2>&1; then
    cores="$(nproc)"
  elif command -v sysctl >/dev/null 2>&1; then
    cores="$(sysctl -n hw.logicalcpu 2>/dev/null || true)"
  elif command -v getconf >/dev/null 2>&1; then
    cores="$(getconf _NPROCESSORS_ONLN 2>/dev/null || true)"
  fi
  if [[ -z "${cores}" || ! "${cores}" =~ ^[0-9]+$ || "${cores}" -lt 1 ]]; then
    cores=1
  fi
  printf '%s\n' "${cores}"
}

if command -v parallel >/dev/null 2>&1; then
  jobs="$(detect_logical_cores)"
  bats --jobs "${jobs}" "${ROOT_DIR}/tests"
else
  bats "${ROOT_DIR}/tests"
fi
