# shellcheck shell=bash

# create_worktree
# Purpose: Create a git worktree for a task/worker combination with a new branch.
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
#   $3: Base ref to branch from (string, optional - defaults to default branch).
# Output: Logs verbose info.
# Returns: 0 on success, 1 on failure.
create_worktree() {
  local task_name="$1"
  local worker="$2"
  local base_ref="${3:-}"

  local worktree_path
  local branch_name
  worktree_path="$(worktree_path_for_task "${task_name}" "${worker}")"
  branch_name="$(worktree_branch_name "${task_name}" "${worker}")"

  if [[ -z "${base_ref}" ]]; then
    base_ref="$(read_default_branch)"
  fi

  ensure_worktrees_dir

  # Remove existing worktree if present (cleanup from previous failed run)
  if [[ -d "${worktree_path}" ]]; then
    git -C "${ROOT_DIR}" worktree remove --force "${worktree_path}" 2>/dev/null || true
  fi

  # Delete existing branch if present (cleanup from previous failed run)
  if git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/heads/${branch_name}"; then
    git -C "${ROOT_DIR}" branch -D "${branch_name}" 2>/dev/null || true
  fi

  # Create the worktree with a new branch
  if ! git -C "${ROOT_DIR}" worktree add -b "${branch_name}" "${worktree_path}" "${base_ref}" >/dev/null 2>&1; then
    log_error "Failed to create worktree at ${worktree_path}"
    return 1
  fi

  log_verbose "Created worktree at ${worktree_path} on branch ${branch_name}"
  return 0
}

# remove_worktree
# Purpose: Remove a git worktree and its associated branch.
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
# Output: Logs verbose info.
# Returns: 0 on success (or if already removed).
remove_worktree() {
  local task_name="$1"
  local worker="$2"

  local worktree_path
  local branch_name
  worktree_path="$(worktree_path_for_task "${task_name}" "${worker}")"
  branch_name="$(worktree_branch_name "${task_name}" "${worker}")"

  # Remove the worktree directory
  if [[ -d "${worktree_path}" ]]; then
    git -C "${ROOT_DIR}" worktree remove --force "${worktree_path}" 2>/dev/null || true
    log_verbose "Removed worktree at ${worktree_path}"
  fi

  # Delete the branch
  if git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/heads/${branch_name}"; then
    git -C "${ROOT_DIR}" branch -D "${branch_name}" 2>/dev/null || true
    log_verbose "Deleted branch ${branch_name}"
  fi

  # Prune stale worktree references
  git -C "${ROOT_DIR}" worktree prune 2>/dev/null || true

  return 0
}

# worktree_exists
# Purpose: Check if a worktree exists for a task/worker combination.
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
# Output: None.
# Returns: 0 if worktree exists, 1 otherwise.
worktree_exists() {
  local task_name="$1"
  local worker="$2"

  local worktree_path
  worktree_path="$(worktree_path_for_task "${task_name}" "${worker}")"

  [[ -d "${worktree_path}" ]]
}

# worktree_branch_exists
# Purpose: Check if a local worker branch exists.
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
# Output: None.
# Returns: 0 if branch exists, 1 otherwise.
worktree_branch_exists() {
  local task_name="$1"
  local worker="$2"

  local branch_name
  branch_name="$(worktree_branch_name "${task_name}" "${worker}")"

  git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/heads/${branch_name}"
}

# list_active_worktrees
# Purpose: List all worktree directories in the worktrees directory.
# Args: None.
# Output: Prints worktree paths to stdout, one per line.
# Returns: 0 always.
list_active_worktrees() {
  if [[ ! -d "${WORKTREES_DIR}" ]]; then
    return 0
  fi
  find "${WORKTREES_DIR}" -mindepth 1 -maxdepth 1 -type d 2>/dev/null || true
}

# list_worker_branches_local
# Purpose: List all local worker branches.
# Args: None.
# Output: Prints branch names to stdout, one per line.
# Returns: 0 always.
list_worker_branches_local() {
  git -C "${ROOT_DIR}" for-each-ref --format='%(refname:short)' "refs/heads/worker/*/*" 2>/dev/null || true
}

# prune_worktrees
# Purpose: Clean up stale worktree references in git.
# Args: None.
# Output: None.
# Returns: 0 always.
prune_worktrees() {
  git -C "${ROOT_DIR}" worktree prune 2>/dev/null || true
}

# has_new_commits
# Purpose: Check if a branch has new commits since a base ref.
# Args:
#   $1: Branch name (string).
#   $2: Base ref to compare against (string).
# Output: None.
# Returns: 0 if branch has commits beyond base_ref, 1 otherwise.
has_new_commits() {
  local branch_name="$1"
  local base_ref="$2"

  local count
  count="$(git -C "${ROOT_DIR}" rev-list --count "${base_ref}..${branch_name}" 2>/dev/null || echo "0")"

  [[ "${count}" -gt 0 ]]
}

# get_worktree_base_ref
# Purpose: Get the base ref (commit) that a worktree branch was created from.
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
# Output: Prints the base commit SHA to stdout.
# Returns: 0 on success, 1 if branch doesn't exist.
get_worktree_base_ref() {
  local task_name="$1"
  local worker="$2"

  local branch_name
  branch_name="$(worktree_branch_name "${task_name}" "${worker}")"

  # Get the merge-base with the default branch
  local default_branch
  default_branch="$(read_default_branch)"

  git -C "${ROOT_DIR}" merge-base "${default_branch}" "${branch_name}" 2>/dev/null
}

# worktree_has_completed
# Purpose: Check if a worker has completed (has commits on its branch).
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
# Output: None.
# Returns: 0 if worker has commits, 1 otherwise.
worktree_has_completed() {
  local task_name="$1"
  local worker="$2"

  local branch_name
  branch_name="$(worktree_branch_name "${task_name}" "${worker}")"

  if ! worktree_branch_exists "${task_name}" "${worker}"; then
    return 1
  fi

  local base_ref
  base_ref="$(get_worktree_base_ref "${task_name}" "${worker}")"

  if [[ -z "${base_ref}" ]]; then
    return 1
  fi

  has_new_commits "${branch_name}" "${base_ref}"
}

# worktree_path_from_branch
# Purpose: Extract the worktree path from a branch name.
# Args:
#   $1: Branch name in format worker/{role}/{task} (string).
# Output: Prints the worktree path to stdout.
# Returns: 0 always.
worktree_path_from_branch() {
  local branch_name="$1"

  # Extract role and task from branch name: worker/{role}/{task}
  local worker_part="${branch_name#worker/}"
  local worker="${worker_part%%/*}"
  local task_name="${worker_part#*/}"

  worktree_path_for_task "${task_name}" "${worker}"
}

# cleanup_worktree_by_branch
# Purpose: Remove a worktree and branch given only the branch name.
# Args:
#   $1: Branch name in format worker/{role}/{task} (string).
# Output: Logs verbose info.
# Returns: 0 on success.
cleanup_worktree_by_branch() {
  local branch_name="$1"

  # Extract role and task from branch name: worker/{role}/{task}
  local worker_part="${branch_name#worker/}"
  local worker="${worker_part%%/*}"
  local task_name="${worker_part#*/}"

  remove_worktree "${task_name}" "${worker}"
}

# cleanup_stale_worktrees
# Purpose: Remove stale worktrees not tracked as active.
# Args:
#   $1: Optional "--dry-run" to list candidates without deleting.
# Output: Prints stale worktrees when running in dry-run mode.
# Returns: 0 on completion.
cleanup_stale_worktrees() {
  local dry_run="${1:-}"
  local timeout
  timeout="$(read_worker_timeout_seconds)"
  local now
  now="$(date +%s)"

  if [[ ! -d "${WORKTREES_DIR}" ]]; then
    return 0
  fi

  # Build list of active worktrees from worker-processes.log
  local active_dirs=()
  if [[ -f "${WORKER_PROCESSES_LOG}" ]]; then
    while IFS=' | ' read -r task_name worker pid worktree_dir branch started_at; do
      if [[ -n "${worktree_dir}" ]]; then
        active_dirs+=("${worktree_dir}")
      fi
    done < "${WORKER_PROCESSES_LOG}"
  fi

  local dir
  while IFS= read -r dir; do
    if [[ -z "${dir}" ]]; then
      continue
    fi

    # Check if this worktree is in the active list
    local active=0
    local candidate
    for candidate in "${active_dirs[@]}"; do
      if [[ "${candidate}" == "${dir}" ]]; then
        active=1
        break
      fi
    done

    if [[ "${active}" -eq 1 ]]; then
      continue
    fi

    # Check age - only remove if older than timeout
    local mtime
    mtime="$(file_mtime_epoch "${dir}")"
    if [[ -n "${mtime}" && "${mtime}" =~ ^[0-9]+$ ]]; then
      local age=$((now - mtime))
      if [[ "${age}" -le "${timeout}" ]]; then
        continue
      fi
    fi

    if [[ "${dry_run}" == "--dry-run" ]]; then
      printf '%s\n' "${dir}"
    else
      # Remove the worktree
      git -C "${ROOT_DIR}" worktree remove --force "${dir}" 2>/dev/null || rm -rf "${dir}"
      log_verbose "Removed stale worktree: ${dir}"
    fi
  done < <(find "${WORKTREES_DIR}" -mindepth 1 -maxdepth 1 -type d 2>/dev/null)

  # Prune any stale worktree references
  prune_worktrees
}
