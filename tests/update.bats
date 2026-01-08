#!/usr/bin/env bats

load ./helpers.bash

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
  cat > "${migration_path}" <<'EOF_MIGRATE'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "migrated" >> "migration-output.txt"
EOF_MIGRATE
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
