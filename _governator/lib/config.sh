# shellcheck shell=bash

# Read a numeric value from a file or return a default.
read_numeric_file() {
  local file="$1"
  local fallback="$2"
  if [[ ! -f "${file}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi

  local value
  value="$(tr -d '[:space:]' < "${file}")"
  if [[ -z "${value}" || ! "${value}" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# Read a single-line config value with fallback (whitespace trimmed).
read_config_value() {
  local file="$1"
  local fallback="$2"
  if [[ ! -f "${file}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi
  local value
  value="$(tr -d '[:space:]' < "${file}")"
  if [[ -z "${value}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi
  printf '%s\n' "${value}"
}

read_project_mode() {
  if [[ ! -f "${PROJECT_MODE_FILE}" ]]; then
    return 1
  fi
  local value
  value="$(tr -d '[:space:]' < "${PROJECT_MODE_FILE}")"
  if [[ "${value}" != "new" && "${value}" != "existing" ]]; then
    return 1
  fi
  printf '%s\n' "${value}"
}

require_project_mode() {
  if ! read_project_mode > /dev/null 2>&1; then
    log_error "Governator has not been initialized yet. Please run \`governator.sh init\` to configure your project."
    return 1
  fi
  return 0
}

read_remote_name() {
  read_config_value "${REMOTE_NAME_FILE}" "${DEFAULT_REMOTE_NAME}"
}

read_default_branch() {
  read_config_value "${DEFAULT_BRANCH_FILE}" "${DEFAULT_BRANCH_NAME}"
}

# Read the global concurrency cap (defaults to 1).
read_global_cap() {
  read_numeric_file "${GLOBAL_CAP_FILE}" "${DEFAULT_GLOBAL_CAP}"
}

# Read the worker timeout in seconds (defaults to 900).
read_worker_timeout_seconds() {
  read_numeric_file "${WORKER_TIMEOUT_FILE}" "${DEFAULT_WORKER_TIMEOUT_SECONDS}"
}

# Read the done-check cooldown in seconds (defaults to 3600).
read_done_check_cooldown_seconds() {
  read_numeric_file "${DONE_CHECK_COOLDOWN_FILE}" "3600"
}

read_done_check_last_run() {
  read_numeric_file "${DONE_CHECK_LAST_RUN_FILE}" "0"
}

write_done_check_last_run() {
  local timestamp="$1"
  printf '%s\n' "${timestamp}" > "${DONE_CHECK_LAST_RUN_FILE}"
}

read_project_done_sha() {
  if [[ ! -f "${PROJECT_DONE_FILE}" ]]; then
    printf '%s\n' ""
    return 0
  fi
  trim_whitespace "$(cat "${PROJECT_DONE_FILE}")"
}

write_project_done_sha() {
  local sha="$1"
  printf '%s\n' "${sha}" > "${PROJECT_DONE_FILE}"
}

governator_doc_sha() {
  git -C "${ROOT_DIR}" hash-object "${ROOT_DIR}/GOVERNATOR.md" 2> /dev/null || true
}

# Read the reasoning effort for a role (defaults to "medium").
read_reasoning_effort() {
  local role="$1"
  local fallback="medium"
  if [[ ! -f "${REASONING_EFFORT_FILE}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi

  local value
  value="$(
    awk -v role="${role}" -v fallback="${fallback}" '
      BEGIN { default=fallback; found=0 }
      $0 ~ /^[[:space:]]*#/ { next }
      $0 ~ /^[[:space:]]*$/ { next }
      $0 ~ /^[[:space:]]*[^:]+[[:space:]]*:[[:space:]]*[^[:space:]]+[[:space:]]*$/ {
        split($0, parts, ":")
        key = parts[1]
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", key)
        val = parts[2]
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", val)
        if (key == "default") {
          default = val
          next
        }
        if (key == role) {
          found = 1
          print val
        }
      }
      END {
        if (found == 0) {
          print default
        }
      }
    ' "${REASONING_EFFORT_FILE}" || true
  )"

  if [[ -z "${value}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# Read per-worker cap from worker_caps (defaults to 1).
read_worker_cap() {
  local role="$1"
  if [[ ! -f "${WORKER_CAPS_FILE}" ]]; then
    printf '%s\n' "${DEFAULT_WORKER_CAP}"
    return 0
  fi

  local cap
  cap="$(
    awk -v role="${role}" '
      $0 ~ /^[[:space:]]*#/ { next }
      $0 ~ /^[[:space:]]*$/ { next }
      $0 ~ /^[[:space:]]*[^:]+[[:space:]]*:[[:space:]]*[0-9]+[[:space:]]*$/ {
        split($0, parts, ":")
        key = parts[1]
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", key)
        if (key == role) {
          val = parts[2]
          gsub(/^[[:space:]]+|[[:space:]]+$/, "", val)
          print val
          exit 0
        }
      }
      END { exit 1 }
    ' "${WORKER_CAPS_FILE}" || true
  )"

  if [[ -z "${cap}" ]]; then
    printf '%s\n' "${DEFAULT_WORKER_CAP}"
    return 0
  fi
  printf '%s\n' "${cap}"
}

# Ensure the simple DB directory exists.
ensure_db_dir() {
  if [[ ! -d "${DB_DIR}" ]]; then
    mkdir -p "${DB_DIR}"
  fi
  mkdir -p "${DB_DIR}/logs"
  touch "${AUDIT_LOG}"
  touch "${WORKER_PROCESSES_LOG}" "${RETRY_COUNTS_LOG}"
  if [[ ! -f "${WORKER_TIMEOUT_FILE}" ]]; then
    printf '%s\n' "${DEFAULT_WORKER_TIMEOUT_SECONDS}" > "${WORKER_TIMEOUT_FILE}"
  fi
  if [[ ! -f "${REASONING_EFFORT_FILE}" ]]; then
    {
      printf '%s\n' "# Role-based reasoning effort for Codex workers."
      printf '%s\n' "# Allowed values: low | medium | high."
      printf '%s\n' "default: medium"
    } > "${REASONING_EFFORT_FILE}"
  fi
  if [[ ! -f "${WORKER_CAPS_FILE}" ]]; then
    printf '%s\n' "# Per-role worker caps, formatted as role: number" > "${WORKER_CAPS_FILE}"
  fi
}

# Ensure state logs exist so reads do not fail.
touch_logs() {
  touch "${FAILED_MERGES_LOG}" "${IN_FLIGHT_LOG}"
}
