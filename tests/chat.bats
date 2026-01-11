#!/usr/bin/env bats

load ./helpers.bash

@test "run_chat_session fails when screen cannot start session" {
  cat > "${BIN_DIR}/screen" <<'EOF_SCREEN'
#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
if [[ "${cmd}" == "-dmS" ]]; then
  if [[ "${SCREEN_FAIL_DMS:-0}" -eq 1 ]]; then
    exit 1
  fi
  exit 0
fi
if [[ "${cmd}" == "-S" && "${3:-}" == "-X" && "${4:-}" == "stuff" ]]; then
  if [[ "${SCREEN_FAIL_STUFF:-0}" -eq 1 ]]; then
    exit 1
  fi
  exit 0
fi
if [[ "${cmd}" == "-r" ]]; then
  if [[ "${SCREEN_FAIL_R:-0}" -eq 1 ]]; then
    exit 1
  fi
  exit 0
fi
exit 0
EOF_SCREEN
  chmod +x "${BIN_DIR}/screen"

  run bash -c "
    set -euo pipefail
    PATH=\"${BIN_DIR}:\$PATH\"
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    CONFIG_FILE=\"${REPO_DIR}/.governator/config.json\"
    CHAT_DOCS_DIR=\"${REPO_DIR}/_governator/docs/chat\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    export SCREEN_FAIL_DMS=1
    source \"${REPO_DIR}/_governator/lib/utils.sh\"
    source \"${REPO_DIR}/_governator/lib/logging.sh\"
    source \"${REPO_DIR}/_governator/lib/config.sh\"
    source \"${REPO_DIR}/_governator/lib/chat.sh\"
    run_chat_session \"chat-session\" \"prompt\" \"${REPO_DIR}/_governator/docs/chat/transcript.log\"
  "
  [ "$status" -ne 0 ]
}

@test "run_chat_session retries prompt injection until screen accepts it" {
  cat > "${BIN_DIR}/screen" <<'EOF_SCREEN'
#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
if [[ "${cmd}" == "-dmS" ]]; then
  exit 0
fi
if [[ "${cmd}" == "-S" && "${3:-}" == "-X" && "${4:-}" == "stuff" ]]; then
  count_file="${SCREEN_STUFF_COUNT_FILE:-}"
  if [[ -n "${count_file}" ]]; then
    count=0
    if [[ -f "${count_file}" ]]; then
      count="$(cat "${count_file}")"
    fi
    count=$((count + 1))
    printf '%s' "${count}" > "${count_file}"
    if [[ -n "${SCREEN_STUFF_FAIL_UNTIL:-}" && "${count}" -le "${SCREEN_STUFF_FAIL_UNTIL}" ]]; then
      exit 1
    fi
  fi
  exit 0
fi
if [[ "${cmd}" == "-r" ]]; then
  exit 0
fi
exit 0
EOF_SCREEN
  chmod +x "${BIN_DIR}/screen"

  count_file="$(mktemp "${BATS_TMPDIR}/screen-count.XXXXXX")"

  run bash -c "
    set -euo pipefail
    PATH=\"${BIN_DIR}:\$PATH\"
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    CONFIG_FILE=\"${REPO_DIR}/.governator/config.json\"
    CHAT_DOCS_DIR=\"${REPO_DIR}/_governator/docs/chat\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    export SCREEN_STUFF_COUNT_FILE=\"${count_file}\"
    export SCREEN_STUFF_FAIL_UNTIL=2
    source \"${REPO_DIR}/_governator/lib/utils.sh\"
    source \"${REPO_DIR}/_governator/lib/logging.sh\"
    source \"${REPO_DIR}/_governator/lib/config.sh\"
    source \"${REPO_DIR}/_governator/lib/chat.sh\"
    run_chat_session \"chat-session\" \"prompt\" \"${REPO_DIR}/_governator/docs/chat/transcript.log\"
  "
  [ "$status" -eq 0 ]

  count="$(cat "${count_file}")"
  [ "${count}" -eq 3 ]
}

@test "run_chat_session fails when screen attach errors" {
  cat > "${BIN_DIR}/screen" <<'EOF_SCREEN'
#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
if [[ "${cmd}" == "-dmS" ]]; then
  exit 0
fi
if [[ "${cmd}" == "-S" && "${3:-}" == "-X" && "${4:-}" == "stuff" ]]; then
  exit 0
fi
if [[ "${cmd}" == "-r" ]]; then
  if [[ "${SCREEN_FAIL_R:-0}" -eq 1 ]]; then
    exit 1
  fi
  exit 0
fi
exit 0
EOF_SCREEN
  chmod +x "${BIN_DIR}/screen"

  run bash -c "
    set -euo pipefail
    PATH=\"${BIN_DIR}:\$PATH\"
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    CONFIG_FILE=\"${REPO_DIR}/.governator/config.json\"
    CHAT_DOCS_DIR=\"${REPO_DIR}/_governator/docs/chat\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    export SCREEN_FAIL_R=1
    source \"${REPO_DIR}/_governator/lib/utils.sh\"
    source \"${REPO_DIR}/_governator/lib/logging.sh\"
    source \"${REPO_DIR}/_governator/lib/config.sh\"
    source \"${REPO_DIR}/_governator/lib/chat.sh\"
    run_chat_session \"chat-session\" \"prompt\" \"${REPO_DIR}/_governator/docs/chat/transcript.log\"
  "
  [ "$status" -ne 0 ]
}
