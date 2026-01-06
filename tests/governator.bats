#!/usr/bin/env bats

repo_git() {
  git -C "${REPO_DIR}" "$@"
}

commit_all() {
  local message="$1"
  repo_git add GOVERNATOR.md _governator .governator
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

complete_bootstrap() {
  mkdir -p "${REPO_DIR}/_governator/docs"
  printf '%s\n' "# ASR" > "${REPO_DIR}/_governator/docs/asr.md"
  printf '%s\n' "# arc42" > "${REPO_DIR}/_governator/docs/arc42.md"
  printf '%s\n' "# Personas" > "${REPO_DIR}/_governator/docs/personas.md"
  printf '%s\n' "# Wardley" > "${REPO_DIR}/_governator/docs/wardley.md"
  printf '%s\n' "# ADR-0001" > "${REPO_DIR}/_governator/docs/adr-0001.md"
  write_task "task-done" "000-architecture-bootstrap-architect"
  commit_paths "Complete bootstrap" \
    "_governator/docs/asr.md" \
    "_governator/docs/arc42.md" \
    "_governator/docs/personas.md" \
    "_governator/docs/wardley.md" \
    "_governator/docs/adr-0001.md" \
    "_governator/task-done/000-architecture-bootstrap-architect.md"
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

create_upstream_dir() {
  local upstream_root
  upstream_root="$(mktemp -d "${BATS_TMPDIR}/upstream.XXXXXX")"
  mkdir -p "${upstream_root}/governator-main"
  cp -R "${REPO_DIR}/_governator" "${upstream_root}/governator-main/_governator"
  printf '%s\n' "${upstream_root}"
}

build_upstream_tarball() {
  local upstream_root="$1"
  local tar_path="$2"
  tar -cz -C "${upstream_root}" -f "${tar_path}" governator-main/_governator
}

stub_curl_with_tarball() {
  local tar_path="$1"
  cat > "${BIN_DIR}/curl" <<EOF
#!/usr/bin/env bash
cat "${tar_path}"
EOF
  chmod +x "${BIN_DIR}/curl"
}

set_next_task_id() {
  printf '%s\n' "$1" > "${REPO_DIR}/.governator/next_task_id"
  commit_paths "Set task id" ".governator/next_task_id"
}

setup() {
  REPO_DIR="$(mktemp -d "${BATS_TMPDIR}/repo.XXXXXX")"
  ORIGIN_DIR="$(mktemp -d "${BATS_TMPDIR}/origin.XXXXXX")"
  BIN_DIR="$(mktemp -d "${BATS_TMPDIR}/bin.XXXXXX")"

  cp -R "${BATS_TEST_DIRNAME}/../_governator" "${REPO_DIR}/_governator"
  cp -R "${BATS_TEST_DIRNAME}/../.governator" "${REPO_DIR}/.governator"
  cp "${BATS_TEST_DIRNAME}/../GOVERNATOR.md" "${REPO_DIR}/GOVERNATOR.md"

  repo_git init -b main >/dev/null
  repo_git config user.email "test@example.com"
  repo_git config user.name "Test User"
  repo_git add GOVERNATOR.md _governator .governator
  repo_git commit -m "Init" >/dev/null

  git init --bare "${ORIGIN_DIR}" >/dev/null
  repo_git remote add origin "${ORIGIN_DIR}"
  repo_git config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"
  repo_git push -u origin main >/dev/null

  printf '%s\n' "new" > "${REPO_DIR}/.governator/project_mode"
  commit_paths "Set project mode" ".governator/project_mode"

  cat > "${BIN_DIR}/codex" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod +x "${BIN_DIR}/codex"

  export PATH="${BIN_DIR}:${PATH}"
}

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
  echo "007-zombie-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"

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
  echo "008-stuck-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  echo "008-stuck-ruby | 1" >> "${REPO_DIR}/.governator/retry-counts.log"

  tmp_dir="$(mktemp -d "${BATS_TMPDIR}/worker-tmp.XXXXXX")"
  echo "008-stuck-ruby | ruby | 999999 | ${tmp_dir} | worker/ruby/008-stuck-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare stuck task"

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/008-stuck-ruby.md" ]
  run grep -F "008-stuck-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
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
  echo "010-review-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  echo "010-review-ruby | ruby | 999999 | /tmp/governator-test | worker/ruby/010-review-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare worked task"

  create_worker_branch "010-review-ruby" "ruby"
  repo_git checkout -b "worker/reviewer/010-review-ruby" "origin/worker/ruby/010-review-ruby" >/dev/null
  cat > "${REPO_DIR}/review.json" <<'EOF'
{"result":"approve","comments":["looks good"]}
EOF
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
    REMOTE_NAME_FILE=\"\${DB_DIR}/remote_name\"
    DEFAULT_BRANCH_FILE=\"\${DB_DIR}/default_branch\"
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

@test "check-zombies blocks multiple tasks in one pass" {
  write_task "task-assigned" "012-zombie-a-ruby"
  write_task "task-assigned" "013-zombie-b-ruby"
  echo "012-zombie-a-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  echo "013-zombie-b-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  echo "012-zombie-a-ruby | 1" >> "${REPO_DIR}/.governator/retry-counts.log"
  echo "013-zombie-b-ruby | 1" >> "${REPO_DIR}/.governator/retry-counts.log"

  project_name="$(basename "${REPO_DIR}")"
  tmp_dir_a="$(mktemp -d "/tmp/governator-${project_name}-ruby-012-zombie-a-ruby-XXXXXX")"
  tmp_dir_b="$(mktemp -d "/tmp/governator-${project_name}-ruby-013-zombie-b-ruby-XXXXXX")"
  echo "012-zombie-a-ruby | ruby | 999999 | ${tmp_dir_a} | worker/ruby/012-zombie-a-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  echo "013-zombie-b-ruby | ruby | 999999 | ${tmp_dir_b} | worker/ruby/013-zombie-b-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare multiple zombie tasks"

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/012-zombie-a-ruby.md" ]
  [ -f "${REPO_DIR}/_governator/task-blocked/013-zombie-b-ruby.md" ]
  run grep -F "012-zombie-a-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
  run grep -F "013-zombie-b-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
}

@test "check-zombies recovers reviewer output by pushing review branch" {
  write_task "task-worked" "016-review-ruby"
  echo "016-review-ruby -> reviewer" >> "${REPO_DIR}/.governator/in-flight.log"

  project_name="$(basename "${REPO_DIR}")"
  tmp_dir="$(mktemp -d "/tmp/governator-${project_name}-reviewer-016-review-ruby-XXXXXX")"
  git clone "${ORIGIN_DIR}" "${tmp_dir}" >/dev/null
  git -C "${tmp_dir}" checkout -b "worker/reviewer/016-review-ruby" "origin/main" >/dev/null
  git -C "${tmp_dir}" config user.email "test@example.com"
  git -C "${tmp_dir}" config user.name "Test User"
  cat > "${tmp_dir}/review.json" <<'EOF'
{"result":"reject","comments":["needs work"]}
EOF

  echo "016-review-ruby | reviewer | 999999 | ${tmp_dir} | worker/reviewer/016-review-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  commit_all "Prepare reviewer recovery"

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  [ -f "${ORIGIN_DIR}/refs/heads/worker/reviewer/016-review-ruby" ]
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
  printf '%s\n' "014-one-ruby -> ruby" >> "${REPO_DIR}/.governator/in-flight.log"
  printf '%s\n' "015-one-sre -> sre" >> "${REPO_DIR}/.governator/in-flight.log"
  commit_all "Add in-flight"

  run bash "${REPO_DIR}/_governator/governator.sh" count-in-flight
  [ "$status" -eq 0 ]
  [ "${output}" = "2" ]

  run bash "${REPO_DIR}/_governator/governator.sh" count-in-flight ruby
  [ "$status" -eq 0 ]
  [ "${output}" = "1" ]
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
  commit_paths "Bump task id" ".governator/next_task_id"
  run bash "${REPO_DIR}/_governator/governator.sh" allocate-task-id
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

@test "status summarizes milestone and epic progress" {
  cat > "${REPO_DIR}/_governator/task-done/030-done-ruby.md" <<'EOF'
---
milestone: m1
epic: e1
---
# Task
EOF
  cat > "${REPO_DIR}/_governator/task-assigned/031-pending-ruby.md" <<'EOF'
---
milestone: m1
epic: e2
---
# Task
EOF
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

@test "init supports defaults and non-interactive options" {
  rm -f "${REPO_DIR}/.governator/project_mode" \
    "${REPO_DIR}/.governator/remote_name" \
    "${REPO_DIR}/.governator/default_branch"
  commit_paths "Clear init files" ".governator"

  run bash "${REPO_DIR}/_governator/governator.sh" init --defaults
  [ "$status" -eq 0 ]
  run cat "${REPO_DIR}/.governator/project_mode"
  [ "$status" -eq 0 ]
  [ "${output}" = "new" ]

  rm -f "${REPO_DIR}/.governator/project_mode" \
    "${REPO_DIR}/.governator/remote_name" \
    "${REPO_DIR}/.governator/default_branch"
  commit_paths "Clear init files again" ".governator"

  run bash "${REPO_DIR}/_governator/governator.sh" init --non-interactive --project-mode=existing --remote=upstream --branch=trunk
  [ "$status" -eq 0 ]
  run cat "${REPO_DIR}/.governator/project_mode"
  [ "$status" -eq 0 ]
  [ "${output}" = "existing" ]
  run cat "${REPO_DIR}/.governator/remote_name"
  [ "$status" -eq 0 ]
  [ "${output}" = "upstream" ]
  run cat "${REPO_DIR}/.governator/default_branch"
  [ "$status" -eq 0 ]
  [ "${output}" = "trunk" ]
}

@test "update refreshes code and writes audit entry" {
  upstream_root="$(create_upstream_dir)"
  printf '%s\n' "# upstream update" >> "${upstream_root}/governator-main/_governator/governator.sh"
  tar_path="${BATS_TMPDIR}/upstream-code.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}"
  stub_curl_with_tarball "${tar_path}"

  run bash "${REPO_DIR}/_governator/governator.sh" update --force-remote
  local update_output="${output}"
  printf 'status=%s\n' "$status"
  printf 'output:\n%s\n' "$update_output"
  [ "$status" -eq 0 ]
  run grep -F "Updated files:" <<< "${update_output}"
  [ "$status" -eq 0 ]
  run grep -F "updated _governator/governator.sh" <<< "${update_output}"
  [ "$status" -eq 0 ]

  run grep -F "# upstream update" "${REPO_DIR}/_governator/governator.sh"
  [ "$status" -eq 0 ]
  run grep -F "update applied: updated _governator/governator.sh" "${REPO_DIR}/.governator/audit.log"
  [ "$status" -eq 0 ]
}

@test "update runs migrations and records state" {
  upstream_root="$(create_upstream_dir)"
  migration_path="${upstream_root}/governator-main/_governator/migrations/202501010000__sample.sh"
  cat > "${migration_path}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "migrated" >> "migration-output.txt"
EOF
  chmod +x "${migration_path}"
  tar_path="${BATS_TMPDIR}/upstream-migrations.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}"
  stub_curl_with_tarball "${tar_path}"

  run bash "${REPO_DIR}/_governator/governator.sh" update --force-remote
  [ "$status" -eq 0 ]
  run grep -F "migrated" "${REPO_DIR}/migration-output.txt"
  [ "$status" -eq 0 ]
  run jq -e --arg id "202501010000__sample.sh" '.applied[]? | select(.id == $id)' \
    "${REPO_DIR}/.governator/migrations.json"
  [ "$status" -eq 0 ]
}

@test "update keeps local prompt with --keep-local" {
  upstream_root="$(create_upstream_dir)"
  tar_path="${BATS_TMPDIR}/upstream-baseline.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}"
  stub_curl_with_tarball "${tar_path}"
  run bash "${REPO_DIR}/_governator/governator.sh" update --force-remote
  local update_output="${output}"
  [ "$status" -eq 0 ]

  local_template="${REPO_DIR}/_governator/templates/task.md"
  original_template="$(cat "${local_template}")"
  printf '%s\n' "local change" >> "${local_template}"

  upstream_root="$(create_upstream_dir)"
  printf '%s\n' "${original_template}" > "${upstream_root}/governator-main/_governator/templates/task.md"
  printf '%s\n' "upstream change" >> "${upstream_root}/governator-main/_governator/templates/task.md"
  tar_path="${BATS_TMPDIR}/upstream-template.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}"
  stub_curl_with_tarball "${tar_path}"

  run bash "${REPO_DIR}/_governator/governator.sh" update --keep-local
  local keep_output="${output}"
  [ "$status" -eq 0 ]
  run grep -F "No updates applied." <<< "${keep_output}"
  [ "$status" -eq 0 ]
  run grep -F "local change" "${local_template}"
  [ "$status" -eq 0 ]
  run grep -F "upstream change" "${local_template}"
  [ "$status" -ne 0 ]
  run grep -F "update applied" "${REPO_DIR}/.governator/audit.log"
  [ "$status" -ne 0 ]
}

@test "update overwrites local prompt with --force-remote" {
  upstream_root="$(create_upstream_dir)"
  tar_path="${BATS_TMPDIR}/upstream-baseline2.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}"
  stub_curl_with_tarball "${tar_path}"
  run bash "${REPO_DIR}/_governator/governator.sh" update --force-remote
  local update_output="${output}"
  [ "$status" -eq 0 ]

  local_template="${REPO_DIR}/_governator/templates/task.md"
  original_template="$(cat "${local_template}")"
  printf '%s\n' "local change" >> "${local_template}"

  upstream_root="$(create_upstream_dir)"
  printf '%s\n' "${original_template}" > "${upstream_root}/governator-main/_governator/templates/task.md"
  printf '%s\n' "upstream change" >> "${upstream_root}/governator-main/_governator/templates/task.md"
  tar_path="${BATS_TMPDIR}/upstream-template2.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}"
  stub_curl_with_tarball "${tar_path}"

  run bash "${REPO_DIR}/_governator/governator.sh" update --force-remote
  local update_output="${output}"
  [ "$status" -eq 0 ]
  run grep -F "Updated files:" <<< "${update_output}"
  [ "$status" -eq 0 ]
  run grep -F "updated _governator/templates/task.md" <<< "${update_output}"
  [ "$status" -eq 0 ]
  run grep -F "upstream change" "${local_template}"
  [ "$status" -eq 0 ]
  run grep -F "local change" "${local_template}"
  [ "$status" -ne 0 ]
  run grep -F "update applied: updated _governator/templates/task.md" "${REPO_DIR}/.governator/audit.log"
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
