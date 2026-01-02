#!/usr/bin/env bats

repo_git() {
  git -C "${REPO_DIR}" "$@"
}

commit_all() {
  local message="$1"
  repo_git add README.md _governator .governator
  repo_git commit -m "${message}" >/dev/null
  repo_git push origin main >/dev/null
}

commit_paths() {
  local message="$1"
  shift
  repo_git add "$@"
  repo_git commit -m "${message}" >/dev/null
  repo_git push origin main >/dev/null
}

write_task() {
  local dir="$1"
  local name="$2"
  cat > "${REPO_DIR}/_governator/${dir}/${name}.md" <<'EOF'
# Task
EOF
}

create_worker_branch() {
  local task_name="$1"
  local worker="$2"
  repo_git checkout -b "worker/${worker}/${task_name}" >/dev/null
  printf '%s\n' "work ${task_name}" > "${REPO_DIR}/work-${task_name}.txt"
  repo_git add "work-${task_name}.txt"
  repo_git commit -m "Work ${task_name}" >/dev/null
  repo_git push -u origin "worker/${worker}/${task_name}" >/dev/null
  repo_git checkout main >/dev/null
}

set_next_ticket_id() {
  printf '%s\n' "$1" > "${REPO_DIR}/.governator/next_ticket_id"
  commit_paths "Set ticket id" ".governator/next_ticket_id"
}

setup() {
  REPO_DIR="$(mktemp -d "${BATS_TMPDIR}/repo.XXXXXX")"
  ORIGIN_DIR="$(mktemp -d "${BATS_TMPDIR}/origin.XXXXXX")"
  BIN_DIR="$(mktemp -d "${BATS_TMPDIR}/bin.XXXXXX")"

  cp -R "${BATS_TEST_DIRNAME}/../_governator" "${REPO_DIR}/_governator"
  cp -R "${BATS_TEST_DIRNAME}/../.governator" "${REPO_DIR}/.governator"
  cp "${BATS_TEST_DIRNAME}/../README.md" "${REPO_DIR}/README.md"

  repo_git init -b main >/dev/null
  repo_git config user.email "test@example.com"
  repo_git config user.name "Test User"
  repo_git add README.md _governator .governator
  repo_git commit -m "Init" >/dev/null

  git init --bare "${ORIGIN_DIR}" >/dev/null
  repo_git remote add origin "${ORIGIN_DIR}"
  repo_git config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"
  repo_git push -u origin main >/dev/null

  cat > "${BIN_DIR}/sgpt" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod +x "${BIN_DIR}/sgpt"

  export PATH="${BIN_DIR}:${PATH}"
  export CODEX_BIN="true"
}

@test "assign-backlog assigns task and logs in-flight" {
  write_task "task-backlog" "001-sample-ruby"
  commit_all "Add backlog task"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/001-sample-ruby.md" ]
  run grep -F "001-sample-ruby -> ruby" "${REPO_DIR}/_governator/in-flight.log"
  [ "$status" -eq 0 ]
}

@test "assign-backlog blocks tasks missing a role suffix" {
  write_task "task-backlog" "002norole"
  commit_all "Add missing role task"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/002norole.md" ]
  run grep -F "Missing required role" "${REPO_DIR}/_governator/task-blocked/002norole.md"
  [ "$status" -eq 0 ]
}

@test "assign-backlog blocks tasks with unknown roles" {
  write_task "task-backlog" "003-unknown-ghost"
  commit_all "Add unknown role task"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/003-unknown-ghost.md" ]
  run grep -F "Unknown role ghost" "${REPO_DIR}/_governator/task-blocked/003-unknown-ghost.md"
  [ "$status" -eq 0 ]
}

@test "assign-backlog respects global cap" {
  write_task "task-backlog" "004-cap-ruby"
  echo "004-busy-ruby -> ruby" >> "${REPO_DIR}/_governator/in-flight.log"
  commit_all "Prepare global cap"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-backlog/004-cap-ruby.md" ]
  [ ! -f "${REPO_DIR}/_governator/task-assigned/004-cap-ruby.md" ]
}

@test "assign-backlog respects per-worker cap" {
  write_task "task-backlog" "005-cap-ruby"
  echo "006-busy-ruby -> ruby" >> "${REPO_DIR}/_governator/in-flight.log"
  printf '%s\n' "2" > "${REPO_DIR}/.governator/global_worker_cap"
  printf '%s\n' "ruby: 1" > "${REPO_DIR}/.governator/worker_caps"
  commit_all "Prepare worker cap"

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-backlog/005-cap-ruby.md" ]
  [ ! -f "${REPO_DIR}/_governator/task-assigned/005-cap-ruby.md" ]
}

@test "check-zombies retries when branch missing and worker dead" {
  write_task "task-assigned" "007-zombie-ruby"
  echo "007-zombie-ruby -> ruby" >> "${REPO_DIR}/_governator/in-flight.log"

  tmp_dir="$(mktemp -d "${BATS_TMPDIR}/worker-tmp.XXXXXX")"
  echo "007-zombie-ruby | ruby | 999999 | ${tmp_dir} | worker/ruby/007-zombie-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare zombie task"

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  run grep -F "007-zombie-ruby | 1" "${REPO_DIR}/.governator/retry-counts.log"
  [ "$status" -eq 0 ]
}

@test "check-zombies blocks after second failure" {
  write_task "task-assigned" "008-stuck-ruby"
  echo "008-stuck-ruby -> ruby" >> "${REPO_DIR}/_governator/in-flight.log"
  echo "008-stuck-ruby | 1" >> "${REPO_DIR}/.governator/retry-counts.log"

  tmp_dir="$(mktemp -d "${BATS_TMPDIR}/worker-tmp.XXXXXX")"
  echo "008-stuck-ruby | ruby | 999999 | ${tmp_dir} | worker/ruby/008-stuck-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare stuck task"

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/008-stuck-ruby.md" ]
  run grep -F "008-stuck-ruby -> ruby" "${REPO_DIR}/_governator/in-flight.log"
  [ "$status" -ne 0 ]
  run grep -F "008-stuck-ruby |" "${REPO_DIR}/.governator/retry-counts.log"
  [ "$status" -ne 0 ]
}

@test "cleanup-tmp removes stale directories but keeps active ones" {
  project_name="$(basename "${REPO_DIR}")"
  active_dir="/tmp/governator-${project_name}-active-123"
  stale_dir="/tmp/governator-${project_name}-stale-123"
  mkdir -p "${active_dir}" "${stale_dir}"
  touch -t 202001010000 "${stale_dir}"

  printf '%s\n' "1" > "${REPO_DIR}/.governator/worker_timeout_seconds"
  echo "009-cleanup-ruby | ruby | 1234 | ${active_dir} | worker/ruby/009-cleanup-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare cleanup dirs"

  run bash "${REPO_DIR}/_governator/governator.sh" cleanup-tmp
  [ "$status" -eq 0 ]

  [ -d "${active_dir}" ]
  [ ! -d "${stale_dir}" ]
}

@test "process-branches approves worked task and moves to done" {
  write_task "task-worked" "010-review-ruby"
  echo "010-review-ruby -> ruby" >> "${REPO_DIR}/_governator/in-flight.log"
  echo "010-review-ruby | ruby | 999999 | /tmp/governator-test | worker/ruby/010-review-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare worked task"

  create_worker_branch "010-review-ruby" "ruby"
  export CODEX_REVIEW_CMD='cat > review.json <<'"'"'EOF'"'"'
{"result":"approve","comments":["looks good"]}
EOF'

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-done/010-review-ruby.md" ]
  run grep -F "Decision: approve" "${REPO_DIR}/_governator/task-done/010-review-ruby.md"
  [ "$status" -eq 0 ]
  run grep -F "010-review-ruby -> ruby" "${REPO_DIR}/_governator/in-flight.log"
  [ "$status" -ne 0 ]
}

@test "process-branches moves feedback task back to assigned" {
  write_task "task-feedback" "011-feedback-ruby"
  echo "011-feedback-ruby -> ruby" >> "${REPO_DIR}/_governator/in-flight.log"
  commit_all "Prepare feedback task"

  create_worker_branch "011-feedback-ruby" "ruby"

  run bash "${REPO_DIR}/_governator/governator.sh" process-branches
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/011-feedback-ruby.md" ]
  run grep -F "## Feedback" "${REPO_DIR}/_governator/task-assigned/011-feedback-ruby.md"
  [ "$status" -eq 0 ]
}

@test "parse-review blocks on missing file" {
  run bash "${REPO_DIR}/_governator/governator.sh" parse-review "${REPO_DIR}/missing.json"
  [ "$status" -eq 0 ]
  [ "${lines[0]}" = "block" ]
  run grep -F "Review file missing" <<< "${lines[1]}"
  [ "$status" -eq 0 ]
}

@test "parse-review blocks on invalid json" {
  printf '%s\n' '{' > "${REPO_DIR}/bad.json"
  commit_paths "Add bad json" "bad.json"

  run bash "${REPO_DIR}/_governator/governator.sh" parse-review "${REPO_DIR}/bad.json"
  [ "$status" -eq 0 ]
  local parsed_output="${output}"
  run grep -F "block" <<< "${parsed_output}"
  [ "$status" -eq 0 ]
  run grep -F "Failed to parse review.json" <<< "${parsed_output}"
  if [ "$status" -ne 0 ]; then
    run grep -F "Python3 unavailable" <<< "${parsed_output}"
    [ "$status" -eq 0 ]
  fi
}

@test "parse-review prints result and comments" {
  cat > "${REPO_DIR}/review.json" <<'EOF'
{"result":"approve","comments":["a","b"]}
EOF
  commit_paths "Add review json" "review.json"

  run bash "${REPO_DIR}/_governator/governator.sh" parse-review "${REPO_DIR}/review.json"
  [ "$status" -eq 0 ]
  [ "${lines[0]}" = "approve" ]
  [ "${lines[1]}" = "a" ]
  [ "${lines[2]}" = "b" ]
}

@test "parse-review coerces non-list comments" {
  cat > "${REPO_DIR}/review.json" <<'EOF'
{"result":"reject","comments":"needs work"}
EOF
  commit_paths "Add review json string" "review.json"

  run bash "${REPO_DIR}/_governator/governator.sh" parse-review "${REPO_DIR}/review.json"
  [ "$status" -eq 0 ]
  [ "${lines[0]}" = "reject" ]
  [ "${lines[1]}" = "needs work" ]
}

@test "list-workers excludes reviewer and includes known roles" {
  run bash "${REPO_DIR}/_governator/governator.sh" list-workers
  local workers_output="${output}"
  run grep -F "reviewer" <<< "${workers_output}"
  [ "$status" -ne 0 ]
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

@test "read-caps prints global and role defaults" {
  run bash "${REPO_DIR}/_governator/governator.sh" read-caps
  local caps_output="${output}"
  run grep -F "global 1" <<< "${caps_output}"
  [ "$status" -eq 0 ]
  run grep -F "ruby 1" <<< "${caps_output}"
  [ "$status" -eq 0 ]
}

@test "read-caps returns configured role cap" {
  printf '%s\n' "ruby: 4" > "${REPO_DIR}/.governator/worker_caps"
  commit_all "Set worker caps"

  run bash "${REPO_DIR}/_governator/governator.sh" read-caps ruby
  [ "$status" -eq 0 ]
  [ "${output}" = "4" ]
}

@test "count-in-flight totals and per-role counts" {
  printf '%s\n' "014-one-ruby -> ruby" >> "${REPO_DIR}/_governator/in-flight.log"
  printf '%s\n' "015-one-sre -> sre" >> "${REPO_DIR}/_governator/in-flight.log"
  commit_all "Add in-flight"

  run bash "${REPO_DIR}/_governator/governator.sh" count-in-flight
  [ "$status" -eq 0 ]
  [ "${output}" = "2" ]

  run bash "${REPO_DIR}/_governator/governator.sh" count-in-flight ruby
  [ "$status" -eq 0 ]
  [ "${output}" = "1" ]
}

@test "format-ticket-id pads to three digits" {
  run bash "${REPO_DIR}/_governator/governator.sh" format-ticket-id 1
  [ "$status" -eq 0 ]
  [ "${output}" = "001" ]
  run bash "${REPO_DIR}/_governator/governator.sh" format-ticket-id 12
  [ "$status" -eq 0 ]
  [ "${output}" = "012" ]
  run bash "${REPO_DIR}/_governator/governator.sh" format-ticket-id 123
  [ "$status" -eq 0 ]
  [ "${output}" = "123" ]
}

@test "allocate-ticket-id increments counter" {
  set_next_ticket_id "7"

  run bash "${REPO_DIR}/_governator/governator.sh" allocate-ticket-id
  [ "$status" -eq 0 ]
  [ "${output}" = "7" ]
  commit_paths "Bump ticket id" ".governator/next_ticket_id"
  run bash "${REPO_DIR}/_governator/governator.sh" allocate-ticket-id
  [ "$status" -eq 0 ]
  [ "${output}" = "8" ]
}

@test "normalize-tmp-path rewrites /tmp when available" {
  run bash "${REPO_DIR}/_governator/governator.sh" normalize-tmp-path "/tmp/sample"
  [ "$status" -eq 0 ]
  run grep -F "/tmp/sample" <<< "${output}"
  [ "$status" -eq 0 ]
}

@test "audit-log appends entries" {
  run bash "${REPO_DIR}/_governator/governator.sh" audit-log "016-audit" "did something"
  [ "$status" -eq 0 ]
  run grep -F "016-audit -> did something" "${REPO_DIR}/.governator/audit.log"
  [ "$status" -eq 0 ]
}

@test "cleanup-tmp dry-run lists stale dirs only" {
  project_name="$(basename "${REPO_DIR}")"
  active_dir="/tmp/governator-${project_name}-active-456"
  stale_dir="/tmp/governator-${project_name}-stale-456"
  mkdir -p "${active_dir}" "${stale_dir}"
  touch -t 202001010000 "${stale_dir}"

  printf '%s\n' "1" > "${REPO_DIR}/.governator/worker_timeout_seconds"
  echo "017-cleanup-ruby | ruby | 1234 | ${active_dir} | worker/ruby/017-cleanup-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare cleanup dry-run"

  run bash "${REPO_DIR}/_governator/governator.sh" cleanup-tmp --dry-run
  [ "$status" -eq 0 ]
  run grep -F "${stale_dir}" <<< "${output}"
  [ "$status" -eq 0 ]
  run grep -F "${active_dir}" <<< "${output}"
  [ "$status" -ne 0 ]
}

@test "lock command writes a lock file and reports active snapshot" {
  run bash "${REPO_DIR}/_governator/governator.sh" lock
  [ "$status" -eq 0 ]
  [ -f "${REPO_DIR}/.governator/governator.locked" ]
  run grep -F "Active work snapshot" <<< "${output}"
  [ "$status" -eq 0 ]
}

@test "status command notes the locked state" {
  run bash "${REPO_DIR}/_governator/governator.sh" lock
  [ "$status" -eq 0 ]
  run bash "${REPO_DIR}/_governator/governator.sh" status
  [ "$status" -eq 0 ]
  run grep -F "LOCKED" <<< "${output}"
  [ "$status" -eq 0 ]
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

@test "abort terminates worker, removes tmp dir, and blocks task" {
  write_task "task-assigned" "019-abort-ruby"
  printf '%s\n' "019-abort-ruby -> ruby" >> "${REPO_DIR}/_governator/in-flight.log"
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

  run grep -F "019-abort-ruby -> ruby" "${REPO_DIR}/_governator/in-flight.log"
  [ "$status" -ne 0 ]

  [ ! -f "${ORIGIN_DIR}/refs/heads/worker/ruby/019-abort-ruby" ]
}
