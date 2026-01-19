#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_DIR="${ROOT_DIR}/_governator"

rename_task_files() {
  local dir="$1"
  local old_base="$2"
  local new_base="$3"
  local old_file="${dir}/${old_base}.md"
  if [[ -f "${old_file}" ]]; then
    mv "${old_file}" "${dir}/${new_base}.md"
  fi
  local path
  shopt -s nullglob
  for path in "${dir}/${old_base}-"*.md; do
    local base="${path##*/}"
    local suffix="${base#${old_base}-}"
    mv "${path}" "${dir}/${new_base}-${suffix}"
  done
  shopt -u nullglob
}

for dir in "${STATE_DIR}"/task-*; do
  if [[ ! -d "${dir}" ]]; then
    continue
  fi
  rename_task_files "${dir}" "000-done-check-reviewer" "000-completion-check-reviewer"
  rename_task_files "${dir}" "000-done-check-planner" "000-gap-analysis-planner"
done
