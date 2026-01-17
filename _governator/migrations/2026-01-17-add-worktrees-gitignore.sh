#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GITIGNORE_PATH="${ROOT_DIR}/.gitignore"
ENTRY=".governator/worktrees/"

if [[ ! -f "${GITIGNORE_PATH}" ]]; then
  printf '# Governator\n' > "${GITIGNORE_PATH}"
fi

if ! grep -Fqx -- "${ENTRY}" "${GITIGNORE_PATH}" 2> /dev/null; then
  printf '%s\n' "${ENTRY}" >> "${GITIGNORE_PATH}"
fi
