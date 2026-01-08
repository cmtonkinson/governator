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

# process_worker_branch
# Purpose: Handle review and merge decisions for a worker branch, then clean up.
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

  local task_name
  task_name="${local_branch##*/}"

  git_fetch_remote
  if ! git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/remotes/${remote_branch}"; then
    log_warn "Worker branch missing at ${remote_branch}, skipping."
    in_flight_remove "${task_name}" "${worker_name}"
    cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
    return 0
  fi
  git -C "${ROOT_DIR}" branch -f "${local_branch}" "${remote_branch}" > /dev/null 2>&1

  local task_dir
  if ! task_dir="$(task_dir_for_branch "${remote_branch}" "${task_name}")"; then
    # No task to annotate; record and drop the branch.
    log_warn "No task file found for ${task_name} on ${remote_branch}, skipping merge."
    local missing_sha
    missing_sha="$(git -C "${ROOT_DIR}" rev-parse "${remote_branch}" 2> /dev/null || true)"
    printf '%s %s %s missing task file\n' "$(timestamp_utc_seconds)" "${remote_branch}" "${missing_sha:-unknown}" >> "${FAILED_MERGES_LOG}"
    in_flight_remove "${task_name}" "${worker_name}"
    delete_worker_branch "${local_branch}"
    cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
    return 0
  fi

  local task_relpath="${STATE_DIR}/${task_dir}/${task_name}.md"
  local decision="block"
  local review_lines=()
  local block_reason=""
  local merge_branch="${local_branch}"
  local merge_remote_branch="${remote_branch}"
  local merge_worker="${worker_name}"

  local elapsed
  if elapsed="$(worker_elapsed_seconds "${task_name}" "${worker_name}" "${local_branch}")"; then
    log_task_event "${task_name}" "worker elapsed ${worker_name}: ${elapsed}s"
  fi

  if [[ "${worker_name}" == "reviewer" ]]; then
    mapfile -t review_lines < <(read_review_output_from_branch "${local_branch}")
    decision="${review_lines[0]:-block}"
    local task_role=""
    if task_role="$(extract_worker_from_task "${task_relpath}" 2> /dev/null)"; then
      if [[ "${task_role}" != "reviewer" ]]; then
        merge_branch="worker/${task_role}/${task_name}"
        merge_remote_branch="${remote}/${merge_branch}"
        merge_worker="${task_role}"
      fi
    else
      decision="block"
      block_reason="Missing role suffix for ${task_name}."
    fi
  else
    if [[ "${task_dir}" == "task-worked" ]]; then
      in_flight_remove "${task_name}" "${worker_name}"
      cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"

      local review_branch="worker/reviewer/${task_name}"
      if in_flight_has_task_worker "${task_name}" "reviewer"; then
        log_verbose "Reviewer already in-flight for ${task_name}; skipping spawn"
        return 0
      fi
      if ! git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/remotes/${remote}/${review_branch}"; then
        local cap_note
        if ! cap_note="$(can_assign_task "reviewer" "${task_name}")"; then
          log_warn "${cap_note}"
        else
          in_flight_add "${task_name}" "reviewer"
          spawn_worker_for_task "${task_relpath}" "reviewer" "starting review for ${task_name}" "${remote}/${local_branch}"
        fi
      fi
      return 0
    fi
    if [[ "${task_dir}" == "task-blocked" ]]; then
      log_warn "Worker marked ${task_name} blocked; skipping merge."
      in_flight_remove "${task_name}" "${worker_name}"
      delete_worker_branch "${local_branch}"
      cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
      ensure_unblock_planner_task || true
      return 0
    fi
  fi

  case "${task_dir}" in
    task-worked)
      :
      ;;
    task-assigned)
      if [[ "${worker_name}" == "reviewer" && "${task_name}" == 000-* ]]; then
        :
      else
        decision="block"
        block_reason="Unexpected task state ${task_dir} during processing."
      fi
      ;;
    *)
      decision="block"
      block_reason="Unexpected task state ${task_dir} during processing."
      ;;
  esac

  local merge_ready=1
  git_fetch_remote
  if ! git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/remotes/${merge_remote_branch}"; then
    decision="block"
    block_reason="Missing worker branch ${merge_remote_branch} for ${task_name}."
    merge_ready=0
  else
    git -C "${ROOT_DIR}" branch -f "${merge_branch}" "${merge_remote_branch}" > /dev/null 2>&1
  fi

  git_checkout_default_branch

  local merged=0
  if [[ "${merge_ready}" -eq 0 ]]; then
    apply_review_decision "${task_name}" "${worker_name}" "${decision}" "${block_reason}" "${review_lines[@]:1}"
    git_push_default_branch
    in_flight_remove "${task_name}" "${worker_name}"
    if [[ "${merge_worker}" != "${worker_name}" ]]; then
      in_flight_remove "${task_name}" "${merge_worker}"
    fi
    delete_worker_branch "${local_branch}"
    if [[ "${merge_branch}" != "${local_branch}" ]]; then
      delete_worker_branch "${merge_branch}"
    fi
    cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
    cleanup_worker_tmp_dirs "${merge_worker}" "${task_name}"
    return 0
  fi
  if [[ "${merge_ready}" -eq 1 ]]; then
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
        failed_sha="$(git -C "${ROOT_DIR}" rev-parse "${merge_remote_branch}" 2> /dev/null || true)"
        printf '%s %s %s\n' "$(timestamp_utc_seconds)" "${merge_remote_branch}" "${failed_sha:-unknown}" >> "${FAILED_MERGES_LOG}"
        in_flight_remove "${task_name}" "${worker_name}"
        if [[ "${merge_worker}" != "${worker_name}" ]]; then
          in_flight_remove "${task_name}" "${merge_worker}"
        fi
        cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
        cleanup_worker_tmp_dirs "${merge_worker}" "${task_name}"
        delete_worker_branch "${merge_branch}"
        if [[ "${merge_branch}" != "${local_branch}" ]]; then
          delete_worker_branch "${local_branch}"
        fi
        return 0
      fi
    fi
  fi

  if [[ "${merged}" -eq 1 ]]; then
    apply_review_decision "${task_name}" "${worker_name}" "${decision}" "${block_reason}" "${review_lines[@]:1}"
    git_push_default_branch
  fi

  in_flight_remove "${task_name}" "${worker_name}"
  if [[ "${merge_worker}" != "${worker_name}" ]]; then
    in_flight_remove "${task_name}" "${merge_worker}"
  fi

  delete_worker_branch "${merge_branch}"
  if [[ "${merge_branch}" != "${local_branch}" ]]; then
    delete_worker_branch "${local_branch}"
  fi
  cleanup_worker_tmp_dirs "${worker_name}" "${task_name}"
  cleanup_worker_tmp_dirs "${merge_worker}" "${task_name}"
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
