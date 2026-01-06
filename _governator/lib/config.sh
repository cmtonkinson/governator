# shellcheck shell=bash

# read_numeric_file
# Purpose: Read a numeric value from a file with fallback handling.
# Args:
#   $1: Path to file (string).
#   $2: Fallback value (string or integer).
# Output: Prints the numeric value or fallback to stdout.
# Returns: 0 always.
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

# read_config_value
# Purpose: Read a single-line config value with fallback and trimming.
# Args:
#   $1: Path to file (string).
#   $2: Fallback value (string).
# Output: Prints the trimmed value or fallback to stdout.
# Returns: 0 always.
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

# read_project_mode
# Purpose: Read and validate the project mode ("new" or "existing").
# Args: None.
# Output: Prints the project mode to stdout.
# Returns: 0 if valid mode exists; 1 otherwise.
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

# require_project_mode
# Purpose: Enforce that project mode is initialized before running commands.
# Args: None.
# Output: Logs an error when initialization is missing.
# Returns: 0 when initialized; 1 otherwise.
require_project_mode() {
  if ! read_project_mode > /dev/null 2>&1; then
    log_error "Governator has not been initialized yet. Please run \`governator.sh init\` to configure your project."
    return 1
  fi
  return 0
}

# ensure_gitignore_entries
# Purpose: Ensure .gitignore contains governator-specific entries.
# Args: None.
# Output: Writes to .gitignore when missing entries.
# Returns: 0 on completion.
ensure_gitignore_entries() {
  if [[ ! -f "${GITIGNORE_PATH}" ]]; then
    printf '# Governator\n' > "${GITIGNORE_PATH}"
  fi
  local entry
  for entry in "${GITIGNORE_ENTRIES[@]}"; do
    if ! grep -Fqx -- "${entry}" "${GITIGNORE_PATH}" 2> /dev/null; then
      printf '%s\n' "${entry}" >> "${GITIGNORE_PATH}"
    fi
  done
}

# init_governator
# Purpose: Initialize Governator config, defaults, and manifest.
# Args: None.
# Output: Prompts for project mode, remote, branch; logs initialization.
# Returns: 0 on success; exits 1 on invalid state.
init_governator() {
  ensure_db_dir
  ensure_gitignore_entries
  if read_project_mode > /dev/null 2>&1; then
    log_error "Governator is already initialized. Re-run init after clearing ${PROJECT_MODE_FILE}."
    exit 1
  fi

  local project_mode=""
  while true; do
    read -r -p "Is this a new or existing project? (new/existing): " project_mode
    project_mode="$(trim_whitespace "${project_mode}")"
    project_mode="$(printf '%s' "${project_mode}" | tr '[:upper:]' '[:lower:]')"
    if [[ "${project_mode}" == "new" || "${project_mode}" == "existing" ]]; then
      break
    fi
    printf 'Please enter "new" or "existing".\n'
  done

  local remote_name
  read -r -p "Default remote [${DEFAULT_REMOTE_NAME}]: " remote_name
  remote_name="$(trim_whitespace "${remote_name}")"
  if [[ -z "${remote_name}" ]]; then
    remote_name="${DEFAULT_REMOTE_NAME}"
  fi

  local default_branch
  read -r -p "Default branch [${DEFAULT_BRANCH_NAME}]: " default_branch
  default_branch="$(trim_whitespace "${default_branch}")"
  if [[ -z "${default_branch}" ]]; then
    default_branch="${DEFAULT_BRANCH_NAME}"
  fi

  printf '%s\n' "${project_mode}" > "${PROJECT_MODE_FILE}"
  printf '%s\n' "${remote_name}" > "${REMOTE_NAME_FILE}"
  printf '%s\n' "${default_branch}" > "${DEFAULT_BRANCH_FILE}"

  write_manifest "${ROOT_DIR}" "${STATE_DIR}" "${MANIFEST_FILE}"

  printf 'Governator initialized:\n'
  printf '  project mode: %s\n' "${project_mode}"
  printf '  default remote: %s\n' "${remote_name}"
  printf '  default branch: %s\n' "${default_branch}"

  git -C "${ROOT_DIR}" add -A
  if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain 2> /dev/null)" ]]; then
    git -C "${ROOT_DIR}" commit -q -m "[governator] Initialize configuration"
  fi
}

# read_remote_name
# Purpose: Read the configured git remote name.
# Args: None.
# Output: Prints the remote name to stdout.
# Returns: 0 always.
read_remote_name() {
  read_config_value "${REMOTE_NAME_FILE}" "${DEFAULT_REMOTE_NAME}"
}

# read_default_branch
# Purpose: Read the configured default branch name.
# Args: None.
# Output: Prints the branch name to stdout.
# Returns: 0 always.
read_default_branch() {
  read_config_value "${DEFAULT_BRANCH_FILE}" "${DEFAULT_BRANCH_NAME}"
}

# read_global_cap
# Purpose: Read the global worker concurrency cap.
# Args: None.
# Output: Prints the cap value to stdout.
# Returns: 0 always.
read_global_cap() {
  read_numeric_file "${GLOBAL_CAP_FILE}" "${DEFAULT_GLOBAL_CAP}"
}

# read_worker_timeout_seconds
# Purpose: Read the worker timeout value in seconds.
# Args: None.
# Output: Prints the timeout to stdout.
# Returns: 0 always.
read_worker_timeout_seconds() {
  read_numeric_file "${WORKER_TIMEOUT_FILE}" "${DEFAULT_WORKER_TIMEOUT_SECONDS}"
}

# read_done_check_cooldown_seconds
# Purpose: Read the done-check cooldown in seconds.
# Args: None.
# Output: Prints the cooldown value to stdout.
# Returns: 0 always.
read_done_check_cooldown_seconds() {
  read_numeric_file "${DONE_CHECK_COOLDOWN_FILE}" "3600"
}

# read_done_check_last_run
# Purpose: Read the last done-check run timestamp.
# Args: None.
# Output: Prints the timestamp to stdout.
# Returns: 0 always.
read_done_check_last_run() {
  read_numeric_file "${DONE_CHECK_LAST_RUN_FILE}" "0"
}

# write_done_check_last_run
# Purpose: Persist the last done-check run timestamp.
# Args:
#   $1: Unix timestamp (string or integer).
# Output: None.
# Returns: 0 on success.
write_done_check_last_run() {
  local timestamp="$1"
  printf '%s\n' "${timestamp}" > "${DONE_CHECK_LAST_RUN_FILE}"
}

# read_last_update_at
# Purpose: Read the last update timestamp for the update command.
# Args: None.
# Output: Prints the timestamp string or "never".
# Returns: 0 always.
read_last_update_at() {
  if [[ ! -f "${LAST_UPDATE_FILE}" ]]; then
    printf '%s\n' "never"
    return 0
  fi
  local value
  value="$(tr -d '[:space:]' < "${LAST_UPDATE_FILE}")"
  if [[ -z "${value}" ]]; then
    printf '%s\n' "never"
    return 0
  fi
  printf '%s\n' "${value}"
}

# write_last_update_at
# Purpose: Persist the last update timestamp for the update command.
# Args:
#   $1: Timestamp string (string).
# Output: None.
# Returns: 0 on success.
write_last_update_at() {
  local timestamp="$1"
  printf '%s\n' "${timestamp}" > "${LAST_UPDATE_FILE}"
}

# read_project_done_sha
# Purpose: Read the stored GOVERNATOR.md hash for done checks.
# Args: None.
# Output: Prints the SHA or empty string to stdout.
# Returns: 0 always.
read_project_done_sha() {
  if [[ ! -f "${PROJECT_DONE_FILE}" ]]; then
    printf '%s\n' ""
    return 0
  fi
  trim_whitespace "$(cat "${PROJECT_DONE_FILE}")"
}

# write_project_done_sha
# Purpose: Write the stored GOVERNATOR.md hash for done checks.
# Args:
#   $1: Git hash string (string, may be empty).
# Output: None.
# Returns: 0 on success.
write_project_done_sha() {
  local sha="$1"
  printf '%s\n' "${sha}" > "${PROJECT_DONE_FILE}"
}

# governator_doc_sha
# Purpose: Compute the Git hash of GOVERNATOR.md.
# Args: None.
# Output: Prints the hash to stdout; prints nothing on failure.
# Returns: 0 always.
governator_doc_sha() {
  git -C "${ROOT_DIR}" hash-object "${ROOT_DIR}/GOVERNATOR.md" 2> /dev/null || true
}

# read_reasoning_effort
# Purpose: Read the reasoning effort setting for a worker role.
# Args:
#   $1: Role name (string).
# Output: Prints the effort value (low|medium|high) to stdout.
# Returns: 0 always; falls back to default on missing/invalid data.
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

# read_worker_cap
# Purpose: Read the per-role worker concurrency cap.
# Args:
#   $1: Role name (string).
# Output: Prints the cap value to stdout.
# Returns: 0 always; falls back to default on missing/invalid data.
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

# ensure_db_dir
# Purpose: Create the state DB directory and initialize default files.
# Args: None.
# Output: Writes default config files as needed.
# Returns: 0 on success.
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

# touch_logs
# Purpose: Ensure log files exist to avoid read failures.
# Args: None.
# Output: None.
# Returns: 0 on success.
touch_logs() {
  touch "${FAILED_MERGES_LOG}" "${IN_FLIGHT_LOG}"
}
