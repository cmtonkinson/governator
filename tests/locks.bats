#!/usr/bin/env bats

load ./helpers.bash

@test "lock command writes a lock file and reports active snapshot" {
  run bash "${REPO_DIR}/_governator/governator.sh" lock
  [ "$status" -eq 0 ]
  [ -f "${REPO_DIR}/.governator/governator.locked" ]
}

@test "locked state stops assign-backlog" {
  write_task "task-backlog" "018-lock-test-ruby"
  commit_all "Add lock test task"

  run bash "${REPO_DIR}/_governator/governator.sh" lock
  [ "$status" -eq 0 ]

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]
  run grep -F "Governator is locked" <<< "${output}"
  [ "$status" -eq 0 ]
  [ -f "${REPO_DIR}/_governator/task-backlog/018-lock-test-ruby.md" ]
}

@test "unlock removes the lock file" {
  run bash "${REPO_DIR}/_governator/governator.sh" lock
  [ "$status" -eq 0 ]

  run bash "${REPO_DIR}/_governator/governator.sh" unlock
  [ "$status" -eq 0 ]
  [ ! -f "${REPO_DIR}/.governator/governator.locked" ]
}
