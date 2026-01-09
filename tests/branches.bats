#!/usr/bin/env bats

load ./helpers.bash

@test "process-branches approves worked task and moves to done" {
  write_task "task-worked" "010-review-ruby"
  echo "010-review-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  echo "010-review-ruby | ruby | 999999 | ${REPO_DIR}/.governator/worktrees/010-review-ruby-ruby | worker/ruby/010-review-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare worked task"

  create_worker_branch "010-review-ruby" "ruby"
  repo_git checkout -b "worker/reviewer/010-review-ruby" "worker/ruby/010-review-ruby" >/dev/null
  cat > "${REPO_DIR}/review.json" <<'EOF_REVIEW'
{"result":"approve","comments":["looks good"]}
EOF_REVIEW
  repo_git add "review.json"
  repo_git commit -m "Review 010-review-ruby" >/dev/null
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

@test "process-branches creates reviewer branch from worker branch" {
  write_task "task-worked" "018-branch-ruby"
  echo "018-branch-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  commit_all "Prepare worked task for branch verification"

  # Create worker branch with identifiable commit
  create_worker_branch "018-branch-ruby" "ruby"
  worker_commit="$(repo_git rev-parse worker/ruby/018-branch-ruby)"

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  # Verify reviewer is in-flight
  run grep -F "018-branch-ruby -> reviewer" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -eq 0 ]

  # Verify worker branch still exists (not deleted before reviewer could use it)
  run repo_git show-ref --verify "refs/heads/worker/ruby/018-branch-ruby"
  [ "$status" -eq 0 ]

  # Verify reviewer branch exists
  run repo_git show-ref --verify "refs/heads/worker/reviewer/018-branch-ruby"
  [ "$status" -eq 0 ]

  # Verify reviewer branch is based on worker branch (contains worker's commit)
  run repo_git merge-base --is-ancestor "${worker_commit}" "worker/reviewer/018-branch-ruby"
  [ "$status" -eq 0 ]
}

@test "process-branches skips reviewer spawn when reviewer already in-flight" {
  write_task "task-worked" "015-review-ruby"
  echo "015-review-ruby -> reviewer" >> "${REPO_DIR}/.governator/in-flight.log"
  # Create a worker process record so the reviewer isn't detected as a zombie
  # Use worktree as working dir and create the reviewer branch so it looks like an active worker
  worktree_dir="${REPO_DIR}/.governator/worktrees/015-review-ruby-reviewer"
  mkdir -p "${worktree_dir}"
  # Use current timestamp and current shell's PID (guaranteed to be running during test)
  started_at="$(date +%s)"
  echo "015-review-ruby | reviewer | $$ | ${worktree_dir} | worker/reviewer/015-review-ruby | ${started_at}" >> "${REPO_DIR}/.governator/worker-processes.log"
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
  # Verify local branch was deleted after processing
  run repo_git show-ref --verify "refs/heads/worker/ruby/060-blocked-ruby"
  [ "$status" -ne 0 ]
}

@test "process_worker_branch clears in-flight entry when branch is missing" {
  write_task "task-assigned" "011-missing-ruby"
  echo "011-missing-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"

  worktree_dir="${REPO_DIR}/.governator/worktrees/011-missing-ruby-ruby"
  mkdir -p "${worktree_dir}"
  echo "011-missing-ruby | ruby | 999999 | ${worktree_dir} | worker/ruby/011-missing-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare missing branch task"

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    WORKTREES_DIR=\"\${DB_DIR}/worktrees\"
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
    source \"\${STATE_DIR}/lib/worktrees.sh\"
    source \"\${STATE_DIR}/lib/tasks.sh\"
    source \"\${STATE_DIR}/lib/workers.sh\"
    source \"\${STATE_DIR}/lib/branches.sh\"
    process_worker_branch \"worker/ruby/011-missing-ruby\"
  "
  [ "$status" -eq 0 ]

  run grep -F "011-missing-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
  run grep -F "011-missing-ruby | ruby" "${REPO_DIR}/.governator/worker-processes.log"
  [ "$status" -ne 0 ]
  [ ! -d "${worktree_dir}" ]
}
