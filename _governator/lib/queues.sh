# shellcheck shell=bash

# Move task to assigned, commit, push, then spawn a worker.
assign_task() {
  local task_file="$1"
  local worker="$2"

  sync_default_branch

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local assigned_file="${STATE_DIR}/task-assigned/${task_name}.md"
  annotate_assignment "${task_file}" "${worker}"
  move_task_file "${task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "assigned to ${worker}"

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Assign task ${task_name}"
  git_push_default_branch

  warn_if_task_template_incomplete "${assigned_file}" "${task_name}"
  spawn_worker_for_task "${assigned_file}" "${worker}" ""
}

# Check caps for a worker/task pair; prints reason on failure.
can_assign_task() {
  local worker="$1"
  local task_name="$2"

  local total_count
  total_count="$(count_in_flight_total)"
  local global_cap
  global_cap="$(read_global_cap)"
  if [[ "${total_count}" -ge "${global_cap}" ]]; then
    printf 'Global worker cap reached (%s/%s), skipping %s.' "${total_count}" "${global_cap}" "${task_name}"
    return 1
  fi

  local role_count
  role_count="$(count_in_flight_role "${worker}")"
  local role_cap
  role_cap="$(read_worker_cap "${worker}")"
  if [[ "${role_count}" -ge "${role_cap}" ]]; then
    printf 'Role %s at cap (%s/%s) for %s, skipping.' "${worker}" "${role_count}" "${role_cap}" "${task_name}"
    return 1
  fi

  return 0
}

# Assign tasks in backlog based on role prefix/suffix in filename.
assign_pending_tasks() {
  touch_logs
  require_project_mode
  ensure_bootstrap_task_exists
  complete_bootstrap_task_if_ready || true

  # Gate normal task assignment until bootstrap completes.
  if ! architecture_bootstrap_complete; then
    log_verbose "Not bootstrapped; skipping task assignment"
    local blocking_task
    if blocking_task="$(has_non_bootstrap_tasks)"; then
      log_warn "Bootstrap incomplete; ignoring task ${blocking_task}."
    fi
    local bootstrap_task
    if bootstrap_task="$(bootstrap_task_path)"; then
      local task_dir
      task_dir="$(basename "$(dirname "${bootstrap_task}")")"
      if [[ "${task_dir}" == "task-backlog" ]]; then
        if ! in_flight_has_task "${BOOTSTRAP_TASK_NAME}"; then
          assign_bootstrap_task "${bootstrap_task}"
        fi
      fi
    fi
    return 0
  fi

  local queues_empty=1
  if [[ "$(count_task_files "${STATE_DIR}/task-backlog")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-assigned")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-worked")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-feedback")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-blocked")" -gt 0 ]]; then
    queues_empty=0
  fi

  if [[ "${queues_empty}" -eq 1 ]]; then
    log_verbose "All queues empty"
    if done_check_needed; then
      if done_check_due; then
        create_done_check_task || true
      else
        local last_run
        last_run="$(read_done_check_last_run)"
        local cooldown
        cooldown="$(read_done_check_cooldown_seconds)"
        local now
        now="$(date +%s)"
        local remaining=$((cooldown - (now - last_run)))
        if [[ "${remaining}" -lt 0 ]]; then
          remaining=0
        fi
        log_verbose "Done check cooldown active (${remaining}s remaining)"
      fi
    else
      log_verbose "Done check not needed (project_done matches GOVERNATOR.md)"
    fi
  else
    log_verbose "Tasks pending; skipping done check"
  fi

  local task_file
  while IFS= read -r task_file; do
    if [[ "${task_file}" == *"/.keep" ]]; then
      continue
    fi

    local metadata_text
    if ! metadata_text="$(parse_task_metadata "${task_file}")"; then
      local task_name
      task_name="$(basename "${task_file}" .md)"
      log_warn "Missing required role for ${task_name}, blocking."
      block_task_from_backlog "${task_file}" "Missing required role in filename suffix."
      continue
    fi
    local metadata=()
    mapfile -t metadata <<< "${metadata_text}"
    local task_name="${metadata[0]}"
    local worker="${metadata[2]}"

    if in_flight_has_task "${task_name}"; then
      continue
    fi

    if ! role_exists "${worker}"; then
      log_warn "Unknown role ${worker} for ${task_name}, blocking."
      block_task_from_backlog "${task_file}" "Unknown role ${worker} referenced in filename suffix."
      continue
    fi

    local cap_note
    if ! cap_note="$(can_assign_task "${worker}" "${task_name}")"; then
      log_warn "${cap_note}"
      continue
    fi

    log_verbose "Assigning backlog task ${task_name} to ${worker}"
    assign_task "${task_file}" "${worker}"
    in_flight_add "${task_name}" "${worker}"
  done < <(list_task_files_in_dir "${STATE_DIR}/task-backlog")
}

# Re-run tasks sitting in task-assigned when not already in flight.
resume_assigned_tasks() {
  touch_logs
  require_project_mode

  log_verbose "Resuming assigned tasks"
  local task_file
  while IFS= read -r task_file; do
    if [[ "${task_file}" == *"/.keep" ]]; then
      continue
    fi

    local metadata_text
    if ! metadata_text="$(parse_task_metadata "${task_file}")"; then
      local task_name
      task_name="$(basename "${task_file}" .md)"
      log_warn "Missing required role for ${task_name}, blocking."
      block_task_from_assigned "${task_file}" "Missing required role in filename suffix."
      continue
    fi
    local metadata=()
    mapfile -t metadata <<< "${metadata_text}"
    local task_name="${metadata[0]}"
    local worker="${metadata[2]}"

    if in_flight_has_task "${task_name}"; then
      log_verbose "Skipping in-flight task ${task_name}"
      continue
    fi

    if ! role_exists "${worker}" && ! special_role_exists "${worker}"; then
      log_warn "Unknown role ${worker} for ${task_name}, blocking."
      block_task_from_assigned "${task_file}" "Unknown role ${worker} referenced in filename suffix."
      continue
    fi

    local cap_note
    if ! cap_note="$(can_assign_task "${worker}" "${task_name}")"; then
      log_warn "${cap_note}"
      continue
    fi

    if special_role_exists "${worker}"; then
      log_verbose "Dispatching special role ${worker} for ${task_name}"
      warn_if_task_template_incomplete "${task_file}" "${task_name}"
      in_flight_add "${task_name}" "${worker}"
      spawn_special_worker_for_task "${task_file}" "${worker}" "retrying ${worker} task"
      in_flight_remove "${task_name}" "${worker}"
      continue
    fi

    log_verbose "Dispatching worker ${worker} for ${task_name}"
    warn_if_task_template_incomplete "${task_file}" "${task_name}"
    in_flight_add "${task_name}" "${worker}"
    spawn_worker_for_task "${task_file}" "${worker}" "retrying ${worker} task"
  done < <(list_task_files_in_dir "${STATE_DIR}/task-assigned")
}
