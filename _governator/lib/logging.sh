# shellcheck shell=bash

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
