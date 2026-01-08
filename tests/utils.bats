#!/usr/bin/env bats

load ./helpers.bash

@test "sha256_file falls back when shasum fails" {
  test_file="${REPO_DIR}/hash-input.txt"
  printf '%s\n' "hash me" > "${test_file}"

  cat > "${BIN_DIR}/shasum" <<'EOF_SHASUM'
#!/usr/bin/env bash
exit 1
EOF_SHASUM
  chmod +x "${BIN_DIR}/shasum"

  cat > "${BIN_DIR}/sha256sum" <<'EOF_SHA256SUM'
#!/usr/bin/env bash
printf '%s  %s\n' "deadbeef" "$1"
EOF_SHA256SUM
  chmod +x "${BIN_DIR}/sha256sum"

  run bash -c "
    set -euo pipefail
    PATH=\"${BIN_DIR}:\$PATH\"
    source \"${REPO_DIR}/_governator/lib/utils.sh\"
    sha256_file \"${test_file}\"
  "
  [ "$status" -eq 0 ]
  [ "${output}" = "deadbeef" ]
}

@test "normalize-tmp-path rewrites /tmp when available" {
  run bash "${REPO_DIR}/_governator/governator.sh" normalize-tmp-path "/tmp/sample"
  [ "$status" -eq 0 ]
  run grep -F "/tmp/sample" <<< "${output}"
  [ "$status" -eq 0 ]
}
