#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COVERAGE_DIR="${ROOT_DIR}/coverage"

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
# Purpose: Execute bats test suite with optional parallelization.
# Args: None.
# Output: Bats output to stdout/stderr.
# Returns: Exit code from bats.
run_bats() {
  if command -v parallel >/dev/null 2>&1; then
    local jobs
    jobs="$(detect_logical_cores)"
    bats --jobs "${jobs}" "${ROOT_DIR}/tests"
  else
    bats "${ROOT_DIR}/tests"
  fi
}

# run_bats_with_kcov
# Purpose: Execute bats under kcov and emit CLI + Cobertura coverage reports.
# Args: None.
# Output: kcov and bats output; writes coverage artifacts to ${COVERAGE_DIR}.
# Returns: Exit code from kcov.
run_bats_with_kcov() {
  require_deps kcov
  mkdir -p "${COVERAGE_DIR}"

  local bats_args=("${ROOT_DIR}/tests")
  if command -v parallel >/dev/null 2>&1; then
    local jobs
    jobs="$(detect_logical_cores)"
    bats_args=(--jobs "${jobs}" "${ROOT_DIR}/tests")
  fi

  kcov \
    --include-path="${ROOT_DIR}" \
    --exclude-path="${ROOT_DIR}/tests" \
    "${COVERAGE_DIR}" \
    bats "${bats_args[@]}"
}

if [[ "${COVERAGE:-0}" == "1" ]]; then
  run_bats_with_kcov
else
  run_bats
fi
