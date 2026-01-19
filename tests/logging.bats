#!/usr/bin/env bats

load ./helpers.bash

@test "audit-log appends entries" {
  run bash "${REPO_DIR}/_governator/governator.sh" audit-log "016-audit" "did something"
  [ "$status" -eq 0 ]
  run grep -F "016-audit -> did something" "${REPO_DIR}/_governator/_local_state/audit.log"
  [ "$status" -eq 0 ]
}
