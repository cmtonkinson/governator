#!/usr/bin/env bats

setup() {
  REPO_DIR="${BATS_TMPDIR}/repo"
  ORIGIN_DIR="${BATS_TMPDIR}/origin.git"
  BIN_DIR="${BATS_TMPDIR}/bin"
  mkdir -p "${REPO_DIR}" "${BIN_DIR}"

  cp -R "${BATS_TEST_DIRNAME}/../_governator" "${REPO_DIR}/_governator"
  cp -R "${BATS_TEST_DIRNAME}/../.governator" "${REPO_DIR}/.governator"
  cp "${BATS_TEST_DIRNAME}/../README.md" "${REPO_DIR}/README.md"

  git -C "${REPO_DIR}" init -b main >/dev/null
  git -C "${REPO_DIR}" config user.email "test@example.com"
  git -C "${REPO_DIR}" config user.name "Test User"
  git -C "${REPO_DIR}" add README.md _governator .governator
  git -C "${REPO_DIR}" commit -m "Init" >/dev/null

  git init --bare "${ORIGIN_DIR}" >/dev/null
  git -C "${REPO_DIR}" remote add origin "${ORIGIN_DIR}"
  git -C "${REPO_DIR}" push -u origin main >/dev/null

  cat > "${BIN_DIR}/sgpt" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod +x "${BIN_DIR}/sgpt"

  export PATH="${BIN_DIR}:${PATH}"
  export CODEX_BIN="true"
}

@test "assign-backlog assigns task and logs in-flight" {
  cat > "${REPO_DIR}/_governator/task-backlog/001-sample-ruby.md" <<'EOF'
# Sample task
EOF
  git -C "${REPO_DIR}" add _governator/task-backlog/001-sample-ruby.md
  git -C "${REPO_DIR}" commit -m "Add backlog task" >/dev/null
  git -C "${REPO_DIR}" push origin main >/dev/null

  run bash "${REPO_DIR}/_governator/governator.sh" assign-backlog
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-assigned/001-sample-ruby.md" ]
  run grep -F "001-sample-ruby -> ruby" "${REPO_DIR}/_governator/in-flight.log"
  [ "$status" -eq 0 ]
}

@test "check-zombies retries when branch missing and worker dead" {
  cat > "${REPO_DIR}/_governator/task-assigned/002-zombie-ruby.md" <<'EOF'
# Zombie task
EOF

  echo "002-zombie-ruby -> ruby" >> "${REPO_DIR}/_governator/in-flight.log"

  tmp_dir="${BATS_TMPDIR}/worker-tmp"
  mkdir -p "${tmp_dir}"
  echo "002-zombie-ruby | ruby | 999999 | ${tmp_dir} | worker/ruby/002-zombie-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"
  git -C "${REPO_DIR}" add _governator/task-assigned/002-zombie-ruby.md _governator/in-flight.log .governator/worker-processes.log
  git -C "${REPO_DIR}" commit -m "Prepare zombie task" >/dev/null
  git -C "${REPO_DIR}" push origin main >/dev/null

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  run grep -F "002-zombie-ruby | 1" "${REPO_DIR}/.governator/retry-counts.log"
  [ "$status" -eq 0 ]
}
