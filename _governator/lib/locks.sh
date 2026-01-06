# shellcheck shell=bash

# cleanup_lock
# Purpose: Remove the transient run lock file during process exit.
# Args: None.
# Output: None.
# Returns: 0 on completion.
cleanup_lock() {
  if [[ -f "${LOCK_FILE}" ]]; then
    rm -f "${LOCK_FILE}"
  fi
}

# ensure_lock
# Purpose: Guard against concurrent governator runs by creating a lock file.
# Args: None.
# Output: Logs a warning if a lock is already present.
# Returns: 0 after creating the lock; exits 0 when a lock already exists.
ensure_lock() {
  if [[ -f "${LOCK_FILE}" ]]; then
    log_warn "Lock file exists at ${LOCK_FILE}, exiting."
    exit 0
  fi
  printf '%s\n' "$(timestamp_utc_seconds)" > "${LOCK_FILE}"
  trap cleanup_lock EXIT
}

# lock_governator
# Purpose: Persist a system-wide lock that blocks new activity.
# Args: None.
# Output: Writes a UTC timestamp to the system lock file.
# Returns: 0 on success.
lock_governator() {
  ensure_db_dir
  printf '%s\n' "$(timestamp_utc_seconds)" > "${SYSTEM_LOCK_FILE}"
}

# unlock_governator
# Purpose: Remove the system-wide lock file.
# Args: None.
# Output: None.
# Returns: 0 on success.
unlock_governator() {
  ensure_db_dir
  rm -f "${SYSTEM_LOCK_FILE}"
}

# system_locked
# Purpose: Check whether the system-wide lock file exists.
# Args: None.
# Output: None.
# Returns: 0 if locked; 1 if unlocked.
system_locked() {
  [[ -f "${SYSTEM_LOCK_FILE}" ]]
}

# locked_since
# Purpose: Read the timestamp for when the system lock was created.
# Args: None.
# Output: Writes the lock timestamp to stdout if present.
# Returns: 0 if locked; 1 if unlocked.
locked_since() {
  if [[ -f "${SYSTEM_LOCK_FILE}" ]]; then
    cat "${SYSTEM_LOCK_FILE}"
    return 0
  fi
  return 1
}
