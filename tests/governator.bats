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

write_task() {
  local dir="$1"
  local name="$2"
  cat > "${REPO_DIR}/_governator/${dir}/${name}.md" <<'EOF'
# Task
EOF
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
