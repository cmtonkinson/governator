#!/usr/bin/env bats

load ./helpers.bash

@test "process-branches approves worked task and moves to done" {
  write_task "task-worked" "010-review-ruby"
  add_in_flight "010-review-ruby" "ruby"
  add_worker_process "010-review-ruby" "ruby" "999999"
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
  run grep -F "010-review-ruby -> ruby" "${REPO_DIR}/_governator/_local_state/in-flight.log"
  [ "$status" -ne 0 ]
}

@test "process-branches can spawn reviewer when global cap is reached by the worker" {
  write_task "task-worked" "014-review-ruby"
  add_in_flight "014-review-ruby" "ruby"
  commit_all "Prepare worked task for reviewer spawn"

  create_worker_branch "014-review-ruby" "ruby"

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  run grep -F "014-review-ruby -> ruby" "${REPO_DIR}/_governator/_local_state/in-flight.log"
  [ "$status" -ne 0 ]
  run grep -F "014-review-ruby -> reviewer" "${REPO_DIR}/_governator/_local_state/in-flight.log"
  [ "$status" -eq 0 ]
}

@test "process-branches creates reviewer branch from worker branch" {
  write_task "task-worked" "018-branch-ruby"
  add_in_flight "018-branch-ruby" "ruby"
  commit_all "Prepare worked task for branch verification"

  # Create worker branch with identifiable commit
  create_worker_branch "018-branch-ruby" "ruby"
  worker_commit="$(repo_git rev-parse worker/ruby/018-branch-ruby)"

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  # Verify reviewer is in-flight
  run grep -F "018-branch-ruby -> reviewer" "${REPO_DIR}/_governator/_local_state/in-flight.log"
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
  add_in_flight "015-review-ruby" "reviewer"
  # Create a worker process record so the reviewer isn't detected as a zombie
  # Use current timestamp and current shell's PID (guaranteed to be running during test)
  worktree_dir="$(create_worktree_dir "015-review-ruby" "reviewer")"
  started_at="$(date +%s)"
  add_worker_process "015-review-ruby" "reviewer" "$$" "${worktree_dir}" "${started_at}"
  commit_all "Prepare reviewer in-flight"

  create_worker_branch "015-review-ruby" "ruby"

  run bash -c "bash \"${REPO_DIR}/_governator/governator.sh\" process-branches 2>&1"
  [ "$status" -eq 0 ]

  run grep -F "Global worker cap reached" <<< "${output}"
  [ "$status" -ne 0 ]
  run grep -F "015-review-ruby -> reviewer" "${REPO_DIR}/_governator/_local_state/in-flight.log"
  [ "$status" -eq 0 ]
}

@test "process-branches keeps blocked tasks and queues unblock planner" {
  write_task "task-blocked" "060-blocked-ruby"
  add_in_flight "060-blocked-ruby" "ruby"
  commit_all "Prepare blocked task"

  create_worker_branch "060-blocked-ruby" "ruby"

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/060-blocked-ruby.md" ]
  [ -f "${REPO_DIR}/_governator/task-assigned/000-unblock-planner.md" ]
  run grep -F "060-blocked-ruby -> ruby" "${REPO_DIR}/_governator/_local_state/in-flight.log"
  [ "$status" -ne 0 ]
  # Verify local branch was deleted after processing
  run repo_git show-ref --verify "refs/heads/worker/ruby/060-blocked-ruby"
  [ "$status" -ne 0 ]
}

@test "process-branches preserves worktree for blocked task with preservation reason" {
  task_name="061-blocked-preserve-ruby"
  worker="ruby"
  worktree_dir="$(create_worktree_dir "${task_name}" "${worker}")"
  cat > "${REPO_DIR}/_governator/task-blocked/${task_name}.md" <<EOF_TASK
# Task
## Governator Block
Worker self-check failed: Worktree has uncommitted changes. Worktree preserved at ${worktree_dir}.
EOF_TASK
  add_in_flight "${task_name}" "${worker}"
  commit_all "Prepare blocked task with preserved worktree"

  create_worker_branch "${task_name}" "${worker}"

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  [ -d "${worktree_dir}" ]
  run repo_git show-ref --verify "refs/heads/worker/ruby/${task_name}"
  [ "$status" -eq 0 ]
  run grep -F "${task_name} -> ${worker}" "${REPO_DIR}/_governator/_local_state/in-flight.log"
  [ "$status" -ne 0 ]
}

@test "process-branches preserves worktree when main task is blocked but branch is stale" {
  task_name="062-blocked-stale-ruby"
  worker="ruby"
  worktree_dir="$(create_worktree_dir "${task_name}" "${worker}")"

  write_task "task-assigned" "${task_name}"
  commit_all "Prepare assigned task"
  create_worker_branch "${task_name}" "${worker}"

  mv "${REPO_DIR}/_governator/task-assigned/${task_name}.md" \
    "${REPO_DIR}/_governator/task-blocked/${task_name}.md"
  cat > "${REPO_DIR}/_governator/task-blocked/${task_name}.md" <<EOF_TASK
# Task
## Governator Block
Worker self-check failed: Worktree has uncommitted changes. Worktree preserved at ${worktree_dir}.
EOF_TASK
  commit_all "Prepare blocked task after branch created"

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  [ -d "${worktree_dir}" ]
  run repo_git show-ref --verify "refs/heads/worker/ruby/${task_name}"
  [ "$status" -eq 0 ]
}

@test "process_worker_branch clears in-flight entry when branch is missing" {
  write_task "task-assigned" "011-missing-ruby"
  add_in_flight "011-missing-ruby" "ruby"
  worktree_dir="$(create_worktree_dir "011-missing-ruby" "ruby")"
  add_worker_process "011-missing-ruby" "ruby" "999999"
  commit_all "Prepare missing branch task"

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/_governator/_local_state\"
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

  run grep -F "011-missing-ruby -> ruby" "${REPO_DIR}/_governator/_local_state/in-flight.log"
  [ "$status" -ne 0 ]
  run grep -F "011-missing-ruby | ruby" "${REPO_DIR}/_governator/_local_state/worker-processes.log"
  [ "$status" -ne 0 ]
  [ ! -d "${worktree_dir}" ]
}
