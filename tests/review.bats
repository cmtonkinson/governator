#!/usr/bin/env bats

load ./helpers.bash

@test "apply_review_decision removes review.json on reject" {
  write_task "task-worked" "020-review-ruby"
  commit_all "Prepare worked task"

  cat > "${REPO_DIR}/review.json" <<'EOF_REVIEW'
{"result":"reject","comments":["needs changes"]}
EOF_REVIEW
  commit_paths "Add review json" "review.json"

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/_governator/_local_state\"
    AUDIT_LOG=\"${REPO_DIR}/_governator/_local_state/audit.log\"
    CONFIG_FILE=\"${REPO_DIR}/_governator/_durable_state/config.json\"
    DEFAULT_REMOTE_NAME=\"origin\"
    DEFAULT_BRANCH_NAME=\"main\"
    COMPLETION_CHECK_REVIEW_TASK=\"000-completion-check-reviewer\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    source \"${REPO_DIR}/_governator/lib/utils.sh\"
    source \"${REPO_DIR}/_governator/lib/logging.sh\"
    source \"${REPO_DIR}/_governator/lib/config.sh\"
    source \"${REPO_DIR}/_governator/lib/git.sh\"
    source \"${REPO_DIR}/_governator/lib/tasks.sh\"
    source \"${REPO_DIR}/_governator/lib/review.sh\"
    apply_review_decision \"020-review-ruby\" \"reviewer\" \"reject\" \"reject reason\" \"needs changes\"
  "
  [ "$status" -eq 0 ]

  [ ! -f "${REPO_DIR}/review.json" ]
  [ -f "${REPO_DIR}/_governator/task-assigned/020-review-ruby.md" ]
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
  cat > "${REPO_DIR}/review.json" <<'EOF_REVIEW'
{"result":"approve","comments":["a","b"]}
EOF_REVIEW
  commit_paths "Add review json" "review.json"

  run bash "${REPO_DIR}/_governator/governator.sh" parse-review "${REPO_DIR}/review.json"
  [ "$status" -eq 0 ]
  [ "${lines[0]}" = "approve" ]
  [ "${lines[1]}" = "a" ]
  [ "${lines[2]}" = "b" ]
}

@test "parse-review normalizes approval, rejection, and block variants" {
  cat > "${REPO_DIR}/review.json" <<'EOF_REVIEW'
{"result":"Accepted","comments":[]}
EOF_REVIEW
  commit_paths "Add approval variant" "review.json"

  run bash "${REPO_DIR}/_governator/governator.sh" parse-review "${REPO_DIR}/review.json"
  [ "$status" -eq 0 ]
  [ "${lines[0]}" = "approve" ]

  cat > "${REPO_DIR}/review.json" <<'EOF_REVIEW'
{"result":"Denied","comments":[]}
EOF_REVIEW
  commit_paths "Add rejection variant" "review.json"

  run bash "${REPO_DIR}/_governator/governator.sh" parse-review "${REPO_DIR}/review.json"
  [ "$status" -eq 0 ]
  [ "${lines[0]}" = "reject" ]

  cat > "${REPO_DIR}/review.json" <<'EOF_REVIEW'
{"result":"Blocked","comments":[]}
EOF_REVIEW
  commit_paths "Add block variant" "review.json"

  run bash "${REPO_DIR}/_governator/governator.sh" parse-review "${REPO_DIR}/review.json"
  [ "$status" -eq 0 ]
  [ "${lines[0]}" = "block" ]
}

@test "parse-review coerces non-list comments" {
  cat > "${REPO_DIR}/review.json" <<'EOF_REVIEW'
{"result":"reject","comments":"needs work"}
EOF_REVIEW
  commit_paths "Add review json string" "review.json"

  run bash "${REPO_DIR}/_governator/governator.sh" parse-review "${REPO_DIR}/review.json"
  [ "$status" -eq 0 ]
  [ "${lines[0]}" = "reject" ]
  [ "${lines[1]}" = "needs work" ]
}
