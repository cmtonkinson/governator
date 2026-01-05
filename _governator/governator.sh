#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

#############################################################################
# The Governator
#############################################################################
#
# Single-file implementation of the orchestrator. The script enforces a lock,
# requires a clean git state, processes worker branches, and assigns backlog
# tasks. It is intentionally explicit about filesystem and git transitions.
#
#############################################################################

#############################################################################
# Configuration
#############################################################################
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="${ROOT_DIR}/_governator"
DB_DIR="${ROOT_DIR}/.governator"

PROJECT_MODE_FILE="${DB_DIR}/project_mode"
DEFAULT_BRANCH_FILE="${DB_DIR}/default_branch"
REMOTE_NAME_FILE="${DB_DIR}/remote_name"

NEXT_TASK_FILE="${DB_DIR}/next_task_id"
GLOBAL_CAP_FILE="${DB_DIR}/global_worker_cap"
WORKER_CAPS_FILE="${DB_DIR}/worker_caps"
WORKER_TIMEOUT_FILE="${DB_DIR}/worker_timeout_seconds"
REASONING_EFFORT_FILE="${DB_DIR}/reasoning_effort"
DONE_CHECK_COOLDOWN_FILE="${DB_DIR}/done_check_cooldown_seconds"
DONE_CHECK_LAST_RUN_FILE="${DB_DIR}/last_done_check"
PROJECT_DONE_FILE="${DB_DIR}/project_done"

AUDIT_LOG="${DB_DIR}/audit.log"
WORKER_PROCESSES_LOG="${DB_DIR}/worker-processes.log"
RETRY_COUNTS_LOG="${DB_DIR}/retry-counts.log"

WORKER_ROLES_DIR="${STATE_DIR}/roles-worker"
SPECIAL_ROLES_DIR="${STATE_DIR}/roles-special"
TEMPLATES_DIR="${STATE_DIR}/templates"
LOCK_FILE="${DB_DIR}/governator.lock"
FAILED_MERGES_LOG="${DB_DIR}/failed-merges.log"
IN_FLIGHT_LOG="${DB_DIR}/in-flight.log"
SYSTEM_LOCK_FILE="${DB_DIR}/governator.locked"
SYSTEM_LOCK_PATH="${SYSTEM_LOCK_FILE#"${ROOT_DIR}/"}"
GITIGNORE_PATH="${ROOT_DIR}/.gitignore"
UPDATE_URL="https://gitlab.com/cmtonkinson/governator/-/raw/main/_governator/governator.sh"

GOV_QUIET=0
GOV_VERBOSE=0

DEFAULT_GLOBAL_CAP=1
DEFAULT_WORKER_CAP=1
DEFAULT_TASK_ID=1
DEFAULT_WORKER_TIMEOUT_SECONDS=900
DEFAULT_REMOTE_NAME="origin"
DEFAULT_BRANCH_NAME="main"

PROJECT_NAME="$(basename "${ROOT_DIR}")"

BOOTSTRAP_ROLE="architect"
BOOTSTRAP_TASK_NAME="000-architecture-bootstrap-${BOOTSTRAP_ROLE}"
BOOTSTRAP_NEW_TEMPLATE="${TEMPLATES_DIR}/000-architecture-bootstrap.md"
BOOTSTRAP_EXISTING_TEMPLATE="${TEMPLATES_DIR}/000-architecture-discovery.md"
BOOTSTRAP_DOCS_DIR="${ROOT_DIR}/_governator/docs"
BOOTSTRAP_NEW_REQUIRED_ARTIFACTS=("asr.md" "arc42.md")
BOOTSTRAP_NEW_OPTIONAL_ARTIFACTS=("personas.md" "wardley.md")
BOOTSTRAP_EXISTING_REQUIRED_ARTIFACTS=("existing-system-discovery.md")
BOOTSTRAP_EXISTING_OPTIONAL_ARTIFACTS=()

DONE_CHECK_REVIEW_ROLE="reviewer"
DONE_CHECK_REVIEW_TASK="000-done-check-${DONE_CHECK_REVIEW_ROLE}"
DONE_CHECK_PLANNER_ROLE="planner"
DONE_CHECK_PLANNER_TASK="000-done-check-${DONE_CHECK_PLANNER_ROLE}"
DONE_CHECK_REVIEW_TEMPLATE="${TEMPLATES_DIR}/000-done-check-reviewer.md"
DONE_CHECK_PLANNER_TEMPLATE="${TEMPLATES_DIR}/000-done-check-planner.md"

GITIGNORE_ENTRIES=(
  ".governator/governator.lock"
  ".governator/governator.locked"
  ".governator/failed-merges.log"
  ".governator/in-flight.log"
  ".governator/logs/"
)

# Standard UTC timestamp helpers.
timestamp_utc_seconds() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

timestamp_utc_minutes() {
  date -u +"%Y-%m-%dT%H:%MZ"
}

# Log with a consistent UTC timestamp prefix.
log_with_level() {
  local level="$1"
  shift
  printf '[%s] %-5s %s\n' "$(timestamp_utc_seconds)" "${level}" "$*"
}

log_info() {
  if [[ "${GOV_QUIET}" -eq 1 ]]; then
    return 0
  fi
  log_with_level "INFO" "$@" >&2
}

log_verbose() {
  if [[ "${GOV_QUIET}" -eq 1 || "${GOV_VERBOSE}" -eq 0 ]]; then
    return 0
  fi
  log_with_level "INFO" "$@" >&2
}

log_warn() {
  log_with_level "WARN" "$@" >&2
}

log_error() {
  log_with_level "ERROR" "$@" >&2
}

log_verbose_file() {
  local label="$1"
  local file="$2"
  if [[ "${GOV_QUIET}" -eq 1 || "${GOV_VERBOSE}" -eq 0 ]]; then
    return 0
  fi
  {
    log_with_level "INFO" "${label}: ${file}"
    cat "${file}"
  } >&2
}

# Append visible separators to per-task worker logs before each new worker starts.
append_worker_log_separator() {
  local log_file="$1"
  local separator
  separator="$(printf '=%.0s' {1..80})"
  {
    printf '\n\n'
    printf '%s\n' "${separator}"
    printf '%s\n' "${separator}"
    printf '%s\n' "${separator}"
    printf '\n\n'
  } >> "${log_file}"
}

# Remove lock on exit.
cleanup_lock() {
  if [[ -f "${LOCK_FILE}" ]]; then
    rm -f "${LOCK_FILE}"
  fi
}

# Ensure we don't run two governators simultaneously.
ensure_lock() {
  if [[ -f "${LOCK_FILE}" ]]; then
    log_warn "Lock file exists at ${LOCK_FILE}, exiting."
    exit 0
  fi
  printf '%s\n' "$(timestamp_utc_seconds)" > "${LOCK_FILE}"
  trap cleanup_lock EXIT
}

# Avoid processing while the repo has local edits.
ensure_clean_git() {
  local status
  status="$(git -C "${ROOT_DIR}" status --porcelain 2> /dev/null || true)"
  if [[ -n "${status}" && -n "${SYSTEM_LOCK_PATH}" ]]; then
    status="$(printf '%s\n' "${status}" | grep -v -F -- "${SYSTEM_LOCK_PATH}" || true)"
  fi
  if [[ -n "${status}" ]]; then
    status="$(
      printf '%s\n' "${status}" | grep -v -E \
        '^[[:space:][:alnum:]\?]{2}[[:space:]](_governator/governator\.lock|\.governator/governator\.locked|\.governator/audit\.log|\.governator/worker-processes\.log|\.governator/retry-counts\.log|\.governator/logs/)' ||
        true
    )"
  fi
  if [[ -n "${status}" ]]; then
    log_warn "Local git changes detected, exiting."
    exit 0
  fi
}

# Ensure required commands exist before running.
ensure_dependencies() {
  local missing=()
  local dep
  for dep in awk date find git jq mktemp nohup stat sgpt; do
    if ! command -v "${dep}" > /dev/null 2>&1; then
      missing+=("${dep}")
    fi
  done
  if ! command -v codex > /dev/null 2>&1; then
    missing+=("codex")
  fi
  if [[ "${#missing[@]}" -gt 0 ]]; then
    log_error "Missing dependencies: ${missing[*]}"
    exit 1
  fi
}

ensure_update_dependencies() {
  if ! command -v curl > /dev/null 2>&1; then
    log_error "Missing dependency: curl"
    exit 1
  fi
  if ! command -v shasum > /dev/null 2>&1; then
    log_error "Missing dependency: shasum"
    exit 1
  fi
}

require_governator_doc() {
  if [[ ! -f "${ROOT_DIR}/GOVERNATOR.md" ]]; then
    log_error "GOVERNATOR.md not found at project root; aborting."
    exit 1
  fi
}
# Checkout the default branch quietly.
git_checkout_default_branch() {
  local branch
  branch="$(read_default_branch)"
  git -C "${ROOT_DIR}" checkout "${branch}" > /dev/null 2>&1
}

# Pull the default branch from the default remote.
git_pull_default_branch() {
  local branch
  local remote
  branch="$(read_default_branch)"
  remote="$(read_remote_name)"
  git -C "${ROOT_DIR}" pull -q "${remote}" "${branch}" > /dev/null
}

# Push the default branch to the default remote.
git_push_default_branch() {
  local branch
  local remote
  branch="$(read_default_branch)"
  remote="$(read_remote_name)"
  git -C "${ROOT_DIR}" push -q "${remote}" "${branch}" > /dev/null
}

# Sync local default branch with the default remote.
sync_default_branch() {
  git_checkout_default_branch
  git_pull_default_branch
}

# Fetch and prune remote refs.
git_fetch_remote() {
  local remote
  remote="$(read_remote_name)"
  git -C "${ROOT_DIR}" fetch -q "${remote}" --prune > /dev/null
}

# Delete a worker branch locally and on the default remote (best-effort).
delete_worker_branch() {
  local branch="$1"
  local remote
  local base_branch
  remote="$(read_remote_name)"
  base_branch="$(read_default_branch)"
  if [[ -z "${branch}" || "${branch}" == "${base_branch}" || "${branch}" == "${remote}/${base_branch}" ]]; then
    return 0
  fi
  git -C "${ROOT_DIR}" branch -D "${branch}" > /dev/null 2>&1 || true
  if ! git -C "${ROOT_DIR}" push "${remote}" --delete "${branch}" > /dev/null 2>&1; then
    log_warn "Failed to delete remote branch ${branch} with --delete"
  fi
  if ! git -C "${ROOT_DIR}" push "${remote}" :"refs/heads/${branch}" > /dev/null 2>&1; then
    log_warn "Failed to delete remote branch ${branch} with explicit refs/heads"
  fi
  git -C "${ROOT_DIR}" fetch "${remote}" --prune > /dev/null 2>&1 || true
}

# Ensure state logs exist so reads do not fail.
touch_logs() {
  touch "${FAILED_MERGES_LOG}" "${IN_FLIGHT_LOG}"
}

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

trim_whitespace() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "${value}"
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
    value="${fallback}"
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

# Count in-flight tasks (all roles).
in_flight_entries() {
  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 0
  fi
  awk -F ' -> ' 'NF == 2 { print $1 "|" $2 }' "${IN_FLIGHT_LOG}"
}

count_in_flight_total() {
  local count=0
  local task
  local worker
  while IFS='|' read -r task worker; do
    count=$((count + 1))
  done < <(in_flight_entries)
  printf '%s\n' "${count}"
}

# Count in-flight tasks for a specific role.
count_in_flight_role() {
  local role="$1"
  local count=0
  local task
  local worker
  while IFS='|' read -r task worker; do
    if [[ "${worker}" == "${role}" ]]; then
      count=$((count + 1))
    fi
  done < <(in_flight_entries)
  printf '%s\n' "${count}"
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
      printf '%s\n' "# Use \"default\" to set the fallback for all roles."
      printf '%s\n' "default: medium"
    } > "${REASONING_EFFORT_FILE}"
  fi
  if [[ ! -f "${DONE_CHECK_COOLDOWN_FILE}" ]]; then
    printf '%s\n' "3600" > "${DONE_CHECK_COOLDOWN_FILE}"
  fi
}

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

  printf 'Governator initialized:\n'
  printf '  project mode: %s\n' "${project_mode}"
  printf '  default remote: %s\n' "${remote_name}"
  printf '  default branch: %s\n' "${default_branch}"

  git -C "${ROOT_DIR}" add -A
  if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain 2> /dev/null)" ]]; then
    git -C "${ROOT_DIR}" commit -q -m "[governator] Initialize configuration"
  fi
}

update_governator() {
  ensure_update_dependencies

  local script_path="${STATE_DIR}/governator.sh"
  if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain -- "${script_path}")" ]]; then
    log_error "Local changes detected in ${script_path}; commit or stash before update."
    exit 1
  fi

  local tmp_file
  tmp_file="$(mktemp)"
  if ! curl -fsSL "${UPDATE_URL}" -o "${tmp_file}"; then
    rm -f "${tmp_file}"
    log_error "Failed to download ${UPDATE_URL}"
    exit 1
  fi
  if [[ ! -s "${tmp_file}" ]]; then
    rm -f "${tmp_file}"
    log_error "Downloaded update is empty; aborting."
    exit 1
  fi

  local local_hash
  local remote_hash
  local_hash="$(shasum -a 256 "${script_path}" | awk '{print $1}')"
  remote_hash="$(shasum -a 256 "${tmp_file}" | awk '{print $1}')"
  if [[ -n "${local_hash}" && "${local_hash}" == "${remote_hash}" ]]; then
    rm -f "${tmp_file}"
    log_info "Already up to date."
    return 0
  fi

  mv "${tmp_file}" "${script_path}"
  chmod +x "${script_path}"
  git -C "${ROOT_DIR}" add "${script_path}"
  if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain -- "${script_path}")" ]]; then
    git -C "${ROOT_DIR}" commit -q -m "[governator] Update governator.sh"
  fi
  log_info "Updated ${script_path} from ${UPDATE_URL}"
}

system_locked() {
  [[ -f "${SYSTEM_LOCK_FILE}" ]]
}

locked_since() {
  if [[ -f "${SYSTEM_LOCK_FILE}" ]]; then
    cat "${SYSTEM_LOCK_FILE}"
    return 0
  fi
  return 1
}

lock_governator() {
  ensure_db_dir
  printf '%s\n' "$(timestamp_utc_seconds)" > "${SYSTEM_LOCK_FILE}"
}

unlock_governator() {
  ensure_db_dir
  rm -f "${SYSTEM_LOCK_FILE}"
}

format_duration() {
  local seconds="$1"
  if [[ -z "${seconds}" || "${seconds}" -lt 0 ]]; then
    printf 'n/a'
    return
  fi
  local hours=$((seconds / 3600))
  local minutes=$((seconds / 60 % 60))
  local secs=$((seconds % 60))
  if [[ "${hours}" -gt 0 ]]; then
    printf '%dh%02dm%02ds' "${hours}" "${minutes}" "${secs}"
  elif [[ "${minutes}" -gt 0 ]]; then
    printf '%dm%02ds' "${minutes}" "${secs}"
  else
    printf '%02ds' "${secs}"
  fi
}

list_task_files_in_dir() {
  local dir="$1"
  if [[ ! -d "${dir}" ]]; then
    return 0
  fi
  local path
  while IFS= read -r path; do
    local base
    base="$(basename "${path}")"
    if [[ "${base}" == ".keep" ]]; then
      continue
    fi
    printf '%s\n' "${path}"
  done < <(find "${dir}" -maxdepth 1 -type f -name '*.md' 2> /dev/null | sort)
}

count_task_files() {
  local dir="$1"
  local count=0
  local path
  while IFS= read -r path; do
    count=$((count + 1))
  done < <(list_task_files_in_dir "${dir}")
  printf '%s\n' "${count}"
}

task_label() {
  local file="$1"
  local name
  name="$(basename "${file}" .md)"
  local role
  if role="$(extract_worker_from_task "${file}" 2> /dev/null)"; then
    printf '%s (%s)' "${name}" "${role}"
  else
    printf '%s' "${name}"
  fi
}

extract_block_reason() {
  local file="$1"
  local reason
  reason="$(
    awk '
      /^## Governator Block/ {
        while (getline && $0 ~ /^[[:space:]]*$/) {}
        if ($0 != "") {
          print
          exit
        }
      }
    ' "${file}" 2> /dev/null
  )"
  if [[ -z "${reason}" ]]; then
    reason="$(
      awk '
        /^## Merge Failure/ {
          while (getline && $0 ~ /^[[:space:]]*$/) {}
          if ($0 != "") {
            print
            exit
          }
        }
      ' "${file}" 2> /dev/null
    )"
  fi
  if [[ -z "${reason}" ]]; then
    reason="reason unavailable"
  fi
  printf '%s\n' "${reason}"
}

print_task_queue_summary() {
  local entries=(
    "task-backlog:Backlog"
    "task-assigned:Assigned"
    "task-worked:Awaiting review"
    "task-feedback:Feedback"
    "task-blocked:Blocked"
    "task-done:Done"
  )
  printf 'Task queues:\n'
  local pair
  for pair in "${entries[@]}"; do
    local dir="${pair%%:*}"
    local label="${pair##*:}"
    local count
    count="$(count_task_files "${STATE_DIR}/${dir}")"
    printf '  %-22s %s\n' "${label}:" "${count}"
  done
}

format_task_label() {
  local path="$1"
  task_label "${path}"
}

format_blocked_task() {
  local path="$1"
  printf '%s (%s)' "$(task_label "${path}")" "$(extract_block_reason "${path}")"
}

print_task_list() {
  local title="$1"
  local dir="$2"
  local formatter="$3"
  local limit="${4:-0}"
  printf '%s:\n' "${title}"
  local printed=0
  local path
  while IFS= read -r path; do
    printed=$((printed + 1))
    printf '  - %s\n' "$("${formatter}" "${path}")"
    if [[ "${limit}" -gt 0 && "${printed}" -ge "${limit}" ]]; then
      break
    fi
  done < <(list_task_files_in_dir "${dir}")
  if [[ "${printed}" -eq 0 ]]; then
    printf '  (none)\n'
  fi
}

print_stage_task_list() {
  local title="$1"
  local dir="$2"
  local limit="${3:-5}"
  print_task_list "${title}" "${dir}" format_task_label "${limit}"
}

print_blocked_tasks_summary() {
  print_task_list "Blocked tasks" "${STATE_DIR}/task-blocked" format_blocked_task
}

print_inflight_summary() {
  local total
  total="$(count_in_flight_total)"
  printf 'In-flight workers (%s):\n' "${total}"
  local now
  now="$(date +%s)"
  local printed=0
  local task
  local worker
  while IFS='|' read -r task worker; do
    local branch="n/a"
    local pid="n/a"
    local age="n/a"
    local info=()
    mapfile -t info < <(worker_process_get "${task}" "${worker}" 2> /dev/null)
    if [[ "${#info[@]}" -gt 0 ]]; then
      pid="${info[0]:-n/a}"
      branch="${info[2]:-n/a}"
      local started="${info[3]:-}"
      if [[ "${started}" =~ ^[0-9]+$ ]]; then
        local elapsed=$((now - started))
        age="$(format_duration "${elapsed}")"
      fi
    fi
    printf '  %-28s %-12s %-28s PID:%-6s age:%s\n' "${task}" "${worker}" "${branch}" "${pid}" "${age}"
    printed=$((printed + 1))
  done < <(in_flight_entries)
  if [[ "${printed}" -eq 0 ]]; then
    printf '  (none)\n'
  fi
}

print_activity_snapshot() {
  print_inflight_summary
  printf '\n'
  print_stage_task_list "Pending reviews" "${STATE_DIR}/task-worked"
  printf '\n'
  print_blocked_tasks_summary
}

status_dashboard() {
  local locked_note=''
  if system_locked; then
    local since
    if since="$(locked_since)"; then
      locked_note=" (LOCKED since ${since})"
    else
      locked_note=' (LOCKED)'
    fi
  fi
  printf 'Governator Status%s\n' "${locked_note}"
  print_task_queue_summary
  printf '\n'
  print_inflight_summary
  printf '\n'
  print_stage_task_list "Pending reviews" "${STATE_DIR}/task-worked"
  printf '\n'
  print_blocked_tasks_summary
  if system_locked; then
    printf '\nNOTE: Governator is locked; no new activity will start and data may be stale.\n'
  fi
}

handle_locked_state() {
  local context="$1"
  if system_locked; then
    printf 'Governator is locked; skipping %s. Active work snapshot:\n' "${context}"
    print_activity_snapshot
    return 0
  fi
  return 1
}

abort_task() {
  local prefix="$1"
  if [[ -z "${prefix:-}" ]]; then
    log_error "Usage: abort <task-prefix>"
    exit 1
  fi

  local task_file
  if ! task_file="$(task_file_for_prefix "${prefix}")"; then
    log_error "No task matches prefix ${prefix}"
    exit 1
  fi

  local task_name
  task_name="$(basename "${task_file}" .md)"
  local worker
  if ! worker="$(extract_worker_from_task "${task_file}" 2> /dev/null)"; then
    worker=""
  fi

  local worker_info=()
  local pid=""
  local tmp_dir=""
  local branch=""
  if mapfile -t worker_info < <(worker_process_get "${task_name}" "${worker}" 2> /dev/null); then
    pid="${worker_info[0]:-}"
    tmp_dir="${worker_info[1]:-}"
    branch="${worker_info[2]:-}"
  fi
  local expected_branch="worker/${worker}/${task_name}"
  if [[ -z "${branch}" ]]; then
    branch="${expected_branch}"
  fi

  if [[ -n "${pid}" ]]; then
    if kill -0 "${pid}" > /dev/null 2>&1; then
      kill -9 "${pid}" > /dev/null 2>&1 || true
    fi
  fi

  if [[ -n "${tmp_dir}" && -d "${tmp_dir}" ]]; then
    cleanup_tmp_dir "${tmp_dir}"
  fi
  cleanup_worker_tmp_dirs "${worker}" "${task_name}"

  delete_worker_branch "${branch}"

  in_flight_remove "${task_name}" "${worker}"

  local blocked_dest="${STATE_DIR}/task-blocked/${task_name}.md"
  sync_default_branch
  if [[ "${task_file}" != "${blocked_dest}" ]]; then
    move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "aborted by operator"
  else
    log_task_event "${task_name}" "aborted by operator"
  fi

  local abort_meta
  abort_meta="Aborted by operator.
Worker: ${worker:-n/a}
PID: ${pid:-n/a}
Branch: ${branch:-n/a}"
  annotate_abort "${blocked_dest}" "${abort_meta}"
  annotate_blocked "${blocked_dest}" "Aborted by operator command."

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Abort task ${task_name}"
  git_push_default_branch
}

# Read the next task id from disk, defaulting to 1.
read_next_task_id() {
  ensure_db_dir
  if [[ ! -f "${NEXT_TASK_FILE}" ]]; then
    printf '%s\n' "${DEFAULT_TASK_ID}"
    return 0
  fi

  local value
  value="$(tr -d '[:space:]' < "${NEXT_TASK_FILE}")"
  if [[ -z "${value}" ]]; then
    printf '%s\n' "${DEFAULT_TASK_ID}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# Persist the next task id.
write_next_task_id() {
  local value="$1"
  ensure_db_dir
  printf '%s\n' "${value}" > "${NEXT_TASK_FILE}"
}

# Format a numeric task id as zero-padded 3 digits.
format_task_id() {
  local value="$1"
  printf '%03d' "${value}"
}

# Allocate the next task id and increment the stored value.
allocate_task_id() {
  local current
  current="$(read_next_task_id)"
  if ! [[ "${current}" =~ ^[0-9]+$ ]]; then
    log_warn "Invalid task id value '${current}', resetting to 1."
    current=1
  fi

  local next=$((current + 1))
  write_next_task_id "${next}"
  printf '%s\n' "${current}"
}

# Create a new task file using the template and allocated id.
create_task_file() {
  local short_name="$1"
  local role="$2"
  local target_dir="$3"

  local task_id
  task_id="$(allocate_task_id)"

  local id_segment
  id_segment="$(format_task_id "${task_id}")"
  local filename="${id_segment}-${short_name}-${role}.md"

  local template="${TEMPLATES_DIR}/task.md"
  if [[ ! -f "${template}" ]]; then
    log_error "Missing task template at ${template}."
    return 1
  fi

  local dest="${target_dir}/${filename}"
  cp "${template}" "${dest}"
  printf '%s\n' "${dest}"
}
# Append a titled section to a task file.
append_section() {
  local file="$1"
  local title="$2"
  local actor="$3"
  local body="$4"
  local prefix
  prefix="$(timestamp_utc_seconds) [${actor}]: "
  {
    printf '\n%s\n\n' "${title}"
    while IFS= read -r line; do
      printf '%s%s\n' "${prefix}" "${line}"
    done <<< "${body}"
  } >> "${file}"
}

# Append a lifecycle event to the audit log.
audit_log() {
  local task_name="$1"
  local message="$2"
  printf '%s %s -> %s\n' "$(timestamp_utc_minutes)" "${task_name}" "${message}" >> "${AUDIT_LOG}"
}

# Record a task event to stdout and the audit log.
log_task_event() {
  local task_name="$1"
  shift
  local message="$*"
  log_info "${task_name} -> ${message}"
  audit_log "${task_name}" "${message}"
}

# Record a warning-level task event to stdout and the audit log.
log_task_warn() {
  local task_name="$1"
  shift
  local message="$*"
  log_warn "${task_name} -> ${message}"
  audit_log "${task_name}" "${message}"
}

commit_audit_log_if_dirty() {
  if [[ ! -f "${AUDIT_LOG}" ]]; then
    return 0
  fi
  if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain -- "${AUDIT_LOG}")" ]]; then
    git -C "${ROOT_DIR}" add "${AUDIT_LOG}"
    git -C "${ROOT_DIR}" commit -q -m "[governator] Update audit log"
    git_push_default_branch
  fi
}

# Move a task file to a new queue and record an audit entry.
move_task_file() {
  local task_file="$1"
  local dest_dir="$2"
  local task_name="$3"
  local audit_message="$4"
  mv "${task_file}" "${dest_dir}/$(basename "${task_file}")"
  log_task_event "${task_name}" "${audit_message}"
}

move_task_file_renamed() {
  local task_file="$1"
  local dest_dir="$2"
  local task_name="$3"
  local new_name="$4"
  local audit_message="$5"
  mv "${task_file}" "${dest_dir}/${new_name}.md"
  log_task_event "${task_name}" "${audit_message}"
}

warn_if_task_template_incomplete() {
  local task_file="$1"
  local task_name="$2"
  if [[ "${task_name}" == 000-* ]]; then
    return 0
  fi

  local sections=(
    "## Objective"
    "## Context"
    "## Requirements"
    "## Non-Goals"
    "## Constraints"
    "## Acceptance Criteria"
  )
  local missing=()
  local section
  for section in "${sections[@]}"; do
    if ! grep -Fq "${section}" "${task_file}"; then
      missing+=("${section}")
    fi
  done
  if [[ "${#missing[@]}" -gt 0 ]]; then
    log_warn "Task ${task_name} missing template sections: ${missing[*]}"
  fi
}

move_done_check_to_planner() {
  local task_file="$1"
  local task_name="$2"
  local dest="${STATE_DIR}/task-assigned/${DONE_CHECK_PLANNER_TASK}.md"
  if [[ ! -f "${DONE_CHECK_PLANNER_TEMPLATE}" ]]; then
    log_error "Missing done-check template at ${DONE_CHECK_PLANNER_TEMPLATE}."
    return 1
  fi
  cp "${DONE_CHECK_PLANNER_TEMPLATE}" "${dest}"
  append_section "${dest}" "## Reviewer Notes" "reviewer" "$(cat "${task_file}")"
  move_task_file "${task_file}" "${STATE_DIR}/task-done" "${task_name}" "moved to task-done"
  log_task_event "${DONE_CHECK_PLANNER_TASK}" "created planner follow-up"
}

# Read a file mtime in epoch seconds (BSD/GNU stat compatible).
file_mtime_epoch() {
  local path="$1"
  if stat -f %m "${path}" > /dev/null 2>&1; then
    stat -f %m "${path}" 2> /dev/null || return 1
    return 0
  fi
  stat -c %Y "${path}" 2> /dev/null || return 1
}

# Normalize tmp paths so /tmp and /private/tmp compare consistently.
normalize_tmp_path() {
  local path="$1"
  if [[ -d "/private/tmp" && "${path}" == /tmp/* ]]; then
    printf '%s\n' "/private${path}"
    return 0
  fi
  printf '%s\n' "${path}"
}

cleanup_tmp_dir() {
  local dir="$1"
  if [[ -n "${dir}" && -d "${dir}" ]]; then
    rm -rf "${dir}"
  fi
}

cleanup_worker_tmp_dirs() {
  local worker="$1"
  local task_name="$2"
  if [[ -z "${worker}" || -z "${task_name}" ]]; then
    return 0
  fi
  local roots=(/tmp)
  if [[ -d "/private/tmp" ]]; then
    roots+=(/private/tmp)
  fi

  local root
  for root in "${roots[@]}"; do
    find "${root}" -maxdepth 1 -type d -name "governator-${PROJECT_NAME}-${worker}-${task_name}-*" -exec rm -rf {} + > /dev/null 2>&1 || true
  done
}

# Filter worker process log entries by task and worker.
filter_worker_process_log() {
  local task_name="$1"
  local worker="$2"
  local tmp_file
  tmp_file="$(mktemp)"
  if [[ -f "${WORKER_PROCESSES_LOG}" ]]; then
    awk -v task="${task_name}" -v worker_name="${worker}" '
      $0 ~ / \| / {
        split($0, parts, " \\| ")
        if (parts[1] == task && parts[2] == worker_name) next
      }
      { print }
    ' "${WORKER_PROCESSES_LOG}" > "${tmp_file}"
  fi
  printf '%s\n' "${tmp_file}"
}

# Filter retry count entries by task.
filter_retry_counts_log() {
  local task_name="$1"
  local tmp_file
  tmp_file="$(mktemp)"
  if [[ -f "${RETRY_COUNTS_LOG}" ]]; then
    awk -v task="${task_name}" '
      $0 ~ / \| / {
        split($0, parts, " \\| ")
        if (parts[1] == task) next
      }
      { print }
    ' "${RETRY_COUNTS_LOG}" > "${tmp_file}"
  fi
  printf '%s\n' "${tmp_file}"
}

# Filter in-flight entries by task and optional worker.
filter_in_flight_log() {
  local task_name="$1"
  local worker_name="${2:-}"
  local tmp_file
  tmp_file="$(mktemp)"
  if [[ -f "${IN_FLIGHT_LOG}" ]]; then
    if [[ -n "${worker_name}" ]]; then
      awk -v task="${task_name}" -v worker="${worker_name}" '
        $0 ~ / -> / {
          split($0, parts, " -> ")
          if (parts[1] == task && parts[2] == worker) next
        }
        { print }
      ' "${IN_FLIGHT_LOG}" > "${tmp_file}"
    else
      awk -v task="${task_name}" '
        $0 ~ / -> / {
          split($0, parts, " -> ")
          if (parts[1] == task) next
        }
        { print }
      ' "${IN_FLIGHT_LOG}" > "${tmp_file}"
    fi
  fi
  printf '%s\n' "${tmp_file}"
}

# Record the worker process that owns a task.
worker_process_set() {
  local task_name="$1"
  local worker="$2"
  local pid="$3"
  local tmp_dir="$4"
  local branch="$5"
  local started_at="$6"

  local tmp_file
  tmp_file="$(filter_worker_process_log "${task_name}" "${worker}")"
  printf '%s | %s | %s | %s | %s | %s\n' "${task_name}" "${worker}" "${pid}" "${tmp_dir}" "${branch}" "${started_at}" >> "${tmp_file}"
  mv "${tmp_file}" "${WORKER_PROCESSES_LOG}"
}

# Remove a worker process record.
worker_process_clear() {
  local task_name="$1"
  local worker="$2"

  if [[ ! -f "${WORKER_PROCESSES_LOG}" ]]; then
    return 0
  fi

  local tmp_file
  tmp_file="$(filter_worker_process_log "${task_name}" "${worker}")"
  mv "${tmp_file}" "${WORKER_PROCESSES_LOG}"
}

# Lookup a worker process record.
worker_process_get() {
  local task_name="$1"
  local worker="$2"

  if [[ ! -f "${WORKER_PROCESSES_LOG}" ]]; then
    return 1
  fi

  awk -v task="${task_name}" -v worker_name="${worker}" '
    $0 ~ / \| / {
      split($0, parts, " \\| ")
      if (parts[1] == task && parts[2] == worker_name) {
        print parts[3]
        print parts[4]
        print parts[5]
        print parts[6]
        exit 0
      }
    }
    END { exit 1 }
  ' "${WORKER_PROCESSES_LOG}"
}

# Remove stale worker tmp dirs that are not tracked as active.
cleanup_stale_worker_dirs() {
  local tmp_root="/tmp"
  if [[ -d "/private/tmp" ]]; then
    tmp_root="/private/tmp"
  fi

  local dry_run="${1:-}"
  local timeout
  timeout="$(read_worker_timeout_seconds)"
  local now
  now="$(date +%s)"

  local active_dirs=()
  if [[ -f "${WORKER_PROCESSES_LOG}" ]]; then
    while IFS=' | ' read -r task_name worker pid tmp_dir branch started_at; do
      if [[ -n "${tmp_dir}" ]]; then
        active_dirs+=("$(normalize_tmp_path "${tmp_dir}")")
      fi
    done < "${WORKER_PROCESSES_LOG}"
  fi

  local dir
  while IFS= read -r dir; do
    if [[ -z "${dir}" ]]; then
      continue
    fi
    dir="$(normalize_tmp_path "${dir}")"
    local active=0
    local active_dir
    for active_dir in "${active_dirs[@]}"; do
      if [[ "${active_dir}" == "${dir}" ]]; then
        active=1
        break
      fi
    done
    if [[ "${active}" -eq 1 ]]; then
      continue
    fi

    local mtime
    mtime="$(file_mtime_epoch "${dir}")"
    if [[ -z "${mtime}" || ! "${mtime}" =~ ^[0-9]+$ ]]; then
      continue
    fi
    local age=$((now - mtime))
    if [[ "${age}" -ge "${timeout}" ]]; then
      if [[ "${dry_run}" == "--dry-run" ]]; then
        printf '%s\n' "${dir}"
      else
        cleanup_tmp_dir "${dir}"
      fi
    fi
  done < <(find "${tmp_root}" -maxdepth 1 -type d -name "governator-${PROJECT_NAME}-*" 2> /dev/null)
}

# Read the retry count for a task (defaults to 0).
retry_count_get() {
  local task_name="$1"
  if [[ ! -f "${RETRY_COUNTS_LOG}" ]]; then
    printf '0\n'
    return 0
  fi

  local count
  count="$(
    awk -v task="${task_name}" '
      $0 ~ / \| / {
        split($0, parts, " \\| ")
        if (parts[1] == task) {
          print parts[2]
          exit 0
        }
      }
      END { exit 1 }
    ' "${RETRY_COUNTS_LOG}" || true
  )"

  if [[ -z "${count}" || ! "${count}" =~ ^[0-9]+$ ]]; then
    printf '0\n'
    return 0
  fi
  printf '%s\n' "${count}"
}

# Write the retry count for a task.
retry_count_set() {
  local task_name="$1"
  local count="$2"

  local tmp_file
  tmp_file="$(filter_retry_counts_log "${task_name}")"
  printf '%s | %s\n' "${task_name}" "${count}" >> "${tmp_file}"
  mv "${tmp_file}" "${RETRY_COUNTS_LOG}"
}

# Clear the retry count for a task.
retry_count_clear() {
  local task_name="$1"
  if [[ ! -f "${RETRY_COUNTS_LOG}" ]]; then
    return 0
  fi

  local tmp_file
  tmp_file="$(filter_retry_counts_log "${task_name}")"
  mv "${tmp_file}" "${RETRY_COUNTS_LOG}"
}
# Join arguments by a delimiter.
join_by() {
  local delimiter="$1"
  shift
  local first=1
  local item
  for item in "$@"; do
    if [[ "${first}" -eq 1 ]]; then
      printf '%s' "${item}"
      first=0
    else
      printf '%s%s' "${delimiter}" "${item}"
    fi
  done
}

# Start the worker without blocking this script.
run_codex_worker_detached() {
  local dir="$1"
  local prompt="$2"
  local log_file="$3"
  local role="$4"
  if [[ -n "${CODEX_WORKER_CMD:-}" ]]; then
    log_verbose "Worker command: GOV_PROMPT=${prompt} nohup bash -c ${CODEX_WORKER_CMD}"
    (
      cd "${dir}"
      GOV_PROMPT="${prompt}" nohup bash -c "${CODEX_WORKER_CMD}" >> "${log_file}" 2>&1 &
      echo $!
    )
    return 0
  fi

  # Use nohup to prevent worker exit from being tied to this process.
  local reasoning
  reasoning="$(read_reasoning_effort "${role}")"
  (
    cd "${dir}"
    log_verbose "Worker command: codex --full-auto --search -c sandbox_workspace_write.network_access=true -c model_reasoning_effort=\"${reasoning}\" exec --sandbox=workspace-write \"${prompt}\""
    nohup codex --full-auto --search -c sandbox_workspace_write.network_access=true -c model_reasoning_effort="${reasoning}" exec --sandbox=workspace-write "${prompt}" >> "${log_file}" 2>&1 &
    echo $!
  )
}

# Run the worker synchronously (blocking) for special roles.
run_codex_worker_blocking() {
  local dir="$1"
  local prompt="$2"
  local log_file="$3"
  local role="$4"
  if [[ -n "${CODEX_WORKER_CMD:-}" ]]; then
    log_verbose "Worker command: GOV_PROMPT=${prompt} bash -c ${CODEX_WORKER_CMD}"
    (cd "${dir}" && GOV_PROMPT="${prompt}" bash -c "${CODEX_WORKER_CMD}" >> "${log_file}" 2>&1)
    return $?
  fi

  local reasoning
  reasoning="$(read_reasoning_effort "${role}")"
  log_verbose "Worker command: codex --full-auto --search -c sandbox_workspace_write.network_access=true -c model_reasoning_effort=\"${reasoning}\" exec --sandbox=workspace-write \"${prompt}\""
  (cd "${dir}" && codex --full-auto --search -c sandbox_workspace_write.network_access=true -c model_reasoning_effort="${reasoning}" exec --sandbox=workspace-write "${prompt}" >> "${log_file}" 2>&1)
}

# Run the reviewer synchronously so a review.json is produced.
run_codex_reviewer() {
  local dir="$1"
  local prompt="$2"
  local log_file="${3:-}"
  if [[ -n "${CODEX_REVIEW_CMD:-}" ]]; then
    log_verbose "Reviewer command: GOV_PROMPT=${prompt} bash -c ${CODEX_REVIEW_CMD}"
    if [[ -n "${log_file}" ]]; then
      (cd "${dir}" && GOV_PROMPT="${prompt}" bash -c "${CODEX_REVIEW_CMD}" >> "${log_file}" 2>&1)
    else
      (cd "${dir}" && GOV_PROMPT="${prompt}" bash -c "${CODEX_REVIEW_CMD}")
    fi
    return 0
  fi

  local reasoning
  reasoning="$(read_reasoning_effort "reviewer")"
  log_verbose "Reviewer command: codex --full-auto --search -c sandbox_workspace_write.network_access=true -c model_reasoning_effort=\"${reasoning}\" exec --sandbox=workspace-write \"${prompt}\""
  if [[ -n "${log_file}" ]]; then
    (cd "${dir}" && codex --full-auto --search -c sandbox_workspace_write.network_access=true -c model_reasoning_effort="${reasoning}" exec --sandbox=workspace-write "${prompt}" >> "${log_file}" 2>&1)
  else
    (cd "${dir}" && codex --full-auto --search -c sandbox_workspace_write.network_access=true -c model_reasoning_effort="${reasoning}" exec --sandbox=workspace-write "${prompt}")
  fi
}

format_prompt_files() {
  local result=""
  local item
  for item in "$@"; do
    if [[ -n "${result}" ]]; then
      result+=", "
    fi
    result+="${item}"
  done
  printf '%s' "${result}"
}

build_worker_prompt() {
  local role="$1"
  local task_relpath="$2"
  local prompt_files=()
  prompt_files+=("_governator/worker-contract.md")
  prompt_files+=("_governator/roles-worker/${role}.md")
  prompt_files+=("_governator/custom-prompts/_global.md")
  prompt_files+=("_governator/custom-prompts/${role}.md")
  prompt_files+=("${task_relpath}")

  local prompt
  prompt="Read and follow the instructions in the following files, in this order: $(format_prompt_files "${prompt_files[@]}")."
  printf '%s' "${prompt}"
}

build_special_prompt() {
  local role="$1"
  local task_relpath="$2"
  local prompt_files=()
  prompt_files+=("_governator/worker-contract.md")
  prompt_files+=("${SPECIAL_ROLES_DIR#"${ROOT_DIR}/"}/${role}.md")
  prompt_files+=("_governator/custom-prompts/_global.md")
  prompt_files+=("_governator/custom-prompts/${role}.md")
  prompt_files+=("${task_relpath}")

  local prompt
  prompt="Read and follow the instructions in the following files, in this order: $(format_prompt_files "${prompt_files[@]}")."
  printf '%s' "${prompt}"
}

# List remote worker branches.
list_worker_branches() {
  local remote
  remote="$(read_remote_name)"
  git -C "${ROOT_DIR}" for-each-ref --format='%(refname:short)' "refs/remotes/${remote}/worker/*/*" || true
}

# Check whether a branch is recorded as a failed merge.
is_failed_merge_branch() {
  local branch="$1"
  if [[ ! -f "${FAILED_MERGES_LOG}" ]]; then
    return 1
  fi
  if awk -v branch="${branch}" '
    NF >= 2 {
      if ($2 == branch) { found=1 }
    }
    END { exit found ? 0 : 1 }
  ' "${FAILED_MERGES_LOG}"; then
    return 0
  fi
  return 1
}

# Add an in-flight record.
in_flight_add() {
  local task_name="$1"
  local worker_name="$2"
  printf '%s -> %s\n' "${task_name}" "${worker_name}" >> "${IN_FLIGHT_LOG}"
}

# Remove an in-flight record when a task completes or is blocked.
in_flight_remove() {
  local task_name="$1"
  local worker_name="$2"
  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 0
  fi

  local tmp_file
  tmp_file="$(filter_in_flight_log "${task_name}" "${worker_name}")"
  mv "${tmp_file}" "${IN_FLIGHT_LOG}"
  if [[ -n "${worker_name}" ]]; then
    worker_process_clear "${task_name}" "${worker_name}"
  fi
  retry_count_clear "${task_name}"
}

# Find a task file in any task-* directory by base name.
find_task_files() {
  local pattern="$1"
  find "${STATE_DIR}" -maxdepth 2 -type f -path "${STATE_DIR}/task-*/${pattern}.md" 2> /dev/null | sort
}

task_exists() {
  local task_name="$1"
  if find_task_files "${task_name}" | grep -q .; then
    return 0
  fi
  return 1
}

task_file_for_name() {
  local task_name="$1"
  local matches=()
  while IFS= read -r path; do
    matches+=("${path}")
  done < <(find_task_files "${task_name}" || true)

  if [[ "${#matches[@]}" -eq 0 ]]; then
    return 1
  fi
  if [[ "${#matches[@]}" -gt 1 ]]; then
    log_warn "Multiple task files found for ${task_name}, using ${matches[0]}"
  fi
  printf '%s\n' "${matches[0]}"
}

task_dir_for_branch() {
  local branch="$1"
  local task_name="$2"
  local path
  path="$(
    git -C "${ROOT_DIR}" ls-tree -r --name-only "${branch}" "${STATE_DIR}" 2> /dev/null |
      awk -v task="${task_name}.md" '$0 ~ ("/" task "$") { print; exit }'
  )"
  if [[ -z "${path}" ]]; then
    return 1
  fi
  basename "$(dirname "${path}")"
}

task_file_for_prefix() {
  local prefix="$1"
  if [[ -z "${prefix}" ]]; then
    return 1
  fi
  local matches=()
  local path
  while IFS= read -r path; do
    matches+=("${path}")
  done < <(find_task_files "${prefix}*" || true)

  if [[ "${#matches[@]}" -eq 0 ]]; then
    return 1
  fi
  if [[ "${#matches[@]}" -gt 1 ]]; then
    log_error "Multiple task files match prefix ${prefix}; please be more specific."
    return 1
  fi
  printf '%s\n' "${matches[0]}"
}

# Enumerate non-reviewer worker roles.
list_available_workers() {
  local worker
  while IFS= read -r path; do
    worker="$(basename "${path}" .md)"
    if [[ "${worker}" == "reviewer" ]]; then
      continue
    fi
    printf '%s\n' "${worker}"
  done < <(find "${WORKER_ROLES_DIR}" -maxdepth 1 -type f -name '*.md' | sort)
}

role_exists() {
  local role="$1"
  [[ -f "${WORKER_ROLES_DIR}/${role}.md" ]]
}

special_role_exists() {
  local role="$1"
  [[ -f "${SPECIAL_ROLES_DIR}/${role}.md" ]]
}

bootstrap_template_path() {
  local mode
  if ! mode="$(read_project_mode)"; then
    printf '%s\n' "${BOOTSTRAP_NEW_TEMPLATE}"
    return 0
  fi
  if [[ "${mode}" == "existing" ]]; then
    printf '%s\n' "${BOOTSTRAP_EXISTING_TEMPLATE}"
    return 0
  fi
  printf '%s\n' "${BOOTSTRAP_NEW_TEMPLATE}"
}

bootstrap_required_artifacts() {
  local mode
  if ! mode="$(read_project_mode)"; then
    printf '%s\n' "${BOOTSTRAP_NEW_REQUIRED_ARTIFACTS[@]}"
    return 0
  fi
  if [[ "${mode}" == "existing" ]]; then
    printf '%s\n' "${BOOTSTRAP_EXISTING_REQUIRED_ARTIFACTS[@]}"
    return 0
  fi
  printf '%s\n' "${BOOTSTRAP_NEW_REQUIRED_ARTIFACTS[@]}"
}

bootstrap_optional_artifacts() {
  local mode
  if ! mode="$(read_project_mode)"; then
    printf '%s\n' "${BOOTSTRAP_NEW_OPTIONAL_ARTIFACTS[@]}"
    return 0
  fi
  if [[ "${mode}" == "existing" ]]; then
    printf '%s\n' "${BOOTSTRAP_EXISTING_OPTIONAL_ARTIFACTS[@]}"
    return 0
  fi
  printf '%s\n' "${BOOTSTRAP_NEW_OPTIONAL_ARTIFACTS[@]}"
}

bootstrap_task_path() {
  local path
  while IFS= read -r path; do
    if [[ -n "${path}" ]]; then
      printf '%s\n' "${path}"
      return 0
    fi
  done < <(find_task_files "${BOOTSTRAP_TASK_NAME}" || true)
  return 1
}

bootstrap_task_dir() {
  local task_file
  if ! task_file="$(bootstrap_task_path)"; then
    return 1
  fi
  basename "$(dirname "${task_file}")"
}

ensure_bootstrap_task_exists() {
  if bootstrap_task_path > /dev/null 2>&1; then
    return 0
  fi
  local template
  template="$(bootstrap_template_path)"
  if [[ ! -f "${template}" ]]; then
    log_error "Missing bootstrap template at ${template}."
    return 1
  fi

  local dest="${STATE_DIR}/task-backlog/${BOOTSTRAP_TASK_NAME}.md"
  cp "${template}" "${dest}"
  log_task_event "${BOOTSTRAP_TASK_NAME}" "created bootstrap task"
  git -C "${ROOT_DIR}" add "${dest}" "${AUDIT_LOG}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Create architecture bootstrap task"
  git_push_default_branch
}

done_check_due() {
  local last_run
  last_run="$(read_done_check_last_run)"
  local cooldown
  cooldown="$(read_done_check_cooldown_seconds)"
  local now
  now="$(date +%s)"
  if [[ "${last_run}" -eq 0 ]]; then
    return 0
  fi
  if [[ $((now - last_run)) -ge "${cooldown}" ]]; then
    return 0
  fi
  return 1
}

done_check_needed() {
  local gov_sha
  gov_sha="$(governator_doc_sha)"
  if [[ -z "${gov_sha}" ]]; then
    return 0
  fi
  local done_sha
  done_sha="$(read_project_done_sha)"
  if [[ "${done_sha}" != "${gov_sha}" ]]; then
    return 0
  fi
  return 1
}

create_done_check_task() {
  if task_exists "${DONE_CHECK_REVIEW_TASK}" || task_exists "${DONE_CHECK_PLANNER_TASK}"; then
    return 0
  fi

  if [[ ! -f "${DONE_CHECK_REVIEW_TEMPLATE}" ]]; then
    log_error "Missing done-check template at ${DONE_CHECK_REVIEW_TEMPLATE}."
    return 1
  fi

  local dest="${STATE_DIR}/task-assigned/${DONE_CHECK_REVIEW_TASK}.md"
  cp "${DONE_CHECK_REVIEW_TEMPLATE}" "${dest}"
  annotate_assignment "${dest}" "${DONE_CHECK_REVIEW_ROLE}"
  log_task_event "${DONE_CHECK_REVIEW_TASK}" "created done check task"

  write_done_check_last_run "$(date +%s)"

  git -C "${ROOT_DIR}" add "${dest}" "${AUDIT_LOG}" "${DONE_CHECK_LAST_RUN_FILE}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Create done check task"
  git_push_default_branch
}

artifact_present() {
  local file="$1"
  [[ -f "${BOOTSTRAP_DOCS_DIR}/${file}" && -s "${BOOTSTRAP_DOCS_DIR}/${file}" ]]
}

artifact_skipped_in_task() {
  local task_file="$1"
  local artifact="$2"
  if [[ ! -f "${task_file}" ]]; then
    return 1
  fi
  local base="${artifact%.md}"
  grep -Eiq "(skip|omit|n/a|not needed).*${base}|${base}.*(skip|omit|n/a|not needed)" "${task_file}"
}

bootstrap_required_artifacts_ok() {
  local artifacts=()
  mapfile -t artifacts < <(bootstrap_required_artifacts)
  local artifact
  for artifact in "${artifacts[@]}"; do
    if ! artifact_present "${artifact}"; then
      return 1
    fi
  done
  return 0
}

bootstrap_optional_artifacts_ok() {
  local task_file
  if ! task_file="$(bootstrap_task_path)"; then
    return 1
  fi
  local artifacts=()
  mapfile -t artifacts < <(bootstrap_optional_artifacts)
  local artifact
  for artifact in "${artifacts[@]}"; do
    if artifact_present "${artifact}"; then
      continue
    fi
    if ! artifact_skipped_in_task "${task_file}" "${artifact}"; then
      return 1
    fi
  done
  return 0
}

bootstrap_adrs_ok() {
  local mode
  if mode="$(read_project_mode)"; then
    if [[ "${mode}" == "existing" ]]; then
      return 0
    fi
  fi
  if [[ -d "${BOOTSTRAP_DOCS_DIR}" ]]; then
    if find "${BOOTSTRAP_DOCS_DIR}" "${BOOTSTRAP_DOCS_DIR}/adr" -maxdepth 1 -type f -iname 'adr*.md' -print -quit 2> /dev/null | grep -q .; then
      return 0
    fi
  fi
  local task_file
  if task_file="$(bootstrap_task_path)"; then
    if grep -Eiq "no adr|no adrs|adr not required" "${task_file}"; then
      return 0
    fi
  fi
  return 1
}

has_non_bootstrap_tasks() {
  local path
  while IFS= read -r path; do
    local base
    base="$(basename "${path}")"
    if [[ "${base}" == ".keep" ]]; then
      continue
    fi
    if [[ "${base}" == "${BOOTSTRAP_TASK_NAME}.md" ]]; then
      continue
    fi
    printf '%s\n' "${path}"
    return 0
  done < <(find "${STATE_DIR}" -maxdepth 2 -type f -path "${STATE_DIR}/task-*/*" -name '*.md' 2> /dev/null | sort)
  return 1
}

bootstrap_requirements_met() {
  if ! bootstrap_task_path > /dev/null 2>&1; then
    return 1
  fi
  if ! bootstrap_required_artifacts_ok; then
    return 1
  fi
  if ! bootstrap_optional_artifacts_ok; then
    return 1
  fi
  if ! bootstrap_adrs_ok; then
    return 1
  fi
  return 0
}

architecture_bootstrap_complete() {
  local task_dir
  if ! task_dir="$(bootstrap_task_dir)"; then
    return 1
  fi
  if [[ "${task_dir}" != "task-done" ]]; then
    return 1
  fi
  if ! bootstrap_requirements_met; then
    return 1
  fi
  return 0
}

complete_bootstrap_task_if_ready() {
  if ! bootstrap_requirements_met; then
    return 1
  fi
  if has_non_bootstrap_tasks > /dev/null 2>&1; then
    return 1
  fi
  if in_flight_has_task "${BOOTSTRAP_TASK_NAME}"; then
    return 0
  fi
  local task_file
  if ! task_file="$(bootstrap_task_path)"; then
    return 0
  fi
  local task_dir
  task_dir="$(basename "$(dirname "${task_file}")")"
  if [[ "${task_dir}" == "task-done" ]]; then
    return 0
  fi
  move_task_file "${task_file}" "${STATE_DIR}/task-done" "${BOOTSTRAP_TASK_NAME}" "moved to task-done"
  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Complete architecture bootstrap"
  git_push_default_branch
  return 0
}

spawn_special_worker_for_task() {
  local task_file="$1"
  local worker="$2"
  local audit_message="$3"

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local tmp_dir
  tmp_dir="$(mktemp -d "/tmp/governator-${PROJECT_NAME}-${worker}-${task_name}-XXXXXX")"

  local log_dir
  log_dir="${DB_DIR}/logs"
  mkdir -p "${log_dir}"
  local log_file
  if [[ "${worker}" == "reviewer" ]]; then
    log_file="${log_dir}/${task_name}-reviewer.log"
  else
    log_file="${log_dir}/${task_name}.log"
  fi
  append_worker_log_separator "${log_file}"

  local remote
  local branch
  remote="$(read_remote_name)"
  branch="$(read_default_branch)"
  git clone "$(git -C "${ROOT_DIR}" remote get-url "${remote}")" "${tmp_dir}" > /dev/null 2>&1
  git -C "${tmp_dir}" checkout -b "worker/${worker}/${task_name}" "${remote}/${branch}" > /dev/null 2>&1

  local task_relpath="${task_file#"${ROOT_DIR}/"}"
  local prompt
  prompt="$(build_special_prompt "${worker}" "${task_relpath}")"

  if [[ "${worker}" == "reviewer" && -f "${TEMPLATES_DIR}/review.json" ]]; then
    cp "${TEMPLATES_DIR}/review.json" "${tmp_dir}/review.json"
  fi

  local started_at
  started_at="$(date +%s)"
  if [[ -n "${audit_message}" ]]; then
    log_task_event "${task_name}" "${audit_message}"
  fi

  log_task_event "${task_name}" "starting special worker ${worker}"
  local worker_status=0
  if [[ "${worker}" == "reviewer" ]]; then
    if ! run_codex_reviewer "${tmp_dir}" "${prompt}" "${log_file}"; then
      worker_status=1
    fi
  elif ! run_codex_worker_blocking "${tmp_dir}" "${prompt}" "${log_file}" "${worker}"; then
    worker_status=1
  fi

  if [[ "${worker_status}" -eq 0 ]]; then
    log_task_event "${task_name}" "special worker ${worker} completed"
  else
    log_task_warn "${task_name}" "special worker ${worker} exited with error"
  fi

  if [[ "${worker}" == "reviewer" ]]; then
    log_verbose_file "Reviewer output file" "${tmp_dir}/review.json"
    local review_output=()
    mapfile -t review_output < <(parse_review_json "${tmp_dir}/review.json")
    if [[ "${#review_output[@]}" -eq 0 ]]; then
      review_output=("block" "Review output missing")
    fi
    local decision="${review_output[0]}"
    local review_lines=("${review_output[@]:1}")
    local block_reason="Unexpected task state during processing."
    git_checkout_default_branch
    apply_review_decision "${task_name}" "${worker}" "${decision}" "${block_reason}" "${review_lines[@]}"
    git_push_default_branch
    rm -f "${tmp_dir}/review.json"
  elif [[ "${worker_status}" -eq 0 ]]; then
    process_special_worker_branch "${task_name}" "${worker}"
  fi
  local finished_at
  finished_at="$(date +%s)"
  if [[ "${finished_at}" -ge "${started_at}" ]]; then
    log_task_event "${task_name}" "worker elapsed ${worker}: $((finished_at - started_at))s"
  fi
  cleanup_tmp_dir "${tmp_dir}"
}

assign_bootstrap_task() {
  local task_file="$1"
  local worker="${BOOTSTRAP_ROLE}"

  sync_default_branch

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local assigned_file="${STATE_DIR}/task-assigned/${task_name}.md"
  annotate_assignment "${task_file}" "${worker}"
  move_task_file "${task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "assigned to ${worker}"

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Assign task ${task_name}"
  git_push_default_branch

  in_flight_add "${task_name}" "${worker}"
  spawn_special_worker_for_task "${assigned_file}" "${worker}" ""
  in_flight_remove "${task_name}" "${worker}"
}

parse_task_metadata() {
  local task_file="$1"
  local task_name
  task_name="$(basename "${task_file}" .md)"

  local role="${task_name##*-}"
  if [[ -z "${role}" || "${role}" == "${task_name}" ]]; then
    return 1
  fi
  local short_name="${task_name%-"${role}"}"
  printf '%s\n' "${task_name}" "${short_name}" "${role}"
}

# Extract the required worker role from the task filename suffix.
extract_worker_from_task() {
  local task_file="$1"
  local metadata_text
  if ! metadata_text="$(parse_task_metadata "${task_file}")"; then
    return 1
  fi
  local metadata=()
  mapfile -t metadata <<< "${metadata_text}"
  printf '%s' "${metadata[2]}"
}

# Check whether a task is already in flight.
in_flight_has_task() {
  local task_name="$1"
  local task
  local worker
  while IFS='|' read -r task worker; do
    if [[ "${task}" == "${task_name}" ]]; then
      return 0
    fi
  done < <(in_flight_entries)
  return 1
}

# Check whether a worker is already in flight.
in_flight_has_worker() {
  local worker_name="$1"
  local task
  local worker
  while IFS='|' read -r task worker; do
    if [[ "${worker}" == "${worker_name}" ]]; then
      return 0
    fi
  done < <(in_flight_entries)
  return 1
}

worker_elapsed_seconds() {
  local task_name="$1"
  local worker="$2"
  local branch="$3"
  local proc_info=()
  if ! mapfile -t proc_info < <(worker_process_get "${task_name}" "${worker}"); then
    return 1
  fi
  local started_at="${proc_info[3]:-}"
  if [[ -z "${started_at}" || ! "${started_at}" =~ ^[0-9]+$ ]]; then
    return 1
  fi
  local finished_at
  finished_at="$(git -C "${ROOT_DIR}" log -1 --format=%ct "${branch}" 2> /dev/null || true)"
  if [[ -z "${finished_at}" || ! "${finished_at}" =~ ^[0-9]+$ ]]; then
    return 1
  fi
  if [[ "${finished_at}" -lt "${started_at}" ]]; then
    return 1
  fi
  printf '%s\n' "$((finished_at - started_at))"
}

# Block a task when required metadata is missing or invalid.
block_task_from_backlog() {
  local task_file="$1"
  local reason="$2"

  sync_default_branch

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local blocked_file="${STATE_DIR}/task-blocked/${task_name}.md"
  move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
  annotate_blocked "${blocked_file}" "${reason}"

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Block task ${task_name}"
  git_push_default_branch
}

block_task_from_assigned() {
  local task_file="$1"
  local reason="$2"

  sync_default_branch

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local blocked_file="${STATE_DIR}/task-blocked/${task_name}.md"
  move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
  annotate_blocked "${blocked_file}" "${reason}"

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Block task ${task_name}"
  git_push_default_branch
}

# Record assignment details in the task file.
annotate_assignment() {
  local task_file="$1"
  local worker="$2"
  append_section "${task_file}" "## Assignment" "governator" "Assigned to ${worker}."
}

# Record review decision and comments in the task file.
annotate_review() {
  local task_file="$1"
  local decision="$2"
  local comments=("$@")
  comments=("${comments[@]:2}")

  local body="Decision: ${decision}"
  if [[ "${#comments[@]}" -gt 0 ]]; then
    body+=$'\nComments:'
    for comment in "${comments[@]}"; do
      body+=$'\n- '"${comment}"
    done
  fi
  append_section "${task_file}" "## Review Result" "reviewer" "${body}"
}

# Add feedback to a task file before reassigning.
annotate_feedback() {
  local task_file="$1"
  append_section "${task_file}" "## Feedback" "governator" "Moved back to task-assigned for follow-up."
}

# Capture a blocking reason in the task file.
annotate_blocked() {
  local task_file="$1"
  local reason="$2"
  append_section "${task_file}" "## Governator Block" "governator" "${reason}"
}

annotate_abort() {
  local task_file="$1"
  local abort_metadata="$2"
  append_section "${task_file}" "## Abort" "governator" "${abort_metadata}"
}

# Record a merge failure for reviewer visibility.
annotate_merge_failure() {
  local task_file="$1"
  local branch="$2"
  local base_branch
  base_branch="$(read_default_branch)"
  append_section "${task_file}" "## Merge Failure" "governator" "Unable to fast-forward merge ${branch} into ${base_branch}."
}

# Parse review.json for decision and comments.
parse_review_json() {
  local file="$1"
  if [[ ! -f "${file}" ]]; then
    printf 'block\nReview file missing at %s\n' "${file}"
    return 0
  fi

  if ! jq -e '.result' "${file}" > /dev/null 2>&1; then
    printf 'block\nFailed to parse review.json\n'
    return 0
  fi

  local result
  result="$(jq -r '.result // ""' "${file}")"
  printf '%s\n' "${result}"
  jq -r '.comments // [] | if type == "array" then .[] else . end' "${file}"
}

# Apply a reviewer decision to the task file and commit the state update.
apply_review_decision() {
  local task_name="$1"
  local worker_name="$2"
  local decision="$3"
  local block_reason="$4"
  shift 4
  local review_lines=("$@")

  local main_task_file
  if ! main_task_file="$(task_file_for_name "${task_name}")"; then
    log_warn "Task file missing for ${task_name} after review; skipping state update."
    return 1
  fi

  local task_dir
  task_dir="$(basename "$(dirname "${main_task_file}")")"

  case "${task_dir}" in
    task-worked | task-assigned)
      if [[ "${task_dir}" == "task-assigned" && ! ("${worker_name}" == "reviewer" && "${task_name}" == 000-*) ]]; then
        log_warn "Unexpected task state ${task_dir} for ${task_name}, blocking."
        annotate_blocked "${main_task_file}" "${block_reason}"
        move_task_file "${main_task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
      else
        annotate_review "${main_task_file}" "${decision}" "${review_lines[@]}"
        log_task_event "${task_name}" "review decision: ${decision}"
        case "${decision}" in
          approve)
            if [[ "${task_name}" == "${DONE_CHECK_REVIEW_TASK}" ]]; then
              write_project_done_sha "$(governator_doc_sha)"
              move_task_file "${main_task_file}" "${STATE_DIR}/task-done" "${task_name}" "moved to task-done"
            else
              move_task_file "${main_task_file}" "${STATE_DIR}/task-done" "${task_name}" "moved to task-done"
            fi
            ;;
          reject)
            if [[ "${task_name}" == "${DONE_CHECK_REVIEW_TASK}" ]]; then
              write_project_done_sha ""
              move_done_check_to_planner "${main_task_file}" "${task_name}"
            else
              move_task_file "${main_task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "moved to task-assigned"
            fi
            ;;
          *)
            if [[ "${task_name}" == "${DONE_CHECK_REVIEW_TASK}" ]]; then
              write_project_done_sha ""
              move_done_check_to_planner "${main_task_file}" "${task_name}"
            else
              move_task_file "${main_task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
            fi
            ;;
        esac
      fi
      ;;
    task-feedback)
      annotate_feedback "${main_task_file}"
      move_task_file "${main_task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "moved to task-assigned"
      ;;
    *)
      log_warn "Unexpected task state ${task_dir} for ${task_name}, blocking."
      annotate_blocked "${main_task_file}" "${block_reason}"
      move_task_file "${main_task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
      ;;
  esac

  git -C "${ROOT_DIR}" add "${STATE_DIR}" "${AUDIT_LOG}"
  if [[ -f "${PROJECT_DONE_FILE}" ]]; then
    git -C "${ROOT_DIR}" add "${PROJECT_DONE_FILE}"
  fi
  git -C "${ROOT_DIR}" commit -q -m "[governator] Process task ${task_name}"
  return 0
}

# Run reviewer flow in a clean clone and return parsed review output.
code_review() {
  local remote_branch="$1"
  local local_branch="$2"
  local task_relpath="$3"

  local tmp_dir
  tmp_dir="$(mktemp -d "/tmp/governator-${PROJECT_NAME}-reviewer-${local_branch//\//-}-XXXXXX")"

  local remote
  remote="$(read_remote_name)"
  git clone "$(git -C "${ROOT_DIR}" remote get-url "${remote}")" "${tmp_dir}" > /dev/null 2>&1
  git -C "${tmp_dir}" fetch "${remote}" > /dev/null 2>&1
  git -C "${tmp_dir}" checkout -B "${local_branch}" "${remote_branch}" > /dev/null 2>&1

  # Seed with a template to guide reviewers toward the expected schema.
  if [[ -f "${TEMPLATES_DIR}/review.json" ]]; then
    cp "${TEMPLATES_DIR}/review.json" "${tmp_dir}/review.json"
  fi

  local log_dir
  log_dir="${DB_DIR}/logs"
  mkdir -p "${log_dir}"
  local task_base
  task_base="$(basename "${task_relpath}" .md)"
  local log_file
  log_file="${log_dir}/${task_base}-reviewer.log"
  append_worker_log_separator "${log_file}"

  local prompt
  prompt="$(build_special_prompt "reviewer" "${task_relpath}")"

  log_task_event "${task_base}" "starting review for ${local_branch}"

  if ! run_codex_reviewer "${tmp_dir}" "${prompt}" "${log_file}"; then
    log_warn "Reviewer command failed for ${local_branch}."
  fi

  log_verbose_file "Reviewer output file" "${tmp_dir}/review.json"

  local review_output=()
  mapfile -t review_output < <(parse_review_json "${tmp_dir}/review.json")
  cleanup_tmp_dir "${tmp_dir}"

  if [[ "${#review_output[@]}" -eq 0 ]]; then
    printf 'block\nReview output missing\n'
    return 0
  fi

  printf '%s\n' "${review_output[@]}"
}

# Move task to assigned, commit, push, then spawn a worker.
assign_task() {
  local task_file="$1"
  local worker="$2"

  sync_default_branch

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local assigned_file="${STATE_DIR}/task-assigned/${task_name}.md"
  annotate_assignment "${task_file}" "${worker}"
  move_task_file "${task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "assigned to ${worker}"

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Assign task ${task_name}"
  git_push_default_branch

  warn_if_task_template_incomplete "${assigned_file}" "${task_name}"
  spawn_worker_for_task "${assigned_file}" "${worker}" ""
}

# Check caps for a worker/task pair; prints reason on failure.
can_assign_task() {
  local worker="$1"
  local task_name="$2"

  local total_count
  total_count="$(count_in_flight_total)"
  local global_cap
  global_cap="$(read_global_cap)"
  if [[ "${total_count}" -ge "${global_cap}" ]]; then
    printf 'Global worker cap reached (%s/%s), skipping %s.' "${total_count}" "${global_cap}" "${task_name}"
    return 1
  fi

  local role_count
  role_count="$(count_in_flight_role "${worker}")"
  local role_cap
  role_cap="$(read_worker_cap "${worker}")"
  if [[ "${role_count}" -ge "${role_cap}" ]]; then
    printf 'Role %s at cap (%s/%s) for %s, skipping.' "${worker}" "${role_count}" "${role_cap}" "${task_name}"
    return 1
  fi

  return 0
}

# Assign tasks in backlog based on role prefix/suffix in filename.
assign_pending_tasks() {
  touch_logs
  require_project_mode
  ensure_bootstrap_task_exists
  complete_bootstrap_task_if_ready || true

  # Gate normal task assignment until bootstrap completes.
  if ! architecture_bootstrap_complete; then
    log_verbose "Not bootstrapped; skipping task assignment"
    local blocking_task
    if blocking_task="$(has_non_bootstrap_tasks)"; then
      log_warn "Bootstrap incomplete; ignoring task ${blocking_task}."
    fi
    local bootstrap_task
    if bootstrap_task="$(bootstrap_task_path)"; then
      local task_dir
      task_dir="$(basename "$(dirname "${bootstrap_task}")")"
      if [[ "${task_dir}" == "task-backlog" ]]; then
        if ! in_flight_has_task "${BOOTSTRAP_TASK_NAME}"; then
          assign_bootstrap_task "${bootstrap_task}"
        fi
      fi
    fi
    return 0
  fi

  local queues_empty=1
  if [[ "$(count_task_files "${STATE_DIR}/task-backlog")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-assigned")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-worked")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-feedback")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-blocked")" -gt 0 ]]; then
    queues_empty=0
  fi

  if [[ "${queues_empty}" -eq 1 ]]; then
    log_verbose "All queues empty"
    if done_check_needed; then
      if done_check_due; then
        create_done_check_task || true
      else
        local last_run
        last_run="$(read_done_check_last_run)"
        local cooldown
        cooldown="$(read_done_check_cooldown_seconds)"
        local now
        now="$(date +%s)"
        local remaining=$((cooldown - (now - last_run)))
        if [[ "${remaining}" -lt 0 ]]; then
          remaining=0
        fi
        log_verbose "Done check cooldown active (${remaining}s remaining)"
      fi
    else
      log_verbose "Done check not needed (project_done matches GOVERNATOR.md)"
    fi
  else
    log_verbose "Tasks pending; skipping done check"
  fi

  local task_file
  while IFS= read -r task_file; do
    if [[ "${task_file}" == *"/.keep" ]]; then
      continue
    fi

    local metadata_text
    if ! metadata_text="$(parse_task_metadata "${task_file}")"; then
      local task_name
      task_name="$(basename "${task_file}" .md)"
      log_warn "Missing required role for ${task_name}, blocking."
      block_task_from_backlog "${task_file}" "Missing required role in filename suffix."
      continue
    fi
    local metadata=()
    mapfile -t metadata <<< "${metadata_text}"
    local task_name="${metadata[0]}"
    local worker="${metadata[2]}"

    if in_flight_has_task "${task_name}"; then
      continue
    fi

    if ! role_exists "${worker}"; then
      log_warn "Unknown role ${worker} for ${task_name}, blocking."
      block_task_from_backlog "${task_file}" "Unknown role ${worker} referenced in filename suffix."
      continue
    fi

    local cap_note
    if ! cap_note="$(can_assign_task "${worker}" "${task_name}")"; then
      log_warn "${cap_note}"
      continue
    fi

    log_verbose "Assigning backlog task ${task_name} to ${worker}"
    assign_task "${task_file}" "${worker}"
    in_flight_add "${task_name}" "${worker}"
  done < <(list_task_files_in_dir "${STATE_DIR}/task-backlog")
}

# Re-run tasks sitting in task-assigned when not already in flight.
resume_assigned_tasks() {
  touch_logs
  require_project_mode

  log_verbose "Resuming assigned tasks"
  local task_file
  while IFS= read -r task_file; do
    if [[ "${task_file}" == *"/.keep" ]]; then
      continue
    fi

    local metadata_text
    if ! metadata_text="$(parse_task_metadata "${task_file}")"; then
      local task_name
      task_name="$(basename "${task_file}" .md)"
      log_warn "Missing required role for ${task_name}, blocking."
      block_task_from_assigned "${task_file}" "Missing required role in filename suffix."
      continue
    fi
    local metadata=()
    mapfile -t metadata <<< "${metadata_text}"
    local task_name="${metadata[0]}"
    local worker="${metadata[2]}"

    if in_flight_has_task "${task_name}"; then
      log_verbose "Skipping in-flight task ${task_name}"
      continue
    fi

    if ! role_exists "${worker}" && ! special_role_exists "${worker}"; then
      log_warn "Unknown role ${worker} for ${task_name}, blocking."
      block_task_from_assigned "${task_file}" "Unknown role ${worker} referenced in filename suffix."
      continue
    fi

    local cap_note
    if ! cap_note="$(can_assign_task "${worker}" "${task_name}")"; then
      log_warn "${cap_note}"
      continue
    fi

    if special_role_exists "${worker}"; then
      log_verbose "Dispatching special role ${worker} for ${task_name}"
      warn_if_task_template_incomplete "${task_file}" "${task_name}"
      in_flight_add "${task_name}" "${worker}"
      spawn_special_worker_for_task "${task_file}" "${worker}" "retrying ${worker} task"
      in_flight_remove "${task_name}" "${worker}"
      continue
    fi

    log_verbose "Dispatching worker ${worker} for ${task_name}"
    warn_if_task_template_incomplete "${task_file}" "${task_name}"
    in_flight_add "${task_name}" "${worker}"
    spawn_worker_for_task "${task_file}" "${worker}" "retrying ${worker} task"
  done < <(list_task_files_in_dir "${STATE_DIR}/task-assigned")
}

# Spawn a worker for a task file with shared setup.
spawn_worker_for_task() {
  local task_file="$1"
  local worker="$2"
  local audit_message="$3"

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local tmp_dir
  tmp_dir="$(mktemp -d "/tmp/governator-${PROJECT_NAME}-${worker}-${task_name}-XXXXXX")"

  local log_dir
  log_dir="${DB_DIR}/logs"
  mkdir -p "${log_dir}"
  local log_file
  log_file="${log_dir}/${task_name}.log"
  append_worker_log_separator "${log_file}"

  local remote
  local branch
  remote="$(read_remote_name)"
  branch="$(read_default_branch)"
  git clone "$(git -C "${ROOT_DIR}" remote get-url "${remote}")" "${tmp_dir}" > /dev/null 2>&1
  git -C "${tmp_dir}" checkout -b "worker/${worker}/${task_name}" "${remote}/${branch}" > /dev/null 2>&1

  local task_relpath="${task_file#"${ROOT_DIR}/"}"
  local prompt
  prompt="$(build_worker_prompt "${worker}" "${task_relpath}")"

  local branch_name="worker/${worker}/${task_name}"
  local pid
  local started_at
  started_at="$(date +%s)"
  pid="$(run_codex_worker_detached "${tmp_dir}" "${prompt}" "${log_file}" "${worker}")"
  if [[ -n "${pid}" ]]; then
    worker_process_set "${task_name}" "${worker}" "${pid}" "${tmp_dir}" "${branch_name}" "${started_at}"
    if [[ -n "${audit_message}" ]]; then
      log_task_event "${task_name}" "${audit_message}"
    fi
    log_task_event "${task_name}" "worker ${worker} started"
  else
    log_task_warn "${task_name}" "failed to capture worker pid"
  fi
}

# Handle missing branches with dead workers.
check_zombie_workers() {
  touch_logs

  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 0
  fi

  local task_name
  local worker
  while IFS='|' read -r task_name worker; do
    local branch="worker/${worker}/${task_name}"

    local remote
    remote="$(read_remote_name)"
    if git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/remotes/${remote}/${branch}"; then
      continue
    fi

    local proc_info=()
    if ! mapfile -t proc_info < <(worker_process_get "${task_name}" "${worker}"); then
      continue
    fi

    local pid="${proc_info[0]:-}"
    local tmp_dir="${proc_info[1]:-}"
    local started_at="${proc_info[3]:-}"
    local timeout
    timeout="$(read_worker_timeout_seconds)"

    if [[ -n "${pid}" ]] && kill -0 "${pid}" > /dev/null 2>&1; then
      if [[ -n "${started_at}" && "${started_at}" =~ ^[0-9]+$ ]]; then
        local now
        now="$(date +%s)"
        local elapsed=$((now - started_at))
        if [[ "${elapsed}" -le "${timeout}" ]]; then
          continue
        fi
        log_task_warn "${task_name}" "worker ${worker} exceeded timeout (${elapsed}s)"
        kill -9 "${pid}" > /dev/null 2>&1 || true
      else
        continue
      fi
    fi

    log_task_warn "${task_name}" "worker ${worker} exited before pushing branch"

    cleanup_tmp_dir "${tmp_dir}"

    local retry_count
    retry_count="$(retry_count_get "${task_name}")"
    retry_count=$((retry_count + 1))
    retry_count_set "${task_name}" "${retry_count}"

    if [[ "${retry_count}" -ge 2 ]]; then
      local task_file
      if task_file="$(task_file_for_name "${task_name}")"; then
        annotate_blocked "${task_file}" "Worker exited before pushing branch twice; blocking task."
        move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
        git -C "${ROOT_DIR}" add "${STATE_DIR}"
        git -C "${ROOT_DIR}" commit -q -m "[governator] Block task ${task_name} on retry failure"
        git_push_default_branch
      fi
      in_flight_remove "${task_name}" "${worker}"
      worker_process_clear "${task_name}" "${worker}"
      return 0
    fi

    local task_file
    if task_file="$(task_file_for_name "${task_name}")"; then
      spawn_worker_for_task "${task_file}" "${worker}" "retry started for ${worker}"
    fi
  done < <(in_flight_entries)
}

# Merge a pushed special-worker branch into main (if present).
process_special_worker_branch() {
  local task_name="$1"
  local worker="$2"

  local remote
  remote="$(read_remote_name)"
  local branch="worker/${worker}/${task_name}"
  local remote_branch="${remote}/${branch}"

  git_fetch_remote
  if ! git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/remotes/${remote}/${branch}"; then
    log_task_warn "${task_name}" "special worker ${worker} did not push ${branch}"
    return 1
  fi

  process_worker_branch "${remote_branch}"
  return 0
}

# Process a single worker branch: review, move task, merge, cleanup.
process_worker_branch() {
  local remote_branch="$1"
  local remote
  remote="$(read_remote_name)"
  local local_branch="${remote_branch#"${remote}"/}"
  local worker_name="${local_branch#worker/}"
  worker_name="${worker_name%%/*}"

  git_fetch_remote
  git -C "${ROOT_DIR}" branch -f "${local_branch}" "${remote_branch}" > /dev/null 2>&1

  local task_name
  task_name="${local_branch##*/}"

  local task_dir
  if ! task_dir="$(task_dir_for_branch "${remote_branch}" "${task_name}")"; then
    # No task to annotate; record and drop the branch.
    log_warn "No task file found for ${task_name} on ${remote_branch}, skipping merge."
    printf '%s %s missing task file\n' "$(timestamp_utc_seconds)" "${local_branch}" >> "${FAILED_MERGES_LOG}"
    in_flight_remove "${task_name}" "${worker_name}"
    delete_worker_branch "${local_branch}"
    cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
    return 0
  fi

  local task_relpath="${STATE_DIR}/${task_dir}/${task_name}.md"
  local decision="block"
  local review_lines=()
  local block_reason=""

  local elapsed
  if elapsed="$(worker_elapsed_seconds "${task_name}" "${worker_name}" "${local_branch}")"; then
    log_task_event "${task_name}" "worker elapsed ${worker_name}: ${elapsed}s"
  fi

  case "${task_dir}" in
    task-worked)
      mapfile -t review_lines < <(code_review "${remote_branch}" "${local_branch}" "${task_relpath}")
      decision="${review_lines[0]:-block}"
      ;;
    task-assigned)
      if [[ "${worker_name}" == "reviewer" && "${task_name}" == 000-* ]]; then
        mapfile -t review_lines < <(code_review "${remote_branch}" "${local_branch}" "${task_relpath}")
        decision="${review_lines[0]:-block}"
      else
        block_reason="Unexpected task state ${task_dir} during processing."
      fi
      ;;
    task-feedback)
      :
      ;;
    *)
      block_reason="Unexpected task state ${task_dir} during processing."
      ;;
  esac

  git_checkout_default_branch

  local merged=0
  if git -C "${ROOT_DIR}" merge --ff-only -q "${local_branch}" > /dev/null; then
    merged=1
  else
    local base_branch
    base_branch="$(read_default_branch)"
    log_warn "Fast-forward merge failed for ${local_branch}; attempting rebase onto ${base_branch}."
    if git -C "${ROOT_DIR}" rebase -q "${base_branch}" "${local_branch}" > /dev/null 2>&1; then
      if git -C "${ROOT_DIR}" merge --ff-only -q "${local_branch}" > /dev/null; then
        merged=1
      fi
    else
      git -C "${ROOT_DIR}" rebase --abort > /dev/null 2>&1 || true
    fi

    if [[ "${merged}" -eq 0 ]]; then
      log_warn "Fast-forward still not possible; attempting merge commit for ${local_branch} into ${base_branch}."
      if git -C "${ROOT_DIR}" merge -q --no-ff "${local_branch}" -m "[governator] Merge task ${task_name}" > /dev/null; then
        merged=1
      else
        git -C "${ROOT_DIR}" merge --abort > /dev/null 2>&1 || true
      fi
    fi

    if [[ "${merged}" -eq 0 ]]; then
      log_error "Failed to merge ${local_branch} into ${base_branch} after rebase/merge attempts."
      log_warn "Pending commits for ${local_branch}: $(git -C "${ROOT_DIR}" log --oneline "${base_branch}..${local_branch}" | tr '\n' ' ' | sed 's/ $//')"

      local main_task_file
      if main_task_file="$(task_file_for_name "${task_name}")"; then
        # Keep the default branch task state authoritative; block and surface the failure.
        annotate_merge_failure "${main_task_file}" "${local_branch}"
        move_task_file "${main_task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
        git -C "${ROOT_DIR}" add "${STATE_DIR}" "${AUDIT_LOG}"
        git -C "${ROOT_DIR}" commit -q -m "[governator] Block task ${task_name} on merge failure"
        git_push_default_branch
      fi

      printf '%s %s\n' "$(timestamp_utc_seconds)" "${local_branch}" >> "${FAILED_MERGES_LOG}"
      in_flight_remove "${task_name}" "${worker_name}"
      cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
      # Preserve worker branch for debugging on merge failure.
      return 0
    fi
  fi

  if [[ "${merged}" -eq 1 ]]; then
    apply_review_decision "${task_name}" "${worker_name}" "${decision}" "${block_reason}" "${review_lines[@]:1}"
    git_push_default_branch
  fi

  in_flight_remove "${task_name}" "${worker_name}"

  delete_worker_branch "${local_branch}"
  cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
}

# Iterate all worker branches, skipping those logged as failed merges.
process_worker_branches() {
  touch_logs
  git_fetch_remote

  check_zombie_workers
  cleanup_stale_worker_dirs

  log_verbose "Scanning worker branches"
  local branch
  while IFS= read -r branch; do
    if [[ -z "${branch}" ]]; then
      continue
    fi
    log_verbose "Found worker branch: ${branch}"
    if is_failed_merge_branch "${branch}"; then
      log_verbose "Skipping failed merge branch: ${branch}"
      continue
    fi
    process_worker_branch "${branch}"
  done < <(list_worker_branches)
}

# Script entrypoint.
main() {
  ensure_clean_git
  ensure_dependencies
  ensure_db_dir
  require_governator_doc
  require_project_mode
  if handle_locked_state "run"; then
    return 0
  fi
  ensure_lock
  log_verbose "Run start (branch: $(read_default_branch))"
  sync_default_branch
  log_verbose "Sync complete"
  process_worker_branches
  resume_assigned_tasks
  assign_pending_tasks
  commit_audit_log_if_dirty
  git_checkout_default_branch
  log_verbose "Run complete (branch: $(read_default_branch))"
}

#############################################################################
# Internal subcommands (undocumented; intended for testing and ops drills)
#############################################################################
#
# These subcommands are not part of the public interface and may change without
# notice. They exist to make targeted testing and troubleshooting possible.
# Each subcommand still enforces the same safety checks (lock, clean git, deps)
# and operates on real state, so use with care.
#
# Usage:
#   governator.sh process-branches
#   governator.sh assign-backlog
#   governator.sh check-zombies
#   governator.sh cleanup-tmp [--dry-run]
#   governator.sh parse-review <file>
#   governator.sh list-workers
#   governator.sh extract-role <task-file>
#   governator.sh read-caps [role]
#   governator.sh count-in-flight [role]
#   governator.sh format-task-id <number>
#   governator.sh allocate-task-id
#   governator.sh normalize-tmp-path <path>
#   governator.sh audit-log <task> <message>
#
# Subcommand reference:
# - run:
#   Runs the normal full loop: lock, clean git, dependency check, ensure DB,
#   sync the default branch, process worker branches, then assign backlog tasks.
#
# - process-branches:
#   Processes only worker branches (including zombie detection and tmp cleanup).
#   This is useful to test review/merge behavior without assigning new work.
#
# - assign-backlog:
#   Assigns only backlog tasks. This is useful to validate filename parsing,
#   role caps, and in-flight handling without processing existing branches.
#
# - check-zombies:
#   Runs zombie detection logic against in-flight workers. If a worker's branch
#   is missing and the worker is dead or timed out, it retries once and blocks
#   on the second failure. Does not process branches or assign backlog.
#
# - cleanup-tmp:
#   Removes stale worker tmp directories in /tmp that are older than the worker
#   timeout and not referenced in the worker process log. Use --dry-run to list
#   candidates without removing them.
#
# - parse-review:
#   Prints the parsed review result and comments from a review.json file.
#
# - list-workers:
#   Prints the available worker roles, one per line.
#
# - extract-role:
#   Prints the role suffix extracted from a task filename (or exits non-zero).
#
# - read-caps:
#   Prints the global cap plus per-role caps. If a role is supplied, prints only
#   that role's cap.
#
# - count-in-flight:
#   Prints the total in-flight count. If a role is supplied, prints only that
#   role's in-flight count.
#
# - format-task-id:
#   Formats a numeric task id to zero-padded 3 digits.
#
# - allocate-task-id:
#   Reserves and prints the next task id (increments the stored counter).
#
# - normalize-tmp-path:
#   Normalizes /tmp paths to their /private/tmp equivalents.
#
# - audit-log:
#   Appends a line to the audit log with the provided task name and message.
#############################################################################
ensure_ready_with_lock() {
  ensure_clean_git
  ensure_lock
  ensure_dependencies
  ensure_db_dir
  require_governator_doc
}

ensure_ready_no_lock() {
  ensure_clean_git
  ensure_dependencies
  ensure_db_dir
  require_governator_doc
}

run_locked_action() {
  local context="$1"
  shift
  ensure_ready_with_lock
  if handle_locked_state "${context}"; then
    return 0
  fi
  "$@"
  commit_audit_log_if_dirty
}

parse_run_args() {
  local arg
  while [[ "$#" -gt 0 ]]; do
    arg="$1"
    case "${arg}" in
      -q | --quiet)
        GOV_QUIET=1
        ;;
      -v | --verbose)
        GOV_VERBOSE=1
        ;;
      --)
        shift
        break
        ;;
      *)
        log_error "Unknown option for run: ${arg}"
        exit 1
        ;;
    esac
    shift
  done
}

process_branches_action() {
  sync_default_branch
  process_worker_branches
}

assign_backlog_action() {
  sync_default_branch
  assign_pending_tasks
}

check_zombies_action() {
  sync_default_branch
  check_zombie_workers
}

print_help() {
  cat << 'EOF'
Usage: governator.sh <command>

Public commands:
  run      Run the normal full loop.
  init     Configure the project mode and defaults.
  update   Replace governator.sh with the latest upstream version.
  status   Show queue counts, in-flight workers, and blocked tasks.
  lock     Prevent new activity from starting and show a work snapshot.
  unlock   Resume activity after a lock.

Options:
  -h, --help   Show this help message.
  run -q, --quiet   Suppress stdout during run (errors still surface).
  run -v, --verbose  Print worker/reviewer command lines.

Note: You must run `governator.sh init` before using any other command.
EOF
}

dispatch_subcommand() {
  local cmd="${1:-}"
  if [[ -z "${cmd}" ]]; then
    print_help
    return 1
  fi
  case "${cmd}" in
    -h | --help)
      print_help
      return 0
      ;;
  esac

  if [[ "${cmd}" != "init" && "${cmd}" != "update" ]]; then
    if ! require_project_mode; then
      return 1
    fi
  fi
  shift || true

  case "${cmd}" in
    run)
      parse_run_args "$@"
      main
      ;;
    init)
      init_governator
      ;;
    update)
      update_governator
      ;;
    status)
      ensure_db_dir
      status_dashboard
      ;;
    lock)
      ensure_db_dir
      if system_locked; then
        local since
        if since="$(locked_since)"; then
          printf 'Governator already locked since %s\n' "${since}"
        else
          printf 'Governator already locked\n'
        fi
      else
        lock_governator
        printf 'Governator locked at %s\n' "$(locked_since)"
      fi
      printf 'Active work snapshot:\n'
      print_activity_snapshot
      ;;
    unlock)
      ensure_db_dir
      if system_locked; then
        unlock_governator
        printf 'Governator unlocked\n'
      else
        printf 'Governator already unlocked\n'
      fi
      ;;
    abort)
      ensure_ready_no_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: abort <task-prefix>"
        exit 1
      fi
      abort_task "${1}"
      ;;
    process-branches)
      run_locked_action "processing worker branches" process_branches_action
      ;;
    assign-backlog)
      run_locked_action "assigning backlog tasks" assign_backlog_action
      ;;
    check-zombies)
      run_locked_action "checking zombie workers" check_zombies_action
      ;;
    cleanup-tmp)
      ensure_ready_with_lock
      cleanup_stale_worker_dirs "${1:-}"
      ;;
    parse-review)
      ensure_ready_with_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: parse-review <file>"
        exit 1
      fi
      parse_review_json "${1}"
      ;;
    list-workers)
      ensure_ready_with_lock
      list_available_workers
      ;;
    extract-role)
      ensure_ready_with_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: extract-role <task-file>"
        exit 1
      fi
      if ! extract_worker_from_task "${1}"; then
        exit 1
      fi
      ;;
    read-caps)
      ensure_ready_with_lock
      if [[ -n "${1:-}" ]]; then
        read_worker_cap "${1}"
      else
        local global_cap
        global_cap="$(read_global_cap)"
        printf 'global %s\n' "${global_cap}"
        local role
        while IFS= read -r role; do
          printf '%s %s\n' "${role}" "$(read_worker_cap "${role}")"
        done < <(list_available_workers)
      fi
      ;;
    count-in-flight)
      ensure_ready_with_lock
      if [[ -n "${1:-}" ]]; then
        count_in_flight_role "${1}"
      else
        count_in_flight_total
      fi
      ;;
    format-task-id)
      ensure_ready_with_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: format-task-id <number>"
        exit 1
      fi
      format_task_id "${1}"
      ;;
    allocate-task-id)
      ensure_ready_with_lock
      allocate_task_id
      ;;
    normalize-tmp-path)
      ensure_ready_with_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: normalize-tmp-path <path>"
        exit 1
      fi
      normalize_tmp_path "${1}"
      ;;
    audit-log)
      ensure_ready_with_lock
      if [[ -z "${1:-}" || -z "${2:-}" ]]; then
        log_error "Usage: audit-log <task> <message>"
        exit 1
      fi
      local task_name="${1}"
      shift
      log_task_event "${task_name}" "$*"
      ;;
    *)
      log_error "Unknown subcommand: ${cmd}"
      exit 1
      ;;
  esac
}

dispatch_subcommand "$@"
