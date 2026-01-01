#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=./common.sh
source "${ROOT_DIR}/scripts/common.sh"
require_deps shellcheck shfmt

shellcheck "${ROOT_DIR}/_governator/governator.sh"
shfmt -d -i 2 -ci -sr "${ROOT_DIR}/_governator/governator.sh"
