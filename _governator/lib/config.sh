# shellcheck shell=bash

# config_json_read_value
# Purpose: Read a scalar value from the config.json file with fallback.
# Args:
#   $1: Dot-delimited key path (string).
#   $2: Fallback value (string or integer).
# Output: Prints the value or fallback to stdout.
# Returns: 0 always.
config_json_read_value() {
  local key_path="$1"
  local fallback="$2"
  if [[ ! -f "${CONFIG_FILE}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi
  local value
  if ! value="$(
    jq -r --arg path "${key_path}" --arg fallback "${fallback}" \
      'getpath($path | split(".")) // $fallback
       | if (type == "string" or type == "number") then . else $fallback end' \
      "${CONFIG_FILE}" 2> /dev/null
  )"; then
    printf '%s\n' "${fallback}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# config_json_read_map_value
# Purpose: Read a map entry from config.json with a per-map default.
# Args:
#   $1: Map key (string).
#   $2: Entry key (string).
#   $3: Default key within the map (string).
#   $4: Fallback value (string or integer).
# Output: Prints the entry value, map default, or fallback to stdout.
# Returns: 0 always.
config_json_read_map_value() {
  local map_key="$1"
  local entry_key="$2"
  local default_key="$3"
  local fallback="$4"
  if [[ ! -f "${CONFIG_FILE}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi
  local value
  if ! value="$(
    jq -r --arg map "${map_key}" --arg entry "${entry_key}" \
      --arg def "${default_key}" --arg fallback "${fallback}" \
      '(.[$map] // {}) as $m
       | ($m[$entry] // $m[$def] // $fallback)
       | if (type == "string" or type == "number") then . else $fallback end' \
      "${CONFIG_FILE}" 2> /dev/null
  )"; then
    printf '%s\n' "${fallback}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# config_json_write_value
# Purpose: Write a scalar value into config.json at the given dot path.
# Args:
#   $1: Dot-delimited key path (string).
#   $2: Value to write (string).
#   $3: Value type ("string" or "number").
# Output: None.
# Returns: 0 on success.
config_json_write_value() {
  local key_path="$1"
  local value="$2"
  local value_type="${3:-string}"
  local tmp_file
  tmp_file="$(mktemp "${DB_DIR}/config.XXXXXX")"
  local safe_value="${value}"
  local jq_args=()
  local jq_value_expr
  if [[ "${value_type}" == "number" ]]; then
    if [[ ! "${safe_value}" =~ ^-?[0-9]+$ ]]; then
      safe_value=0
    fi
    jq_args=(--argjson value "${safe_value}")
    jq_value_expr='$value'
  else
    jq_args=(--arg value "${safe_value}")
    jq_value_expr='$value'
  fi

  if [[ -f "${CONFIG_FILE}" ]] && jq -e . "${CONFIG_FILE}" > /dev/null 2>&1; then
    jq -S --arg path "${key_path}" "${jq_args[@]}" \
      "setpath(\$path | split(\".\"); ${jq_value_expr})" \
      "${CONFIG_FILE}" > "${tmp_file}"
  else
    jq -S -n --arg path "${key_path}" "${jq_args[@]}" \
      "setpath(\$path | split(\".\"); ${jq_value_expr})" \
      > "${tmp_file}"
  fi
  mv "${tmp_file}" "${CONFIG_FILE}"
}

# ensure_config_file
# Purpose: Ensure the config.json file exists, copying the template if missing.
# Args: None.
# Output: None.
# Returns: 0 on completion.
ensure_config_file() {
  if [[ -f "${CONFIG_FILE}" ]]; then
    return 0
  fi
  if [[ ! -f "${CONFIG_TEMPLATE}" ]]; then
    log_error "Missing config template at ${CONFIG_TEMPLATE}."
    return 1
  fi
  cp "${CONFIG_TEMPLATE}" "${CONFIG_FILE}"
}

# read_project_mode
# Purpose: Read and validate the project mode ("new" or "existing").
# Args: None.
# Output: Prints the project mode to stdout.
# Returns: 0 if valid mode exists; 1 otherwise.
read_project_mode() {
  local value
  value="$(config_json_read_value "project_mode" "")"
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
# Args:
#   --defaults: Use sane defaults (new, origin, main) without prompting.
#   --non-interactive: Require explicit inputs or fall back to defaults.
#   --project-mode=<new|existing>: Project mode for non-interactive init.
#   --remote=<name>: Remote name for non-interactive init.
#   --branch=<name>: Default branch for non-interactive init.
# Output: Prompts for project mode, remote, branch; logs initialization.
# Returns: 0 on success; exits 1 on invalid state.
init_governator() {
  ensure_db_dir
  ensure_gitignore_entries
  if read_project_mode > /dev/null 2>&1; then
    log_error "Governator is already initialized. Re-run init after clearing ${CONFIG_FILE}."
    exit 1
  fi

  local non_interactive=0
  local use_defaults=0
  local project_mode=""
  local remote_name=""
  local default_branch=""

  local arg
  for arg in "$@"; do
    case "${arg}" in
      --defaults)
        use_defaults=1
        non_interactive=1
        ;;
      --non-interactive)
        non_interactive=1
        ;;
      --project-mode=*)
        project_mode="${arg#*=}"
        ;;
      --remote=*)
        remote_name="${arg#*=}"
        ;;
      --branch=*)
        default_branch="${arg#*=}"
        ;;
      *)
        log_error "Unknown init option: ${arg}"
        exit 1
        ;;
    esac
  done

  if [[ "${use_defaults}" -eq 1 ]]; then
    project_mode="new"
    remote_name="${DEFAULT_REMOTE_NAME}"
    default_branch="${DEFAULT_BRANCH_NAME}"
  fi

  if [[ "${non_interactive}" -eq 1 ]]; then
    project_mode="$(trim_whitespace "${project_mode}")"
    project_mode="$(printf '%s' "${project_mode}" | tr '[:upper:]' '[:lower:]')"
    if [[ -z "${project_mode}" ]]; then
      log_error "Missing required --project-mode for non-interactive init."
      exit 1
    fi
    if [[ "${project_mode}" != "new" && "${project_mode}" != "existing" ]]; then
      log_error "Invalid --project-mode: ${project_mode}."
      exit 1
    fi
    remote_name="$(trim_whitespace "${remote_name}")"
    if [[ -z "${remote_name}" ]]; then
      remote_name="${DEFAULT_REMOTE_NAME}"
    fi
    default_branch="$(trim_whitespace "${default_branch}")"
    if [[ -z "${default_branch}" ]]; then
      default_branch="${DEFAULT_BRANCH_NAME}"
    fi
  else
    while true; do
      read -r -p "Is this a new or existing project? (new/existing): " project_mode
      project_mode="$(trim_whitespace "${project_mode}")"
      project_mode="$(printf '%s' "${project_mode}" | tr '[:upper:]' '[:lower:]')"
      if [[ "${project_mode}" == "new" || "${project_mode}" == "existing" ]]; then
        break
      fi
      printf 'Please enter "new" or "existing".\n'
    done

    read -r -p "Default remote [${DEFAULT_REMOTE_NAME}]: " remote_name
    remote_name="$(trim_whitespace "${remote_name}")"
    if [[ -z "${remote_name}" ]]; then
      remote_name="${DEFAULT_REMOTE_NAME}"
    fi

    read -r -p "Default branch [${DEFAULT_BRANCH_NAME}]: " default_branch
    default_branch="$(trim_whitespace "${default_branch}")"
    if [[ -z "${default_branch}" ]]; then
      default_branch="${DEFAULT_BRANCH_NAME}"
    fi
  fi

  config_json_write_value "project_mode" "${project_mode}" "string"
  config_json_write_value "remote_name" "${remote_name}" "string"
  config_json_write_value "default_branch" "${default_branch}" "string"

  ensure_sha256_tool
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
  local value
  value="$(config_json_read_value "remote_name" "")"
  if [[ -z "${value}" ]]; then
    printf '%s\n' "${DEFAULT_REMOTE_NAME}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# read_default_branch
# Purpose: Read the configured default branch name.
# Args: None.
# Output: Prints the branch name to stdout.
# Returns: 0 always.
read_default_branch() {
  local value
  value="$(config_json_read_value "default_branch" "")"
  if [[ -z "${value}" ]]; then
    printf '%s\n' "${DEFAULT_BRANCH_NAME}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# read_global_cap
# Purpose: Read the global worker concurrency cap from worker_caps.global.
# Args: None.
# Output: Prints the cap value to stdout.
# Returns: 0 always; trims whitespace before validation.
read_global_cap() {
  local value
  value="$(config_json_read_map_value "worker_caps" "global" "global" "${DEFAULT_GLOBAL_CAP}")"
  value="$(trim_whitespace "${value}")"
  if [[ -z "${value}" || ! "${value}" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "${DEFAULT_GLOBAL_CAP}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# read_worker_timeout_seconds
# Purpose: Read the worker timeout value in seconds.
# Args: None.
# Output: Prints the timeout to stdout.
# Returns: 0 always.
read_worker_timeout_seconds() {
  local value
  value="$(config_json_read_value "worker_timeout_seconds" "${DEFAULT_WORKER_TIMEOUT_SECONDS}")"
  if [[ -z "${value}" || ! "${value}" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "${DEFAULT_WORKER_TIMEOUT_SECONDS}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# read_completion_check_cooldown_seconds
# Purpose: Read the completion-check cooldown in seconds from done_check config.
# Args: None.
# Output: Prints the cooldown value to stdout.
# Returns: 0 always.
read_completion_check_cooldown_seconds() {
  local value
  value="$(config_json_read_value "done_check.cooldown_seconds" "3600")"
  if [[ -z "${value}" || ! "${value}" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "3600"
    return 0
  fi
  printf '%s\n' "${value}"
}

# read_completion_check_last_run
# Purpose: Read the last completion-check run timestamp from done_check config.
# Args: None.
# Output: Prints the timestamp to stdout.
# Returns: 0 always.
read_completion_check_last_run() {
  local value
  value="$(config_json_read_value "done_check.last_check" "0")"
  if [[ -z "${value}" || ! "${value}" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "0"
    return 0
  fi
  printf '%s\n' "${value}"
}

# write_completion_check_last_run
# Purpose: Persist the last completion-check run timestamp to done_check config.
# Args:
#   $1: Unix timestamp (string or integer).
# Output: None.
# Returns: 0 on success.
write_completion_check_last_run() {
  local timestamp="$1"
  config_json_write_value "done_check.last_check" "${timestamp}" "number"
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

# read_planning_gov_sha
# Purpose: Read the stored GOVERNATOR.md hash for planning reanalysis from planning config.
# Args: None.
# Output: Prints the SHA or empty string to stdout.
# Returns: 0 always.
read_planning_gov_sha() {
  local value
  value="$(config_json_read_value "planning.gov_hash" "")"
  if [[ -z "${value}" ]]; then
    printf '%s\n' ""
    return 0
  fi
  trim_whitespace "${value}"
}

# write_planning_gov_sha
# Purpose: Write the stored GOVERNATOR.md hash for planning reanalysis to planning config.
# Args:
#   $1: SHA-256 hash string (string, may be empty).
# Output: None.
# Returns: 0 on success.
write_planning_gov_sha() {
  local sha="$1"
  config_json_write_value "planning.gov_hash" "${sha}" "string"
}

# governator_doc_sha
# Purpose: Compute the SHA-256 hash of GOVERNATOR.md.
# Args: None.
# Output: Prints the hash to stdout; prints nothing on failure.
# Returns: 0 always.
governator_doc_sha() {
  sha256_file "${ROOT_DIR}/GOVERNATOR.md" 2> /dev/null || true
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
  local value
  value="$(config_json_read_map_value "reasoning_effort" "${role}" "default" "${fallback}")"

  if [[ -z "${value}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi
  case "${value}" in
    low|medium|high)
      printf '%s\n' "${value}"
      return 0
      ;;
    *)
      printf '%s\n' "${fallback}"
      return 0
      ;;
  esac
}

# read_worker_cap
# Purpose: Read the per-role worker concurrency cap, falling back to global.
# Args:
#   $1: Role name (string).
# Output: Prints the cap value to stdout.
# Returns: 0 always; trims whitespace and falls back on missing/invalid data.
read_worker_cap() {
  local role="$1"
  local cap
  cap="$(config_json_read_map_value "worker_caps" "${role}" "global" "${DEFAULT_WORKER_CAP}")"
  cap="$(trim_whitespace "${cap}")"
  if [[ -z "${cap}" || ! "${cap}" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "${DEFAULT_WORKER_CAP}"
    return 0
  fi
  if [[ "${cap}" == "${DEFAULT_WORKER_CAP}" ]]; then
    local direct_cap
    direct_cap="$(config_json_read_value "worker_caps.${role}" "")"
    direct_cap="$(trim_whitespace "${direct_cap}")"
    if [[ -n "${direct_cap}" && "${direct_cap}" =~ ^[0-9]+$ ]]; then
      cap="${direct_cap}"
    fi
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
  ensure_migrations_state_file
  ensure_config_file
}

# touch_logs
# Purpose: Ensure log files exist to avoid read failures.
# Args: None.
# Output: None.
# Returns: 0 on success.
touch_logs() {
  touch "${FAILED_MERGES_LOG}" "${IN_FLIGHT_LOG}"
}
