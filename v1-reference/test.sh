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

# run_bats
# Purpose: Execute bats test suite with verbose timing and output on failure.
# Args:
#   $1: Optional "--fast" to enable parallel execution when available.
# Output: Bats output to stdout/stderr.
# Returns: Exit code from bats.
run_bats() {
  local mode="${1:-}"
  if [[ "${mode}" == "--fast" && $(command -v parallel >/dev/null 2>&1; printf '%s' "$?") -eq 0 ]]; then
    local jobs
    jobs="$(detect_logical_cores)"
    echo "Running bats in parallel with ${jobs} jobs..."
    bats -xT --print-output-on-failure --jobs "${jobs}" "${ROOT_DIR}/tests"
  else
    echo "Running bats serially..."
    bats -xT --print-output-on-failure "${ROOT_DIR}/tests"
  fi
}

mode="serial"
seen_fast=0
for arg in "$@"; do
  case "${arg}" in
    --fast)
      seen_fast=1
      mode="fast"
      ;;
    *)
      printf 'Usage: %s [--fast]\n' "$0" >&2
      exit 1
      ;;
  esac
done

if [[ "${mode}" == "fast" ]]; then
  run_bats --fast
else
  run_bats
fi
