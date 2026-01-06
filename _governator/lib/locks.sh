# shellcheck shell=bash

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

lock_governator() {
  ensure_db_dir
  printf '%s\n' "$(timestamp_utc_seconds)" > "${SYSTEM_LOCK_FILE}"
}

unlock_governator() {
  ensure_db_dir
  rm -f "${SYSTEM_LOCK_FILE}"
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
