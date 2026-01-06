# shellcheck shell=bash

# is_failed_merge_branch
# Purpose: Determine whether a worker branch is recorded as a failed merge.
# Args:
#   $1: Branch name (string).
# Output: None.
# Returns: 0 if branch is listed in FAILED_MERGES_LOG; 1 otherwise.
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

# worker_elapsed_seconds
# Purpose: Compute elapsed seconds between worker start and branch commit time.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Local branch name (string).
# Output: Prints elapsed seconds to stdout.
# Returns: 0 if elapsed time is computed; 1 if missing data or invalid timestamps.
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

# process_special_worker_branch
# Purpose: Handle a special worker branch and apply the same merge/review flow.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
# Output: Logs warnings and task events as needed.
# Returns: 0 on success; 1 when expected branch is missing.
process_special_worker_branch() {
  local task_name="$1"
  local worker="$2"

  local remote
  remote="$(read_remote_name)"
  local branch="worker/${worker}/${task_name}"
  local remote_branch="${remote}/${branch}"

  git_fetch_remote
  if ! git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/remotes/${remote}/${branch}"; then
    # Special non-reviewer workers must produce a branch; no branch means no reviewable output.
    log_task_warn "${task_name}" "special worker ${worker} did not push ${branch}"
    local task_file
    if task_file="$(task_file_for_name "${task_name}")"; then
      annotate_blocked "${task_file}" "Special worker completed without pushing a branch; no reviewable output."
      move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
      git -C "${ROOT_DIR}" add "${STATE_DIR}" "${AUDIT_LOG}"
      git -C "${ROOT_DIR}" commit -q -m "[governator] Block task ${task_name} on missing worker branch"
      git_push_default_branch
    fi
    return 1
  fi

  process_worker_branch "${remote_branch}"
  return 0
}

# process_worker_branch
# Purpose: Review and merge a worker branch, update task state, and clean up.
# Args:
#   $1: Remote branch ref (string).
# Output: Logs state transitions, merge issues, and audit events.
# Returns: 0 on completion; exits on fatal git errors.
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

# process_worker_branches
# Purpose: Process all remote worker branches while skipping known failed merges.
# Args: None.
# Output: Logs scanning and per-branch processing messages.
# Returns: 0 on completion.
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
