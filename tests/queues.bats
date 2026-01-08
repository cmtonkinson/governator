#!/usr/bin/env bats

load ./helpers.bash

@test "assign-backlog assigns task and logs in-flight" {
  complete_bootstrap
  write_task "task-backlog" "001-sample-ruby"
  commit_all "Add backlog task"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/001-sample-ruby.md" ]
  run grep -F "001-sample-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -eq 0 ]
}

@test "assign-backlog queues gap-analysis planner on GOVERNATOR changes" {
  complete_bootstrap
  set_config_value "planning.gov_hash" "deadbeef"
  commit_paths "Set stale planning hash" ".governator/config.json"
  write_task "task-backlog" "001-sample-ruby"
  commit_all "Add backlog task"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/000-gap-analysis-planner.md" ]
  [ -f "${REPO_DIR}/_governator/task-backlog/001-sample-ruby.md" ]
}

@test "run skips gap-analysis planner before bootstrap completes" {
  set_config_value "planning.gov_hash" "deadbeef"
  commit_paths "Set stale planning hash" ".governator/config.json"

  run bash "${REPO_DIR}/_governator/governator.sh" run
  [ "$status" -eq 0 ]

  [ ! -f "${REPO_DIR}/_governator/task-assigned/000-gap-analysis-planner.md" ]
}

@test "assign-backlog blocks tasks missing a role suffix" {
  complete_bootstrap
  write_task "task-backlog" "002norole"
  commit_all "Add missing role task"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/002norole.md" ]
  run grep -F "Missing required role" "${REPO_DIR}/_governator/task-blocked/002norole.md"
  [ "$status" -eq 0 ]
}

@test "assign-backlog blocks tasks with unknown roles" {
  complete_bootstrap
  write_task "task-backlog" "003-unknown-ghost"
  commit_all "Add unknown role task"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/003-unknown-ghost.md" ]
  run grep -F "Unknown role ghost" "${REPO_DIR}/_governator/task-blocked/003-unknown-ghost.md"
  [ "$status" -eq 0 ]
}

@test "assign-backlog respects global cap" {
  complete_bootstrap
  write_task "task-backlog" "004-cap-ruby"
  echo "004-busy-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  commit_all "Prepare global cap"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-backlog/004-cap-ruby.md" ]
  [ ! -f "${REPO_DIR}/_governator/task-assigned/004-cap-ruby.md" ]
}

@test "assign-backlog respects per-worker cap" {
  complete_bootstrap
  write_task "task-backlog" "005-cap-ruby"
  echo "006-busy-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  set_config_map_value "worker_caps" "global" "2" "number"
  set_config_map_value "worker_caps" "ruby" "1" "number"
  commit_all "Prepare worker cap"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-backlog/005-cap-ruby.md" ]
  [ ! -f "${REPO_DIR}/_governator/task-assigned/005-cap-ruby.md" ]
}

@test "assign-backlog defers tasks with unmet dependencies" {
  complete_bootstrap
  set_config_map_value "worker_caps" "global" "2" "number"
  write_task_with_frontmatter "task-backlog" "001-dependency-ruby" $'milestone:\nepic:\ntask:\ndepends_on: []'
  write_task_with_frontmatter "task-backlog" "002-dependent-ruby" $'milestone:\nepic:\ntask:\ndepends_on: [001]'
  commit_all "Add dependency tasks"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/001-dependency-ruby.md" ]
  [ -f "${REPO_DIR}/_governator/task-backlog/002-dependent-ruby.md" ]
}

@test "assign-backlog supports quoted and unpadded dependency ids" {
  complete_bootstrap
  set_config_map_value "worker_caps" "global" "2" "number"
  write_task "task-done" "001-base-ruby"
  write_task_with_frontmatter "task-backlog" "002-dependent-ruby" $'milestone:\nepic:\ntask:\ndepends_on: ["1"]'
  commit_all "Add quoted dependency tasks"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/002-dependent-ruby.md" ]
}

@test "assign-backlog supports multiline dependency lists" {
  complete_bootstrap
  set_config_map_value "worker_caps" "global" "2" "number"
  write_task "task-done" "003-base-ruby"
  write_task_with_frontmatter "task-backlog" "004-dependent-ruby" $'milestone:\nepic:\ntask:\ndepends_on:\n  - 003'
  commit_all "Add multiline dependency tasks"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/004-dependent-ruby.md" ]
}

@test "assign-backlog gates later milestones until earlier milestones complete" {
  complete_bootstrap
  set_config_map_value "worker_caps" "global" "2" "number"
  write_task_with_frontmatter "task-backlog" "010-m0-ruby" $'milestone: M0\nepic:\ntask:\ndepends_on: []'
  write_task_with_frontmatter "task-backlog" "011-m1-ruby" $'milestone: M1\nepic:\ntask:\ndepends_on: []'
  commit_all "Add milestone tasks"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/010-m0-ruby.md" ]
  [ -f "${REPO_DIR}/_governator/task-backlog/011-m1-ruby.md" ]
}

@test "assign-backlog skips completion check during cooldown" {
  complete_bootstrap
  set_config_value "planning.gov_hash" "deadbeef"
  set_config_value "done_check.last_check" "$(date +%s)" "number"
  commit_all "Prepare completion cooldown state"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ ! -f "${REPO_DIR}/_governator/task-assigned/000-completion-check-reviewer.md" ]
}

@test "assign-backlog creates unblock planner task for blocked tasks" {
  complete_bootstrap
  write_task "task-blocked" "040-blocked-ruby"
  commit_all "Add blocked task"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/000-unblock-planner.md" ]
  run grep -F "040-blocked-ruby" "${REPO_DIR}/_governator/task-assigned/000-unblock-planner.md"
  [ "$status" -eq 0 ]
}

@test "assign-backlog skips unblock planner task after analysis" {
  complete_bootstrap
  write_task "task-blocked" "041-blocked-ruby"
  cat >> "${REPO_DIR}/_governator/task-blocked/041-blocked-ruby.md" <<'EOF_BLOCK'

## Unblock Analysis

2026-01-01T00:00:00Z [planner]: Needs clarification.
EOF_BLOCK
  commit_all "Add analyzed blocked task"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ ! -f "${REPO_DIR}/_governator/task-assigned/000-unblock-planner.md" ]
}
