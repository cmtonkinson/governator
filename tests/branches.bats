#!/usr/bin/env bats

load ./helpers.bash

@test "process-branches approves worked task and moves to done" {
  write_task "task-worked" "010-review-ruby"
  echo "010-review-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  echo "010-review-ruby | ruby | 999999 | /tmp/governator-test | worker/ruby/010-review-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare worked task"

  create_worker_branch "010-review-ruby" "ruby"
  repo_git checkout -b "worker/reviewer/010-review-ruby" "origin/worker/ruby/010-review-ruby" >/dev/null
  cat > "${REPO_DIR}/review.json" <<'EOF_REVIEW'
{"result":"approve","comments":["looks good"]}
EOF_REVIEW
  repo_git add "review.json"
  repo_git commit -m "Review 010-review-ruby" >/dev/null
  repo_git push -u origin "worker/reviewer/010-review-ruby" >/dev/null
  repo_git checkout main >/dev/null

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-done/010-review-ruby.md" ]
  run grep -F "Decision: approve" "${REPO_DIR}/_governator/task-done/010-review-ruby.md"
  [ "$status" -eq 0 ]
  run grep -F "010-review-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
}

@test "process-branches can spawn reviewer when global cap is reached by the worker" {
  write_task "task-worked" "014-review-ruby"
  echo "014-review-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  commit_all "Prepare worked task for reviewer spawn"

  create_worker_branch "014-review-ruby" "ruby"

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  run grep -F "014-review-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
  run grep -F "014-review-ruby -> reviewer" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -eq 0 ]
}

@test "process-branches skips reviewer spawn when reviewer already in-flight" {
  write_task "task-worked" "015-review-ruby"
  echo "015-review-ruby -> reviewer" >> "${REPO_DIR}/.governator/in-flight.log"
  commit_all "Prepare reviewer in-flight"

  create_worker_branch "015-review-ruby" "ruby"

  run bash -c "bash \"${REPO_DIR}/_governator/governator.sh\" process-branches 2>&1"
  [ "$status" -eq 0 ]

  run grep -F "Global worker cap reached" <<< "${output}"
  [ "$status" -ne 0 ]
  run grep -F "015-review-ruby -> reviewer" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -eq 0 ]
}

@test "process-branches keeps blocked tasks and queues unblock planner" {
  write_task "task-blocked" "060-blocked-ruby"
  echo "060-blocked-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  commit_all "Prepare blocked task"

  create_worker_branch "060-blocked-ruby" "ruby"

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/060-blocked-ruby.md" ]
  [ -f "${REPO_DIR}/_governator/task-assigned/000-unblock-planner.md" ]
  run grep -F "060-blocked-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
  [ ! -f "${ORIGIN_DIR}/refs/heads/worker/ruby/060-blocked-ruby" ]
}

@test "process_worker_branch clears in-flight entry when branch is missing" {
  write_task "task-assigned" "011-missing-ruby"
  echo "011-missing-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"

  project_name="$(basename "${REPO_DIR}")"
  tmp_dir="$(mktemp -d "/tmp/governator-${project_name}-ruby-011-missing-ruby-XXXXXX")"
  echo "011-missing-ruby | ruby | 999999 | ${tmp_dir} | worker/ruby/011-missing-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare missing branch task"

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    PROJECT_NAME=\"${project_name}\"
    DEFAULT_REMOTE_NAME=\"origin\"
    DEFAULT_BRANCH_NAME=\"main\"
    CONFIG_FILE=\"\${DB_DIR}/config.json\"
    IN_FLIGHT_LOG=\"\${DB_DIR}/in-flight.log\"
    RETRY_COUNTS_LOG=\"\${DB_DIR}/retry-counts.log\"
    WORKER_PROCESSES_LOG=\"\${DB_DIR}/worker-processes.log\"
    FAILED_MERGES_LOG=\"\${DB_DIR}/failed-merges.log\"
    AUDIT_LOG=\"\${DB_DIR}/audit.log\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    source \"\${STATE_DIR}/lib/utils.sh\"
    source \"\${STATE_DIR}/lib/logging.sh\"
    source \"\${STATE_DIR}/lib/config.sh\"
    source \"\${STATE_DIR}/lib/git.sh\"
    source \"\${STATE_DIR}/lib/tasks.sh\"
    source \"\${STATE_DIR}/lib/workers.sh\"
    source \"\${STATE_DIR}/lib/branches.sh\"
    process_worker_branch \"origin/worker/ruby/011-missing-ruby\"
  "
  [ "$status" -eq 0 ]

  run grep -F "011-missing-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
  run grep -F "011-missing-ruby | ruby" "${REPO_DIR}/.governator/worker-processes.log"
  [ "$status" -ne 0 ]
  [ ! -d "${tmp_dir}" ]
}
