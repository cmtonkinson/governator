#!/usr/bin/env bats

load ./helpers.bash

@test "status command notes the locked state" {
  run bash "${REPO_DIR}/_governator/governator.sh" lock
  [ "$status" -eq 0 ]
  run bash "${REPO_DIR}/_governator/governator.sh" status
  [ "$status" -eq 0 ]
  run grep -F "LOCKED" <<< "${output}"
  [ "$status" -eq 0 ]
}

@test "status lists only tracked in-flight worker branches" {
  create_worker_branch "020-status-ruby" "ruby"
  create_worker_branch "021-other-ruby" "ruby"
  printf '%s -> %s\n' "020-status-ruby" "ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  commit_paths "Add status in-flight" ".governator/in-flight.log"

  run bash "${REPO_DIR}/_governator/governator.sh" status
  local status_output="${output}"
  [ "$status" -eq 0 ]
  run grep -F "Pending worker branches:" <<< "${status_output}"
  [ "$status" -eq 0 ]
  run grep -F "origin/worker/ruby/020-status-ruby" <<< "${status_output}"
  [ "$status" -eq 0 ]
  run grep -F "origin/worker/ruby/021-other-ruby" <<< "${status_output}"
  [ "$status" -ne 0 ]
}

@test "status reports project done when checks are up to date" {
  done_sha="$(file_sha256 "${REPO_DIR}/GOVERNATOR.md")"
  set_config_value "planning.gov_hash" "deadbeef" "string"
  commit_paths "Set stale planning hash" ".governator/config.json"
  set_config_value "planning.gov_hash" "${done_sha}" "string"
  commit_paths "Set project done" ".governator/config.json"

  run bash "${REPO_DIR}/_governator/governator.sh" status
  local status_output="${output}"
  [ "$status" -eq 0 ]
  run grep -F "Project status: DONE" <<< "${status_output}"
  [ "$status" -eq 0 ]
}

@test "status summarizes milestone and epic progress" {
  cat > "${REPO_DIR}/_governator/task-done/030-done-ruby.md" <<'EOF_TASK'
---
milestone: m1
epic: e1
---
# Task
EOF_TASK
  cat > "${REPO_DIR}/_governator/task-assigned/031-pending-ruby.md" <<'EOF_TASK'
---
milestone: m1
epic: e2
---
# Task
EOF_TASK
  commit_paths "Add milestone tasks" \
    "_governator/task-done/030-done-ruby.md" \
    "_governator/task-assigned/031-pending-ruby.md"

  run bash "${REPO_DIR}/_governator/governator.sh" status
  local status_output="${output}"
  [ "$status" -eq 0 ]
  run grep -F "Milestone m1: 50%" <<< "${status_output}"
  [ "$status" -eq 0 ]
  run grep -F "Epic e1: 100%" <<< "${status_output}"
  [ "$status" -eq 0 ]
  run grep -F "Epic e2: 0%" <<< "${status_output}"
  [ "$status" -eq 0 ]
}
