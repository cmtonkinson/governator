# shellcheck shell=bash

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
