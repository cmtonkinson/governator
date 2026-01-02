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

NEXT_TICKET_FILE="${DB_DIR}/next_ticket_id"
GLOBAL_CAP_FILE="${DB_DIR}/global_worker_cap"
WORKER_CAPS_FILE="${DB_DIR}/worker_caps"
WORKER_TIMEOUT_FILE="${DB_DIR}/worker_timeout_seconds"

AUDIT_LOG="${DB_DIR}/audit.log"
WORKER_PROCESSES_LOG="${DB_DIR}/worker-processes.log"
RETRY_COUNTS_LOG="${DB_DIR}/retry-counts.log"

WORKER_ROLES_DIR="${STATE_DIR}/worker-roles"
TEMPLATES_DIR="${STATE_DIR}/templates"
LOCK_FILE="${STATE_DIR}/governator.lock"
FAILED_MERGES_LOG="${STATE_DIR}/failed-merges.log"
IN_FLIGHT_LOG="${STATE_DIR}/in-flight.log"
SYSTEM_LOCK_FILE="${DB_DIR}/governator.locked"
SYSTEM_LOCK_PATH="${SYSTEM_LOCK_FILE#"${ROOT_DIR}/"}"

CODEX_BIN="${CODEX_BIN:-codex}"
CODEX_WORKER_ARGS="${CODEX_WORKER_ARGS:---non-interactive}"
CODEX_REVIEW_ARGS="${CODEX_REVIEW_ARGS:---non-interactive}"

DEFAULT_GLOBAL_CAP=1
DEFAULT_WORKER_CAP=1
DEFAULT_TICKET_ID=1
DEFAULT_WORKER_TIMEOUT_SECONDS=900

PROJECT_NAME="$(basename "${ROOT_DIR}")"

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
  log_with_level "INFO" "$@"
}

log_warn() {
  log_with_level "WARN" "$@"
}

log_error() {
  log_with_level "ERROR" "$@"
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
    log_warn "Local git changes detected, exiting."
    exit 0
  fi
}

# Ensure required commands exist before running.
ensure_dependencies() {
  local missing=()
  local dep
  for dep in awk date find git mktemp nohup stat sgpt; do
    if ! command -v "${dep}" > /dev/null 2>&1; then
      missing+=("${dep}")
    fi
  done
  if ! command -v "${CODEX_BIN}" > /dev/null 2>&1; then
    missing+=("${CODEX_BIN}")
  fi
  if [[ "${#missing[@]}" -gt 0 ]]; then
    log_error "Missing dependencies: ${missing[*]}"
    exit 1
  fi
}
# Checkout main quietly.
git_checkout_main() {
  git -C "${ROOT_DIR}" checkout main > /dev/null 2>&1
}

# Pull main from origin.
git_pull_main() {
  git -C "${ROOT_DIR}" pull origin main
}

# Sync local main with origin.
sync_main() {
  git_checkout_main
  git_pull_main
}

# Fetch and prune remote refs.
git_fetch_origin() {
  git -C "${ROOT_DIR}" fetch origin --prune
}

# Delete a worker branch locally and on origin (best-effort).
delete_worker_branch() {
  local branch="$1"
  if [[ -z "${branch}" || "${branch}" == "main" || "${branch}" == "origin/main" ]]; then
    return 0
  fi
  git -C "${ROOT_DIR}" branch -D "${branch}" > /dev/null 2>&1 || true
  if ! git -C "${ROOT_DIR}" push origin --delete "${branch}" > /dev/null 2>&1; then
    log_warn "Failed to delete remote branch ${branch} with --delete"
  fi
  if ! git -C "${ROOT_DIR}" push origin :"refs/heads/${branch}" > /dev/null 2>&1; then
    log_warn "Failed to delete remote branch ${branch} with explicit refs/heads"
  fi
  git -C "${ROOT_DIR}" fetch origin --prune > /dev/null 2>&1 || true
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

# Read the global concurrency cap (defaults to 1).
read_global_cap() {
  read_numeric_file "${GLOBAL_CAP_FILE}" "${DEFAULT_GLOBAL_CAP}"
}

# Read the worker timeout in seconds (defaults to 900).
read_worker_timeout_seconds() {
  read_numeric_file "${WORKER_TIMEOUT_FILE}" "${DEFAULT_WORKER_TIMEOUT_SECONDS}"
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
  git_checkout_main
  if [[ "${task_file}" != "${blocked_dest}" ]]; then
    move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "aborted by operator"
  else
    audit_log "${task_name}" "aborted by operator"
  fi

  local aborted_at
  aborted_at="$(timestamp_utc_seconds)"
  local abort_meta
  abort_meta="Aborted by operator on ${aborted_at}
Worker: ${worker:-n/a}
PID: ${pid:-n/a}
Branch: ${branch:-n/a}"
  annotate_abort "${blocked_dest}" "${abort_meta}"
  annotate_blocked "${blocked_dest}" "Aborted by operator command."

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -m "Abort task ${task_name}"
  git -C "${ROOT_DIR}" push origin main
}

# Read the next ticket id from disk, defaulting to 1.
read_next_ticket_id() {
  ensure_db_dir
  if [[ ! -f "${NEXT_TICKET_FILE}" ]]; then
    printf '%s\n' "${DEFAULT_TICKET_ID}"
    return 0
  fi

  local value
  value="$(tr -d '[:space:]' < "${NEXT_TICKET_FILE}")"
  if [[ -z "${value}" ]]; then
    printf '%s\n' "${DEFAULT_TICKET_ID}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# Persist the next ticket id.
write_next_ticket_id() {
  local value="$1"
  ensure_db_dir
  printf '%s\n' "${value}" > "${NEXT_TICKET_FILE}"
}

# Format a numeric ticket id as zero-padded 3 digits.
format_ticket_id() {
  local value="$1"
  printf '%03d' "${value}"
}

# Allocate the next ticket id and increment the stored value.
allocate_ticket_id() {
  local current
  current="$(read_next_ticket_id)"
  if ! [[ "${current}" =~ ^[0-9]+$ ]]; then
    log_warn "Invalid ticket id value '${current}', resetting to 1."
    current=1
  fi

  local next=$((current + 1))
  write_next_ticket_id "${next}"
  printf '%s\n' "${current}"
}

# Create a new ticket file using the template and allocated id.
create_ticket_file() {
  local short_name="$1"
  local role="$2"
  local target_dir="$3"

  local ticket_id
  ticket_id="$(allocate_ticket_id)"

  local id_segment
  id_segment="$(format_ticket_id "${ticket_id}")"
  local filename="${id_segment}-${short_name}-${role}.md"

  local template="${TEMPLATES_DIR}/ticket.md"
  if [[ ! -f "${template}" ]]; then
    log_error "Missing ticket template at ${template}."
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
  local body="$3"
  {
    printf '\n%s\n\n' "${title}"
    printf '%s\n' "${body}"
  } >> "${file}"
}

# Append a lifecycle event to the audit log.
audit_log() {
  local task_name="$1"
  local message="$2"
  printf '%s %s -> %s\n' "$(timestamp_utc_minutes)" "${task_name}" "${message}" >> "${AUDIT_LOG}"
}

# Move a task file to a new queue and record an audit entry.
move_task_file() {
  local task_file="$1"
  local dest_dir="$2"
  local task_name="$3"
  local audit_message="$4"
  mv "${task_file}" "${dest_dir}/$(basename "${task_file}")"
  audit_log "${task_name}" "${audit_message}"
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
  if [[ -n "${CODEX_WORKER_CMD:-}" ]]; then
    (
      cd "${dir}"
      GOV_PROMPT="${prompt}" nohup bash -c "${CODEX_WORKER_CMD}" >> "${log_file}" 2>&1 &
      echo $!
    )
    return 0
  fi

  # Use nohup to prevent worker exit from being tied to this process.
  local args=()
  read -r -a args <<< "${CODEX_WORKER_ARGS}"
  (
    cd "${dir}"
    nohup "${CODEX_BIN}" exec "${args[@]}" --message "${prompt}" >> "${log_file}" 2>&1 &
    echo $!
  )
}

# Run the reviewer synchronously so a review.json is produced.
run_codex_reviewer() {
  local dir="$1"
  local prompt="$2"
  if [[ -n "${CODEX_REVIEW_CMD:-}" ]]; then
    (cd "${dir}" && GOV_PROMPT="${prompt}" bash -c "${CODEX_REVIEW_CMD}")
    return 0
  fi

  local args=()
  read -r -a args <<< "${CODEX_REVIEW_ARGS}"
  (cd "${dir}" && "${CODEX_BIN}" exec "${args[@]}" --message "${prompt}")
}

# List remote worker branches.
list_worker_branches() {
  git -C "${ROOT_DIR}" for-each-ref --format='%(refname:short)' refs/remotes/origin/worker/*/* || true
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

# Block a task when required metadata is missing or invalid.
block_task_from_backlog() {
  local task_file="$1"
  local reason="$2"

  git_checkout_main

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local blocked_file="${STATE_DIR}/task-blocked/${task_name}.md"
  move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
  annotate_blocked "${blocked_file}" "${reason}"

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -m "Block task ${task_name}"
  git -C "${ROOT_DIR}" push origin main
}

# Record assignment details in the task file.
annotate_assignment() {
  local task_file="$1"
  local worker="$2"
  append_section "${task_file}" "## Assignment" "Assigned to ${worker} by Governator on $(timestamp_utc_seconds)."
}

# Record review decision and comments in the task file.
annotate_review() {
  local task_file="$1"
  local decision="$2"
  local comments=("$@")
  comments=("${comments[@]:2}")

  append_section "${task_file}" "## Review Result" "Decision: ${decision}"
  if [[ "${#comments[@]}" -gt 0 ]]; then
    {
      printf '\nComments:\n'
      for comment in "${comments[@]}"; do
        printf -- '- %s\n' "${comment}"
      done
    } >> "${task_file}"
  fi
}

# Add feedback to a task file before reassigning.
annotate_feedback() {
  local task_file="$1"
  append_section "${task_file}" "## Feedback" "Moved back to task-assigned for follow-up on $(timestamp_utc_seconds)."
}

# Capture a blocking reason in the task file.
annotate_blocked() {
  local task_file="$1"
  local reason="$2"
  append_section "${task_file}" "## Governator Block" "${reason}"
}

annotate_abort() {
  local task_file="$1"
  local abort_metadata="$2"
  append_section "${task_file}" "## Abort" "${abort_metadata}"
}

# Record a merge failure for reviewer visibility.
annotate_merge_failure() {
  local task_file="$1"
  local branch="$2"
  append_section "${task_file}" "## Merge Failure" "Unable to fast-forward merge ${branch} into main on $(timestamp_utc_seconds)."
}

# Parse review.json for decision and comments.
parse_review_json() {
  local file="$1"
  if [[ ! -f "${file}" ]]; then
    printf 'block\nReview file missing at %s\n' "${file}"
    return 0
  fi

  # Use Python for strict JSON parsing; shell parsing is error-prone.
  if command -v python3 > /dev/null 2>&1; then
    if ! python3 - "${file}" << 'PY'; then
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)

result = str(data.get("result", "")).strip()
comments = data.get("comments") or []
if not isinstance(comments, list):
    comments = [str(comments)]

print(result)
for comment in comments:
    print(comment)
PY
      printf 'block\nFailed to parse review.json\n'
    fi
    return 0
  fi

  printf 'block\nPython3 unavailable to parse review.json\n'
}

# Run reviewer flow in a clean clone and return parsed review output.
code_review() {
  local remote_branch="$1"
  local local_branch="$2"
  local task_relpath="$3"

  local tmp_dir
  tmp_dir="$(mktemp -d "/tmp/governator-${PROJECT_NAME}-reviewer-${local_branch//\//-}-XXXXXX")"

  git clone "$(git -C "${ROOT_DIR}" remote get-url origin)" "${tmp_dir}" > /dev/null 2>&1
  git -C "${tmp_dir}" fetch origin > /dev/null 2>&1
  git -C "${tmp_dir}" checkout -B "${local_branch}" "${remote_branch}" > /dev/null 2>&1

  # Seed with a template to guide reviewers toward the expected schema.
  if [[ -f "${TEMPLATES_DIR}/review.json" ]]; then
    cp "${TEMPLATES_DIR}/review.json" "${tmp_dir}/review.json"
  fi

  local prompt
  prompt="Read and follow the instructions in the following files, in this order: _governator/worker-contract.md, _governator/special-roles/reviewer.md, _governator/custom-prompts/_global.md, _governator/custom-prompts/reviewer.md, ${task_relpath}."

  if ! run_codex_reviewer "${tmp_dir}" "${prompt}"; then
    log_warn "Reviewer command failed for ${local_branch}."
  fi

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

  git_checkout_main

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local assigned_file="${STATE_DIR}/task-assigned/${task_name}.md"
  mv "${task_file}" "${assigned_file}"
  annotate_assignment "${assigned_file}" "${worker}"
  audit_log "${task_name}" "assigned to ${worker}"

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -m "Assign task ${task_name}"
  git -C "${ROOT_DIR}" push origin main

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

    assign_task "${task_file}" "${worker}"
    in_flight_add "${task_name}" "${worker}"
  done < <(list_task_files_in_dir "${STATE_DIR}/task-backlog")
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

  git clone "$(git -C "${ROOT_DIR}" remote get-url origin)" "${tmp_dir}" > /dev/null 2>&1
  git -C "${tmp_dir}" checkout -b "worker/${worker}/${task_name}" origin/main > /dev/null 2>&1

  local task_relpath="${task_file#"${ROOT_DIR}/"}"
  local prompt
  prompt="Read and follow the instructions in the following files, in this order: _governator/worker-contract.md, _governator/worker-roles/${worker}.md, _governator/custom-prompts/_global.md, _governator/custom-prompts/${worker}.md, ${task_relpath}."

  local branch_name="worker/${worker}/${task_name}"
  local pid
  local started_at
  started_at="$(date +%s)"
  pid="$(run_codex_worker_detached "${tmp_dir}" "${prompt}" "${log_file}")"
  if [[ -n "${pid}" ]]; then
    worker_process_set "${task_name}" "${worker}" "${pid}" "${tmp_dir}" "${branch_name}" "${started_at}"
    if [[ -n "${audit_message}" ]]; then
      audit_log "${task_name}" "${audit_message}"
    fi
    log_info "Worker started for ${task_name} on ${worker} in ${tmp_dir}"
  else
    log_warn "Failed to capture worker pid for ${task_name}."
  fi
}

# Handle missing branches with dead workers.
check_zombie_workers() {
  touch_logs

  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 0
  fi

  local line
  while IFS= read -r line; do
    if [[ -z "${line}" ]]; then
      continue
    fi
    if [[ "${line}" != *" -> "* ]]; then
      continue
    fi

    local task_name="${line%% -> *}"
    local worker="${line##* -> }"
    local branch="worker/${worker}/${task_name}"

    if git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/remotes/origin/${branch}"; then
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
        audit_log "${task_name}" "worker ${worker} exceeded timeout (${elapsed}s)"
        kill -9 "${pid}" > /dev/null 2>&1 || true
      else
        continue
      fi
    fi

    audit_log "${task_name}" "worker ${worker} exited before pushing branch"

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
        git -C "${ROOT_DIR}" commit -m "Block task ${task_name} on retry failure"
        git -C "${ROOT_DIR}" push origin main
      fi
      in_flight_remove "${task_name}" "${worker}"
      worker_process_clear "${task_name}" "${worker}"
      return 0
    fi

    local task_file
    if task_file="$(task_file_for_name "${task_name}")"; then
      spawn_worker_for_task "${task_file}" "${worker}" "retry started for ${worker}"
    fi
  done < "${IN_FLIGHT_LOG}"
}

# Process a single worker branch: review, move task, merge, cleanup.
process_worker_branch() {
  local remote_branch="$1"
  local local_branch="${remote_branch#origin/}"
  local worker_name="${local_branch#worker/}"
  worker_name="${worker_name%%/*}"

  git_fetch_origin
  git -C "${ROOT_DIR}" checkout -B "${local_branch}" "${remote_branch}" > /dev/null 2>&1

  local task_name
  task_name="${local_branch##*/}"

  local task_file
  if ! task_file="$(task_file_for_name "${task_name}")"; then
    # No task to annotate; record and drop the branch.
    log_warn "No task file found for ${task_name}, skipping merge."
    printf '%s %s missing task file\n' "$(timestamp_utc_seconds)" "${local_branch}" >> "${FAILED_MERGES_LOG}"
    in_flight_remove "${task_name}" "${worker_name}"
    delete_worker_branch "${local_branch}"
    cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
    return 0
  fi

  local task_dir
  task_dir="$(basename "$(dirname "${task_file}")")"

  case "${task_dir}" in
    task-worked)
      local review_lines=()
      mapfile -t review_lines < <(code_review "${remote_branch}" "${local_branch}" "${task_file#"${ROOT_DIR}/"}")
      local decision="${review_lines[0]:-block}"
      annotate_review "${task_file}" "${decision}" "${review_lines[@]:1}"
      audit_log "${task_name}" "moved to task-worked"

      case "${decision}" in
        approve)
          move_task_file "${task_file}" "${STATE_DIR}/task-done" "${task_name}" "moved to task-done"
          ;;
        reject)
          move_task_file "${task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "moved to task-assigned"
          ;;
        *)
          move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
          ;;
      esac
      ;;
    task-feedback)
      annotate_feedback "${task_file}"
      move_task_file "${task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "moved to task-assigned"
      ;;
    *)
      log_warn "Unexpected task state ${task_dir} for ${task_name}, blocking."
      annotate_blocked "${task_file}" "Unexpected task state ${task_dir} during processing."
      move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
      ;;
  esac

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -m "Process task ${task_name}"

  git_checkout_main

  if git -C "${ROOT_DIR}" merge --ff-only "${local_branch}"; then
    git -C "${ROOT_DIR}" push origin main
  else
    log_error "Failed to fast-forward merge ${local_branch} into main."

    local main_task_file
    if main_task_file="$(task_file_for_name "${task_name}")"; then
      # Keep main's task state authoritative; block and surface the failure.
      annotate_merge_failure "${main_task_file}" "${local_branch}"
      move_task_file "${main_task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
      git -C "${ROOT_DIR}" add "${STATE_DIR}"
      git -C "${ROOT_DIR}" commit -m "Block task ${task_name} on merge failure"
      git -C "${ROOT_DIR}" push origin main
    fi

    printf '%s %s\n' "$(timestamp_utc_seconds)" "${local_branch}" >> "${FAILED_MERGES_LOG}"
  fi

  in_flight_remove "${task_name}" "${worker_name}"

  delete_worker_branch "${local_branch}"
  cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
}

# Iterate all worker branches, skipping those logged as failed merges.
process_worker_branches() {
  touch_logs
  git_fetch_origin

  check_zombie_workers
  cleanup_stale_worker_dirs

  local branch
  while IFS= read -r branch; do
    if [[ -z "${branch}" ]]; then
      continue
    fi
    if is_failed_merge_branch "${branch}"; then
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
  if handle_locked_state "run"; then
    return 0
  fi
  ensure_lock
  sync_main
  process_worker_branches
  assign_pending_tasks
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
#   governator.sh format-ticket-id <number>
#   governator.sh allocate-ticket-id
#   governator.sh normalize-tmp-path <path>
#   governator.sh audit-log <task> <message>
#
# Subcommand reference:
# - run:
#   Runs the normal full loop: lock, clean git, dependency check, ensure DB,
#   sync main, process worker branches, then assign backlog tasks.
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
# - format-ticket-id:
#   Formats a numeric ticket id to zero-padded 3 digits.
#
# - allocate-ticket-id:
#   Reserves and prints the next ticket id (increments the stored counter).
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
}

ensure_ready_no_lock() {
  ensure_clean_git
  ensure_dependencies
  ensure_db_dir
}

run_locked_action() {
  local context="$1"
  shift
  ensure_ready_with_lock
  if handle_locked_state "${context}"; then
    return 0
  fi
  "$@"
}

process_branches_action() {
  sync_main
  process_worker_branches
}

assign_backlog_action() {
  sync_main
  assign_pending_tasks
}

check_zombies_action() {
  sync_main
  check_zombie_workers
}

print_help() {
  cat <<'EOF'
Usage: governator.sh [command]

Public commands:
  run      Run the normal full loop (default).
  status   Show queue counts, in-flight workers, and blocked tasks.
  lock     Prevent new activity from starting and show a work snapshot.
  unlock   Resume activity after a lock.

Options:
  -h, --help   Show this help message.
EOF
}

dispatch_subcommand() {
  local cmd="${1:-run}"
  case "${cmd}" in
    -h|--help)
      print_help
      return 0
      ;;
  esac
  shift || true

  case "${cmd}" in
    run)
      main
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
    format-ticket-id)
      ensure_ready_with_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: format-ticket-id <number>"
        exit 1
      fi
      format_ticket_id "${1}"
      ;;
    allocate-ticket-id)
      ensure_ready_with_lock
      allocate_ticket_id
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
      audit_log "${task_name}" "$*"
      ;;
    *)
      log_error "Unknown subcommand: ${cmd}"
      exit 1
      ;;
  esac
}

dispatch_subcommand "$@"
