#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=./common.sh
source "${ROOT_DIR}/scripts/common.sh"
require_deps bats

bats "${ROOT_DIR}/tests"
