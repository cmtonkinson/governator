#!/usr/bin/env bats

load ./helpers.bash

@test "list-workers includes reviewer and includes known roles" {
  run bash "${REPO_DIR}/_governator/governator.sh" list-workers
  local workers_output="${output}"
  run grep -F "reviewer" <<< "${workers_output}"
  [ "$status" -eq 0 ]
  run grep -F "ruby" <<< "${workers_output}"
  [ "$status" -eq 0 ]
}

@test "extract-role returns suffix and rejects missing hyphen" {
  write_task "task-backlog" "012-sample-ruby"
  write_task "task-backlog" "013norole"
  commit_all "Add extract-role tasks"

  run bash "${REPO_DIR}/_governator/governator.sh" extract-role "${REPO_DIR}/_governator/task-backlog/012-sample-ruby.md"
  [ "$status" -eq 0 ]
  [ "${output}" = "ruby" ]

  run bash "${REPO_DIR}/_governator/governator.sh" extract-role "${REPO_DIR}/_governator/task-backlog/013norole.md"
  [ "$status" -ne 0 ]
}

@test "format-task-id pads to three digits" {
  run bash "${REPO_DIR}/_governator/governator.sh" format-task-id 1
  [ "$status" -eq 0 ]
  [ "${output}" = "001" ]
  run bash "${REPO_DIR}/_governator/governator.sh" format-task-id 12
  [ "$status" -eq 0 ]
  [ "${output}" = "012" ]
  run bash "${REPO_DIR}/_governator/governator.sh" format-task-id 123
  [ "$status" -eq 0 ]
  [ "${output}" = "123" ]
}

@test "allocate-task-id increments counter" {
  set_next_task_id "7"

  run bash "${REPO_DIR}/_governator/governator.sh" allocate-task-id
  [ "$status" -eq 0 ]
  [ "${output}" = "7" ]
  commit_paths "Bump task id" ".governator/config.json"
  run bash "${REPO_DIR}/_governator/governator.sh" allocate-task-id
  [ "$status" -eq 0 ]
  [ "${output}" = "8" ]
}

@test "abort terminates worker, removes tmp dir, and blocks task" {
  write_task "task-assigned" "019-abort-ruby"
  printf '%s\n' "019-abort-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  tmp_dir="$(mktemp -d "${BATS_TMPDIR}/worker-XXXXXX")"
  sleep 60 >/dev/null &
  pid=$!
  printf '%s | %s | %s | %s | worker/ruby/019-abort-ruby | 0\n' "019-abort-ruby" "ruby" "${pid}" "${tmp_dir}" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare abort task"

  create_worker_branch "019-abort-ruby" "ruby"

  run bash "${REPO_DIR}/_governator/governator.sh" abort 019
  [ "$status" -eq 0 ]

  run kill -0 "${pid}" >/dev/null 2>&1
  [ "$status" -ne 0 ]
  kill -9 "${pid}" >/dev/null 2>&1 || true
  wait "${pid}" >/dev/null 2>&1 || true

  [ ! -d "${tmp_dir}" ]
  [ -f "${REPO_DIR}/_governator/task-blocked/019-abort-ruby.md" ]
  run grep -F "## Abort" "${REPO_DIR}/_governator/task-blocked/019-abort-ruby.md"
  [ "$status" -eq 0 ]
  run grep -F "Aborted by operator" "${REPO_DIR}/_governator/task-blocked/019-abort-ruby.md"
  [ "$status" -eq 0 ]

  run grep -F "019-abort-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]

  [ ! -f "${ORIGIN_DIR}/refs/heads/worker/ruby/019-abort-ruby" ]
}

@test "unblock moves blocked task to assigned with note" {
  write_task "task-blocked" "022-unblock-ruby"
  commit_all "Add blocked task"

  run bash "${REPO_DIR}/_governator/governator.sh" unblock "022" "Needs another pass"
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/022-unblock-ruby.md" ]
  [ ! -f "${REPO_DIR}/_governator/task-blocked/022-unblock-ruby.md" ]
  run grep -F "Unblock Note" "${REPO_DIR}/_governator/task-assigned/022-unblock-ruby.md"
  [ "$status" -eq 0 ]
  run grep -F "Needs another pass" "${REPO_DIR}/_governator/task-assigned/022-unblock-ruby.md"
  [ "$status" -eq 0 ]
}

@test "restart resets tasks to backlog and truncates notes for multiple prefixes" {
  cat > "${REPO_DIR}/_governator/task-assigned/030-restart-ruby.md" <<'EOF_TASK'
# Task
## Notes
Assigned note
## Assignment
should be removed
EOF_TASK
  cat > "${REPO_DIR}/_governator/task-blocked/031-restart-ruby.md" <<'EOF_TASK'
# Task
## Notes
Blocked note
## Governator Block
should be removed
EOF_TASK

  tmp_dir="$(mktemp -d "${BATS_TMPDIR}/worker-XXXXXX")"
  sleep 60 >/dev/null &
  pid=$!
  printf '%s | %s | %s | %s | %s | %s\n' \
    "030-restart-ruby" "ruby" "${pid}" "${tmp_dir}" "worker/ruby/030-restart-ruby" "0" \
    >> "${REPO_DIR}/.governator/worker-processes.log"
  printf '%s -> %s\n' "030-restart-ruby" "ruby" >> "${REPO_DIR}/.governator/in-flight.log"

  commit_all "Prepare restart tasks"
  create_worker_branch "030-restart-ruby" "ruby"

  run bash "${REPO_DIR}/_governator/governator.sh" restart 030 031
  [ "$status" -eq 0 ]

  run kill -0 "${pid}" >/dev/null 2>&1
  [ "$status" -ne 0 ]
  kill -9 "${pid}" >/dev/null 2>&1 || true
  wait "${pid}" >/dev/null 2>&1 || true

  [ ! -d "${tmp_dir}" ]
  [ -f "${REPO_DIR}/_governator/task-backlog/030-restart-ruby.md" ]
  [ -f "${REPO_DIR}/_governator/task-backlog/031-restart-ruby.md" ]
  [ ! -f "${REPO_DIR}/_governator/task-assigned/030-restart-ruby.md" ]
  [ ! -f "${REPO_DIR}/_governator/task-blocked/031-restart-ruby.md" ]

  run grep -F "## Notes" "${REPO_DIR}/_governator/task-backlog/030-restart-ruby.md"
  [ "$status" -eq 0 ]
  run grep -F "Assigned note" "${REPO_DIR}/_governator/task-backlog/030-restart-ruby.md"
  [ "$status" -ne 0 ]
  run grep -F "Assignment" "${REPO_DIR}/_governator/task-backlog/030-restart-ruby.md"
  [ "$status" -ne 0 ]

  run grep -F "## Notes" "${REPO_DIR}/_governator/task-backlog/031-restart-ruby.md"
  [ "$status" -eq 0 ]
  run grep -F "Blocked note" "${REPO_DIR}/_governator/task-backlog/031-restart-ruby.md"
  [ "$status" -ne 0 ]
  run grep -F "Governator Block" "${REPO_DIR}/_governator/task-backlog/031-restart-ruby.md"
  [ "$status" -ne 0 ]

  run grep -F "030-restart-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
  run grep -F "030-restart-ruby | ruby" "${REPO_DIR}/.governator/worker-processes.log"
  [ "$status" -ne 0 ]

  [ ! -f "${ORIGIN_DIR}/refs/heads/worker/ruby/030-restart-ruby" ]
}

@test "restart --dry-run reports changes without mutating tasks" {
  cat > "${REPO_DIR}/_governator/task-assigned/032-restart-ruby.md" <<'EOF_TASK'
# Task
## Notes
Keep me
## Assignment
keep annotation
EOF_TASK

  printf '%s | %s | %s | %s | %s | %s\n' \
    "032-restart-ruby" "ruby" "99999" "/tmp/governator-test" "worker/ruby/032-restart-ruby" "0" \
    >> "${REPO_DIR}/.governator/worker-processes.log"
  printf '%s -> %s\n' "032-restart-ruby" "ruby" >> "${REPO_DIR}/.governator/in-flight.log"

  commit_all "Prepare dry-run restart task"

  run bash "${REPO_DIR}/_governator/governator.sh" restart --dry-run 032
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/032-restart-ruby.md" ]
  [ ! -f "${REPO_DIR}/_governator/task-backlog/032-restart-ruby.md" ]

  run grep -F "Keep me" "${REPO_DIR}/_governator/task-assigned/032-restart-ruby.md"
  [ "$status" -eq 0 ]
  run grep -F "Assignment" "${REPO_DIR}/_governator/task-assigned/032-restart-ruby.md"
  [ "$status" -eq 0 ]

  run grep -F "032-restart-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -eq 0 ]
  run grep -F "032-restart-ruby | ruby" "${REPO_DIR}/.governator/worker-processes.log"
  [ "$status" -eq 0 ]
}

@test "archive_done_system_tasks archives done 000 tasks with timestamp" {
  complete_bootstrap
  write_task "task-done" "000-completion-check-reviewer"
  write_task "task-done" "023-keep-ruby"
  commit_all "Prepare done system tasks"

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    AUDIT_LOG=\"${REPO_DIR}/.governator/audit.log\"
    CONFIG_FILE=\"${REPO_DIR}/.governator/config.json\"
    DEFAULT_REMOTE_NAME=\"origin\"
    DEFAULT_BRANCH_NAME=\"main\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    source \"${REPO_DIR}/_governator/lib/utils.sh\"
    source \"${REPO_DIR}/_governator/lib/logging.sh\"
    source \"${REPO_DIR}/_governator/lib/config.sh\"
    source \"${REPO_DIR}/_governator/lib/git.sh\"
    source \"${REPO_DIR}/_governator/lib/tasks.sh\"
    archive_done_system_tasks
  "
  [ "$status" -eq 0 ]

  [ ! -f "${REPO_DIR}/_governator/task-done/000-architecture-bootstrap-architect.md" ]
  [ ! -f "${REPO_DIR}/_governator/task-done/000-completion-check-reviewer.md" ]
  [ -f "${REPO_DIR}/_governator/task-done/023-keep-ruby.md" ]

  run find "${REPO_DIR}/_governator/task-archive" -maxdepth 1 -name '000-architecture-bootstrap-architect-*.md' -print
  [ "$status" -eq 0 ]
  [[ "${output}" =~ 000-architecture-bootstrap-architect-[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{2}-[0-9]{2}-[0-9]{2}\.md ]]

  run find "${REPO_DIR}/_governator/task-archive" -maxdepth 1 -name '000-completion-check-reviewer-*.md' -print
  [ "$status" -eq 0 ]
  [[ "${output}" =~ 000-completion-check-reviewer-[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{2}-[0-9]{2}-[0-9]{2}\.md ]]
}

@test "archive_done_system_tasks leaves non-000 tasks in task-done" {
  complete_bootstrap
  write_task "task-done" "024-keep-ruby"
  commit_all "Prepare non-system done task"

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    AUDIT_LOG=\"${REPO_DIR}/.governator/audit.log\"
    CONFIG_FILE=\"${REPO_DIR}/.governator/config.json\"
    DEFAULT_REMOTE_NAME=\"origin\"
    DEFAULT_BRANCH_NAME=\"main\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    source \"${REPO_DIR}/_governator/lib/utils.sh\"
    source \"${REPO_DIR}/_governator/lib/logging.sh\"
    source \"${REPO_DIR}/_governator/lib/config.sh\"
    source \"${REPO_DIR}/_governator/lib/git.sh\"
    source \"${REPO_DIR}/_governator/lib/tasks.sh\"
    archive_done_system_tasks
  "
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-done/024-keep-ruby.md" ]
  run find "${REPO_DIR}/_governator/task-archive" -maxdepth 1 -name '024-keep-ruby-*.md' -print
  [ "$status" -eq 0 ]
  [ -z "${output}" ]
}

@test "bootstrap treats archived bootstrap task as complete" {
  complete_bootstrap

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    AUDIT_LOG=\"${REPO_DIR}/.governator/audit.log\"
    CONFIG_FILE=\"${REPO_DIR}/.governator/config.json\"
    DEFAULT_REMOTE_NAME=\"origin\"
    DEFAULT_BRANCH_NAME=\"main\"
    BOOTSTRAP_TASK_NAME=\"000-architecture-bootstrap-architect\"
    BOOTSTRAP_DOCS_DIR=\"${REPO_DIR}/_governator/docs\"
    BOOTSTRAP_NEW_REQUIRED_ARTIFACTS=(\"asr.md\" \"arc42.md\")
    BOOTSTRAP_NEW_OPTIONAL_ARTIFACTS=(\"personas.md\" \"wardley.md\")
    BOOTSTRAP_EXISTING_REQUIRED_ARTIFACTS=(\"existing-system-discovery.md\")
    BOOTSTRAP_EXISTING_OPTIONAL_ARTIFACTS=()
    GOV_QUIET=1
    GOV_VERBOSE=0
    source \"${REPO_DIR}/_governator/lib/utils.sh\"
    source \"${REPO_DIR}/_governator/lib/logging.sh\"
    source \"${REPO_DIR}/_governator/lib/config.sh\"
    source \"${REPO_DIR}/_governator/lib/git.sh\"
    source \"${REPO_DIR}/_governator/lib/tasks.sh\"
    source \"${REPO_DIR}/_governator/lib/bootstrap.sh\"
    archive_done_system_tasks
    architecture_bootstrap_complete
  "
  [ "$status" -eq 0 ]
}
