# shellcheck shell=bash

# timestamp_utc_seconds
# Purpose: Produce a UTC timestamp with seconds precision.
# Args: None.
# Output: Prints timestamp to stdout.
# Returns: 0 always.
timestamp_utc_seconds() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

# timestamp_utc_minutes
# Purpose: Produce a UTC timestamp with minute precision.
# Args: None.
# Output: Prints timestamp to stdout.
# Returns: 0 always.
timestamp_utc_minutes() {
  date -u +"%Y-%m-%dT%H:%MZ"
}

# log_with_level
# Purpose: Format a log line with UTC timestamp and level.
# Args:
#   $1: Log level (string).
#   $2+: Message text (strings).
# Output: Prints formatted log line to stdout.
# Returns: 0 always.
log_with_level() {
  local level="$1"
  shift
  printf '[%s] %-5s %s\n' "$(timestamp_utc_seconds)" "${level}" "$*"
}

# log_info
# Purpose: Emit an INFO log line unless quiet mode is enabled.
# Args:
#   $@: Message text (strings).
# Output: Writes to stderr.
# Returns: 0 always.
log_info() {
  if [[ "${GOV_QUIET}" -eq 1 ]]; then
    return 0
  fi
  log_with_level "INFO" "$@" >&2
}

# log_verbose
# Purpose: Emit a verbose INFO log line when verbose mode is enabled.
# Args:
#   $@: Message text (strings).
# Output: Writes to stderr.
# Returns: 0 always.
log_verbose() {
  if [[ "${GOV_QUIET}" -eq 1 || "${GOV_VERBOSE}" -eq 0 ]]; then
    return 0
  fi
  log_with_level "INFO" "$@" >&2
}

# log_warn
# Purpose: Emit a WARN log line.
# Args:
#   $@: Message text (strings).
# Output: Writes to stderr.
# Returns: 0 always.
log_warn() {
  log_with_level "WARN" "$@" >&2
}

# log_error
# Purpose: Emit an ERROR log line.
# Args:
#   $@: Message text (strings).
# Output: Writes to stderr.
# Returns: 0 always.
log_error() {
  log_with_level "ERROR" "$@" >&2
}

# append_worker_log_separator
# Purpose: Add visual separators to a worker log file.
# Args:
#   $1: Log file path (string).
# Output: Writes separator lines to the log file.
# Returns: 0 on success.
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

# audit_log
# Purpose: Append a lifecycle event to the audit log.
# Args:
#   $1: Task name (string).
#   $2: Message (string).
# Output: Writes to the audit log file.
# Returns: 0 on success.
audit_log() {
  local task_name="$1"
  local message="$2"
  printf '%s %s -> %s\n' "$(timestamp_utc_minutes)" "${task_name}" "${message}" >> "${AUDIT_LOG}"
}

# log_task_event
# Purpose: Log a task event and append it to the audit log.
# Args:
#   $1: Task name (string).
#   $2+: Message text (strings).
# Output: Writes to stderr and audit log.
# Returns: 0 always.
log_task_event() {
  local task_name="$1"
  shift
  local message="$*"
  log_info "${task_name} -> ${message}"
  audit_log "${task_name}" "${message}"
}

# log_task_warn
# Purpose: Log a warning-level task event and append it to the audit log.
# Args:
#   $1: Task name (string).
#   $2+: Message text (strings).
# Output: Writes to stderr and audit log.
# Returns: 0 always.
log_task_warn() {
  local task_name="$1"
  shift
  local message="$*"
  log_warn "${task_name} -> ${message}"
  audit_log "${task_name}" "${message}"
}

# commit_audit_log_if_dirty
# Purpose: Commit and push the audit log if it has uncommitted changes.
# Args: None.
# Output: None.
# Returns: 0 on completion.
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
