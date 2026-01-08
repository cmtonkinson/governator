# shellcheck shell=bash

# ensure_dependencies
# Purpose: Verify runtime toolchain dependencies for normal execution.
# Args: None.
# Output: Logs missing dependencies via log_error.
# Returns: 0 if all dependencies are present; exits 1 if any are missing.
ensure_dependencies() {
  local missing=()
  local dep
  for dep in awk date find git jq mktemp nohup stat; do
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
  ensure_sha256_tool
}

# ensure_update_dependencies
# Purpose: Verify toolchain dependencies required specifically for update workflows.
# Args: None.
# Output: Logs missing dependencies via log_error.
# Returns: 0 if all dependencies are present; exits 1 if any are missing.
ensure_update_dependencies() {
  if ! command -v curl > /dev/null 2>&1; then
    log_error "Missing dependency: curl"
    exit 1
  fi
  ensure_sha256_tool
  if ! command -v tar > /dev/null 2>&1; then
    log_error "Missing dependency: tar"
    exit 1
  fi
}

# ensure_sha256_tool
# Purpose: Verify a SHA-256 hashing tool is available.
# Args: None.
# Output: Logs missing dependency via log_error.
# Returns: 0 if available; exits 1 if missing.
ensure_sha256_tool() {
  if command -v shasum > /dev/null 2>&1; then
    return 0
  fi
  if command -v sha256sum > /dev/null 2>&1; then
    return 0
  fi
  if command -v openssl > /dev/null 2>&1; then
    return 0
  fi
  log_error "Missing dependency: sha256 tool (shasum, sha256sum, or openssl)"
  exit 1
}

# require_governator_doc
# Purpose: Ensure GOVERNATOR.md exists at the repository root.
# Args: None.
# Output: Logs error when missing.
# Returns: 0 if present; exits 1 if missing.
require_governator_doc() {
  if [[ ! -f "${ROOT_DIR}/GOVERNATOR.md" ]]; then
    log_error "GOVERNATOR.md not found at project root; aborting."
    exit 1
  fi
}

# ensure_ready_with_lock
# Purpose: Apply all safety checks for commands that require the lock.
# Args: None.
# Output: Errors are logged by called helpers.
# Returns: 0 if environment is ready; exits on failure.
ensure_ready_with_lock() {
  ensure_clean_git
  ensure_lock
  ensure_dependencies
  ensure_db_dir
  require_governator_doc
}

# ensure_ready_no_lock
# Purpose: Apply safety checks for commands that do not require a lock.
# Args: None.
# Output: Errors are logged by called helpers.
# Returns: 0 if environment is ready; exits on failure.
ensure_ready_no_lock() {
  ensure_clean_git
  ensure_dependencies
  ensure_db_dir
  require_governator_doc
}

# ensure_ready_read_only
# Purpose: Apply safety checks for read-only commands without enforcing clean git or locks.
# Args: None.
# Output: Errors are logged by called helpers.
# Returns: 0 if environment is ready; exits on failure.
ensure_ready_read_only() {
  ensure_dependencies
  ensure_db_dir
  require_governator_doc
}
