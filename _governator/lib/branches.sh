# shellcheck shell=bash

# is_failed_merge_branch
# Purpose: Determine whether a worker branch is recorded as a failed merge.
# Args:
#   $1: Branch name (string).
# Output: None.
# Returns: 0 if branch+head SHA is listed in FAILED_MERGES_LOG; 1 otherwise.
is_failed_merge_branch() {
  local branch="$1"
  if [[ ! -f "${FAILED_MERGES_LOG}" ]]; then
    return 1
  fi
  local head_sha
  head_sha="$(git -C "${ROOT_DIR}" rev-parse "${branch}" 2> /dev/null || true)"
  if [[ -z "${head_sha}" ]]; then
    return 1
  fi
  if awk -v branch="${branch}" -v head_sha="${head_sha}" '
    NF >= 3 {
      if ($2 == branch && $3 == head_sha) { found=1 }
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

# ensure_worker_branch_present
# Purpose: Ensure the local worker branch exists.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Branch name (string).
# Output: Logs missing branch warnings and cleanup.
# Returns: 0 if branch exists; 1 if missing.
ensure_worker_branch_present() {
  local task_name="$1"
  local worker_name="$2"
  local branch="$3"

  if ! git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/heads/${branch}"; then
    log_warn "Worker branch missing at ${branch}, skipping."
    in_flight_remove "${task_name}" "${worker_name}"
    remove_worktree "${task_name}" "${worker_name}"
    worker_process_clear "${task_name}" "${worker_name}"
    return 1
  fi
  return 0
}

# resolve_task_dir_or_cleanup
# Purpose: Resolve the task directory for a worker branch or clean up when missing.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Branch name (string).
# Output: Prints task directory on success; logs missing task cleanup.
# Returns: 0 if task dir found; 1 if missing.
resolve_task_dir_or_cleanup() {
  local task_name="$1"
  local worker_name="$2"
  local branch="$3"

  local task_dir
  if ! task_dir="$(task_dir_for_branch "${branch}" "${task_name}")"; then
    # No task to annotate; record and drop the branch.
    log_warn "No task file found for ${task_name} on ${branch}, skipping merge."
    local missing_sha
    missing_sha="$(git -C "${ROOT_DIR}" rev-parse "${branch}" 2> /dev/null || true)"
    printf '%s %s %s missing task file\n' "$(timestamp_utc_seconds)" "${branch}" "${missing_sha:-unknown}" >> "${FAILED_MERGES_LOG}"
    in_flight_remove "${task_name}" "${worker_name}"
    delete_worker_branch "${branch}"
    remove_worktree "${task_name}" "${worker_name}"
    return 1
  fi
  printf '%s\n' "${task_dir}"
}

# log_worker_elapsed_if_known
# Purpose: Log worker elapsed time when metadata exists.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Local branch name (string).
# Output: Logs elapsed time when available.
# Returns: 0 always.
log_worker_elapsed_if_known() {
  local task_name="$1"
  local worker_name="$2"
  local local_branch="$3"

  local elapsed
  if elapsed="$(worker_elapsed_seconds "${task_name}" "${worker_name}" "${local_branch}")"; then
    log_task_event "${task_name}" "worker elapsed ${worker_name}: ${elapsed}s"
  fi
}

# handle_non_reviewer_branch_state
# Purpose: Handle non-reviewer branches for worked or blocked tasks.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Task dir name (string).
#   $4: Branch name (string).
#   $5: Task file path (string).
# Output: Logs and spawns reviewers as needed.
# Returns: 0 if handled and should stop; 1 if processing should continue.
handle_non_reviewer_branch_state() {
  local task_name="$1"
  local worker_name="$2"
  local task_dir="$3"
  local branch="$4"
  local task_relpath="$5"

  if [[ "${task_dir}" == "task-worked" ]]; then
    in_flight_remove "${task_name}" "${worker_name}"
    # Only remove the worktree directory, keep the branch for the reviewer to branch from
    remove_worktree_dir "${task_name}" "${worker_name}"

    local review_branch="worker/reviewer/${task_name}"
    if in_flight_has_task_worker "${task_name}" "reviewer"; then
      log_verbose "Reviewer already in-flight for ${task_name}; skipping spawn"
      return 0
    fi
    # Check if reviewer branch already exists (reviewer already completed)
    if ! git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/heads/${review_branch}"; then
      local cap_note
      if ! cap_note="$(can_assign_task "reviewer" "${task_name}")"; then
        log_warn "${cap_note}"
      else
        in_flight_add "${task_name}" "reviewer"
        # Reviewer branches from the worker's local branch
        spawn_worker_for_task "${task_relpath}" "reviewer" "starting review for ${task_name}" "${branch}"
      fi
    fi
    return 0
  fi

  if [[ "${task_dir}" == "task-blocked" ]]; then
    log_warn "Worker marked ${task_name} blocked; skipping merge."
    in_flight_remove "${task_name}" "${worker_name}"
    if task_block_reason_requests_preserved_worktree "${task_relpath}"; then
      log_verbose "Preserving worktree for ${task_name} based on block reason."
    else
      delete_worker_branch "${branch}"
      remove_worktree "${task_name}" "${worker_name}"
    fi
    ensure_unblock_planner_task || true
    return 0
  fi

  return 1
}

# task_block_reason_requests_preserved_worktree
# Purpose: Check if the blocked task reason requests preserving the worktree.
# Args:
#   $1: Task file path (string).
# Output: None.
# Returns: 0 if the reason indicates preservation; 1 otherwise.
task_block_reason_requests_preserved_worktree() {
  local task_file="$1"
  if [[ -z "${task_file}" || ! -f "${task_file}" ]]; then
    return 1
  fi
  local reason
  reason="$(extract_block_reason "${task_file}")"
  if [[ "${reason}" == *"Worktree preserved at "* ]]; then
    return 0
  fi
  return 1
}

# validate_task_state_for_processing
# Purpose: Block unexpected task state transitions during processing.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Task dir name (string).
#   $4: Decision variable name (string).
#   $5: Block reason variable name (string).
# Output: None.
# Returns: 0 always.
validate_task_state_for_processing() {
  local task_name="$1"
  local worker_name="$2"
  local task_dir="$3"
  local decision_var="$4"
  local reason_var="$5"

  local decision="${!decision_var}"
  local reason="${!reason_var}"

  case "${task_dir}" in
    task-worked)
      :
      ;;
    task-assigned)
      if [[ "${worker_name}" == "reviewer" && "${task_name}" == 000-* ]]; then
        :
      else
        decision="block"
        reason="Unexpected task state ${task_dir} during processing."
      fi
      ;;
    *)
      decision="block"
      reason="Unexpected task state ${task_dir} during processing."
      ;;
  esac

  printf -v "${decision_var}" '%s' "${decision}"
  printf -v "${reason_var}" '%s' "${reason}"
}

# cleanup_worker_artifacts
# Purpose: Remove in-flight entries, branches, and worktrees for workers.
# Args:
#   $1: Task name (string).
#   $2: Primary worker name (string).
#   $3: Merge worker name (string).
# Output: None.
# Returns: 0 always.
cleanup_worker_artifacts() {
  local task_name="$1"
  local worker_name="$2"
  local merge_worker="$3"

  in_flight_remove "${task_name}" "${worker_name}"
  if [[ "${merge_worker}" != "${worker_name}" ]]; then
    in_flight_remove "${task_name}" "${merge_worker}"
  fi

  remove_worktree "${task_name}" "${worker_name}"
  if [[ "${merge_worker}" != "${worker_name}" ]]; then
    remove_worktree "${task_name}" "${merge_worker}"
  fi
}

# finalize_review_and_cleanup
# Purpose: Apply review decision, push, and cleanup worker artifacts.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Decision (string).
#   $4: Block reason (string).
#   $5: Merge worker name (string).
#   $@: Review comment lines.
# Output: None.
# Returns: 0 always.
finalize_review_and_cleanup() {
  local task_name="$1"
  local worker_name="$2"
  local decision="$3"
  local block_reason="$4"
  local merge_worker="$5"
  shift 5
  local review_lines=("$@")

  apply_review_decision "${task_name}" "${worker_name}" "${decision}" "${block_reason}" "${review_lines[@]}"
  git_push_default_branch
  cleanup_worker_artifacts "${task_name}" "${worker_name}" "${merge_worker}"
}

# merge_branch_into_default
# Purpose: Merge a worker branch into the default branch with fallbacks.
# Args:
#   $1: Task name (string).
#   $2: Merge branch name (string).
# Output: Logs merge attempts and failures.
# Returns: 0 on merge success; 1 on failure.
merge_branch_into_default() {
  local task_name="$1"
  local merge_branch="$2"
  local merged=0

  if git -C "${ROOT_DIR}" merge --ff-only -q "${merge_branch}" > /dev/null 2>&1; then
    merged=1
  else
    local base_branch
    base_branch="$(read_default_branch)"
    log_warn "Fast-forward merge failed for ${merge_branch}; attempting rebase onto ${base_branch}."
    if git -C "${ROOT_DIR}" rebase -q "${base_branch}" "${merge_branch}" > /dev/null 2>&1; then
      if git -C "${ROOT_DIR}" merge --ff-only -q "${merge_branch}" > /dev/null 2>&1; then
        merged=1
      fi
    else
      git -C "${ROOT_DIR}" rebase --abort > /dev/null 2>&1 || true
    fi

    if [[ "${merged}" -eq 0 ]]; then
      log_warn "Fast-forward still not possible; attempting merge commit for ${merge_branch} into ${base_branch}."
      if git -C "${ROOT_DIR}" merge -q --no-ff "${merge_branch}" -m "[governator] Merge task ${task_name}" > /dev/null 2>&1; then
        merged=1
      else
        git -C "${ROOT_DIR}" merge --abort > /dev/null 2>&1 || true
      fi
    fi

    if [[ "${merged}" -eq 0 ]]; then
      log_error "Failed to merge ${merge_branch} into ${base_branch} after rebase/merge attempts."
      log_warn "Pending commits for ${merge_branch}: $(git -C "${ROOT_DIR}" log --oneline "${base_branch}..${merge_branch}" | tr '\n' ' ' | sed 's/ $//')"
    fi
  fi

  if [[ "${merged}" -eq 1 ]]; then
    return 0
  fi
  return 1
}

# handle_merge_failure
# Purpose: Requeue a task and clean up after merge failure.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Merge worker name (string).
#   $4: Merge branch name (string).
# Output: Logs failures, updates tasks, and cleans up.
# Returns: 0 always.
handle_merge_failure() {
  local task_name="$1"
  local worker_name="$2"
  local merge_worker="$3"
  local merge_branch="$4"

  local main_task_file
  if main_task_file="$(task_file_for_name "${task_name}")"; then
    # Keep the default branch task state authoritative; requeue and surface the failure.
    annotate_merge_failure "${main_task_file}" "${merge_branch}"
    move_task_file "${main_task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "moved to task-assigned"
    git -C "${ROOT_DIR}" add "${STATE_DIR}" "${AUDIT_LOG}"
    git -C "${ROOT_DIR}" commit -q -m "[governator] Requeue task ${task_name} after merge failure"
    git_push_default_branch
  fi

  local failed_sha
  failed_sha="$(git -C "${ROOT_DIR}" rev-parse "${merge_branch}" 2> /dev/null || true)"
  printf '%s %s %s\n' "$(timestamp_utc_seconds)" "${merge_branch}" "${failed_sha:-unknown}" >> "${FAILED_MERGES_LOG}"
  cleanup_worker_artifacts "${task_name}" "${worker_name}" "${merge_worker}"
}

# process_worker_branch
# Purpose: Handle review and merge decisions for a worker branch, then clean up.
# Args:
#   $1: Branch name (string, format: worker/{role}/{task}).
# Output: Logs state transitions, merge issues, and audit events.
# Returns: 0 on completion; exits on fatal git errors.
process_worker_branch() {
  local branch="$1"
  local worker_name="${branch#worker/}"
  worker_name="${worker_name%%/*}"

  local task_name
  task_name="${branch##*/}"

  if ! ensure_worker_branch_present "${task_name}" "${worker_name}" "${branch}"; then
    return 0
  fi

  local task_dir
  if ! task_dir="$(resolve_task_dir_or_cleanup "${task_name}" "${worker_name}" "${branch}")"; then
    return 0
  fi

  local main_task_file=""
  local main_task_dir=""
  if main_task_file="$(task_file_for_name "${task_name}")"; then
    main_task_dir="$(basename "$(dirname "${main_task_file}")")"
  fi
  if [[ "${worker_name}" != "reviewer" && "${main_task_dir}" == "task-blocked" ]]; then
    handle_non_reviewer_branch_state "${task_name}" "${worker_name}" "task-blocked" "${branch}" "${main_task_file}"
    return 0
  fi

  if in_flight_has_task_worker "${task_name}" "${worker_name}" && ! worktree_has_completed "${task_name}" "${worker_name}"; then
    log_verbose "Worker branch ${branch} not completed; skipping."
    return 0
  fi

  local task_relpath="${STATE_DIR}/${task_dir}/${task_name}.md"
  local decision="block"
  local review_lines=()
  local block_reason=""
  local merge_branch="${branch}"
  local merge_worker="${worker_name}"

  log_worker_elapsed_if_known "${task_name}" "${worker_name}" "${branch}"

  if [[ "${worker_name}" == "reviewer" ]]; then
    mapfile -t review_lines < <(read_review_output_from_branch "${branch}")
    decision="${review_lines[0]:-block}"
    local task_role=""
    if task_role="$(extract_worker_from_task "${task_relpath}" 2> /dev/null)"; then
      if [[ "${task_role}" != "reviewer" ]]; then
        merge_branch="worker/${task_role}/${task_name}"
        merge_worker="${task_role}"
      fi
    else
      decision="block"
      block_reason="Missing role suffix for ${task_name}."
    fi
  else
    if handle_non_reviewer_branch_state "${task_name}" "${worker_name}" "${task_dir}" "${branch}" "${task_relpath}"; then
      return 0
    fi
  fi

  validate_task_state_for_processing "${task_name}" "${worker_name}" "${task_dir}" "decision" "block_reason"

  local merge_ready=1
  # Check if the merge branch exists locally
  if ! git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/heads/${merge_branch}"; then
    decision="block"
    block_reason="Missing worker branch ${merge_branch} for ${task_name}."
    merge_ready=0
  fi

  git_checkout_default_branch

  if [[ "${merge_ready}" -eq 0 ]]; then
    finalize_review_and_cleanup \
      "${task_name}" \
      "${worker_name}" \
      "${decision}" \
      "${block_reason}" \
      "${merge_worker}" \
      "${review_lines[@]:1}"
    return 0
  fi
  if ! merge_branch_into_default "${task_name}" "${merge_branch}"; then
    handle_merge_failure "${task_name}" "${worker_name}" "${merge_worker}" "${merge_branch}"
    return 0
  fi

  finalize_review_and_cleanup \
    "${task_name}" \
    "${worker_name}" \
    "${decision}" \
    "${block_reason}" \
    "${merge_worker}" \
    "${review_lines[@]:1}"
}

# process_worker_branches
# Purpose: Process all local worker branches while skipping known failed merges.
# Args: None.
# Output: Logs scanning and per-branch processing messages.
# Returns: 0 on completion.
process_worker_branches() {
  touch_logs

  check_zombie_workers
  cleanup_stale_worktrees

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
