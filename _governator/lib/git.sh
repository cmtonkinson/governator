# shellcheck shell=bash

# ensure_clean_git
# Purpose: Exit when local git changes are detected (excluding allowed files).
# Args: None.
# Output: Logs warning on dirty working tree.
# Returns: 0 when clean; exits 0 when dirty.
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

# git_checkout_default_branch
# Purpose: Checkout the configured default branch.
# Args: None.
# Output: None.
# Returns: 0 on success.
git_checkout_default_branch() {
  local branch
  branch="$(read_default_branch)"
  git -C "${ROOT_DIR}" checkout "${branch}" > /dev/null 2>&1
}

# git_pull_default_branch
# Purpose: Pull the default branch from the configured remote.
# Args: None.
# Output: None.
# Returns: 0 on success.
git_pull_default_branch() {
  local branch
  local remote
  branch="$(read_default_branch)"
  remote="$(read_remote_name)"
  git -C "${ROOT_DIR}" pull -q "${remote}" "${branch}" > /dev/null
}

# git_push_default_branch
# Purpose: Push the default branch to the configured remote.
# Args: None.
# Output: None.
# Returns: 0 on success.
git_push_default_branch() {
  local branch
  local remote
  branch="$(read_default_branch)"
  remote="$(read_remote_name)"
  git -C "${ROOT_DIR}" push -q "${remote}" "${branch}" > /dev/null
}

# sync_default_branch
# Purpose: Checkout and pull the default branch from the remote.
# Args: None.
# Output: None.
# Returns: 0 on success.
sync_default_branch() {
  git_checkout_default_branch
  git_pull_default_branch
}

# git_fetch_remote
# Purpose: Fetch and prune remote refs.
# Args: None.
# Output: None.
# Returns: 0 on success.
git_fetch_remote() {
  local remote
  remote="$(read_remote_name)"
  git -C "${ROOT_DIR}" fetch -q "${remote}" --prune > /dev/null
}

# delete_worker_branch
# Purpose: Delete a worker branch locally and remotely (best effort).
# Args:
#   $1: Branch name (string).
# Output: Logs warnings if remote deletion fails.
# Returns: 0 on completion.
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
