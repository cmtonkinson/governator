#!/usr/bin/env bats

load ./helpers.bash

@test "update refreshes code and writes audit entry" {
  local tag="v1.0.0"
  upstream_root="$(create_upstream_dir "${tag}")"
  printf '%s\n' "# upstream update" >> "${upstream_root}/governator-1.0.0/_governator/governator.sh"
  tar_path="${BATS_TMPDIR}/upstream-code.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}" "${tag}"
  stub_curl_for_release "${tar_path}" "${tag}"

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
  local tag="v1.0.0"
  upstream_root="$(create_upstream_dir "${tag}")"
  migration_path="${upstream_root}/governator-1.0.0/_governator/migrations/202501010000__sample.sh"
  cat > "${migration_path}" <<'EOF_MIGRATE'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "migrated" >> "migration-output.txt"
EOF_MIGRATE
  chmod +x "${migration_path}"
  tar_path="${BATS_TMPDIR}/upstream-migrations.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}" "${tag}"
  stub_curl_for_release "${tar_path}" "${tag}"

  run bash "${REPO_DIR}/_governator/governator.sh" update --force-remote
  [ "$status" -eq 0 ]
  run grep -F "migrated" "${REPO_DIR}/migration-output.txt"
  [ "$status" -eq 0 ]
  run jq -e --arg id "202501010000__sample.sh" '.applied[]? | select(.id == $id)' \
    "${REPO_DIR}/.governator/migrations.json"
  [ "$status" -eq 0 ]
}

@test "update keeps local prompt with --keep-local" {
  local tag="v1.0.0"
  upstream_root="$(create_upstream_dir "${tag}")"
  tar_path="${BATS_TMPDIR}/upstream-baseline.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}" "${tag}"
  stub_curl_for_release "${tar_path}" "${tag}"
  run bash "${REPO_DIR}/_governator/governator.sh" update --force-remote
  local update_output="${output}"
  [ "$status" -eq 0 ]

  local_template="${REPO_DIR}/_governator/templates/task.md"
  original_template="$(cat "${local_template}")"
  printf '%s\n' "local change" >> "${local_template}"

  local tag2="v1.0.1"
  upstream_root="$(create_upstream_dir "${tag2}")"
  printf '%s\n' "${original_template}" > "${upstream_root}/governator-1.0.1/_governator/templates/task.md"
  printf '%s\n' "upstream change" >> "${upstream_root}/governator-1.0.1/_governator/templates/task.md"
  tar_path="${BATS_TMPDIR}/upstream-template.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}" "${tag2}"
  stub_curl_for_release "${tar_path}" "${tag2}"

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
  local tag="v1.0.0"
  upstream_root="$(create_upstream_dir "${tag}")"
  tar_path="${BATS_TMPDIR}/upstream-baseline2.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}" "${tag}"
  stub_curl_for_release "${tar_path}" "${tag}"
  run bash "${REPO_DIR}/_governator/governator.sh" update --force-remote
  local update_output="${output}"
  [ "$status" -eq 0 ]

  local_template="${REPO_DIR}/_governator/templates/task.md"
  original_template="$(cat "${local_template}")"
  printf '%s\n' "local change" >> "${local_template}"

  local tag2="v1.0.1"
  upstream_root="$(create_upstream_dir "${tag2}")"
  printf '%s\n' "${original_template}" > "${upstream_root}/governator-1.0.1/_governator/templates/task.md"
  printf '%s\n' "upstream change" >> "${upstream_root}/governator-1.0.1/_governator/templates/task.md"
  tar_path="${BATS_TMPDIR}/upstream-template2.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}" "${tag2}"
  stub_curl_for_release "${tar_path}" "${tag2}"

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

@test "update fails on checksum mismatch" {
  local tag="v1.0.0"
  upstream_root="$(create_upstream_dir "${tag}")"
  tar_path="${BATS_TMPDIR}/upstream-bad-checksum.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}" "${tag}"
  # Provide a bad checksum to trigger verification failure
  stub_curl_for_release "${tar_path}" "${tag}" "bad_checksum_value"

  run bash "${REPO_DIR}/_governator/governator.sh" update --force-remote
  [ "$status" -ne 0 ]
  run grep -F "Checksum verification failed" <<< "${output}"
  [ "$status" -eq 0 ]
}

@test "update respects --version flag" {
  local tag="v2.0.0"
  upstream_root="$(create_upstream_dir "${tag}")"
  printf '%s\n' "# version 2.0.0 update" >> "${upstream_root}/governator-2.0.0/_governator/governator.sh"
  tar_path="${BATS_TMPDIR}/upstream-pinned.tar.gz"
  build_upstream_tarball "${upstream_root}" "${tar_path}" "${tag}"
  stub_curl_for_release "${tar_path}" "${tag}"

  run bash "${REPO_DIR}/_governator/governator.sh" update --force-remote --version "${tag}"
  local update_output="${output}"
  [ "$status" -eq 0 ]
  run grep -F "Pinned version: ${tag}" <<< "${update_output}"
  [ "$status" -eq 0 ]
  run grep -F "# version 2.0.0 update" "${REPO_DIR}/_governator/governator.sh"
  [ "$status" -eq 0 ]
}
