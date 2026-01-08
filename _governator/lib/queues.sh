# shellcheck shell=bash

# assign_task
# Purpose: Move a backlog task to assigned, commit it, and spawn a worker.
# Args:
#   $1: Task file path (string).
#   $2: Worker role (string).
# Output: Logs assignment and spawns worker process.
# Returns: 0 on success; exits on git or spawn failures.
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

# can_assign_task
# Purpose: Check global and per-role caps for a task assignment.
# Args:
#   $1: Worker role (string).
#   $2: Task name (string).
# Output: Prints a human-readable skip reason on failure.
# Returns: 0 if assignment is allowed; 1 otherwise.
can_assign_task() {
  local worker="$1"
  local task_name="$2"

  local total_count
  total_count="$(count_in_flight)"
  local global_cap
  global_cap="$(read_global_cap)"
  if [[ "${total_count}" -ge "${global_cap}" ]]; then
    printf 'Global worker cap reached (%s/%s), skipping %s.' "${total_count}" "${global_cap}" "${task_name}"
    return 1
  fi

  local role_count
  role_count="$(count_in_flight "${worker}")"
  local role_cap
  role_cap="$(read_worker_cap "${worker}")"
  if [[ "${role_count}" -ge "${role_cap}" ]]; then
    printf 'Role %s at cap (%s/%s) for %s, skipping.' "${worker}" "${role_count}" "${role_cap}" "${task_name}"
    return 1
  fi

  return 0
}

# gap_analysis_planner_active
# Purpose: Determine whether the gap-analysis planner task exists.
# Args: None.
# Output: None.
# Returns: 0 if the planner task exists; 1 otherwise.
gap_analysis_planner_active() {
  if task_exists "${GAP_ANALYSIS_PLANNER_TASK}"; then
    return 0
  fi
  return 1
}

# ensure_gap_analysis_planner_task
# Purpose: Create the gap-analysis planner task when GOVERNATOR.md changes.
# Notes: Skips creation until a bootstrap task has been completed.
# Args: None.
# Output: Logs task creation and commits changes.
# Returns: 0 on completion; 1 on failure to copy the template.
ensure_gap_analysis_planner_task() {
  if ! architecture_bootstrap_complete; then
    return 0
  fi
  if ! governator_hash_mismatch; then
    return 0
  fi
  if task_exists "${GAP_ANALYSIS_PLANNER_TASK}"; then
    return 0
  fi
  if [[ ! -f "${GAP_ANALYSIS_PLANNER_TEMPLATE}" ]]; then
    log_error "Missing gap-analysis template at ${GAP_ANALYSIS_PLANNER_TEMPLATE}."
    return 1
  fi
  if ! role_exists "${GAP_ANALYSIS_PLANNER_ROLE}"; then
    log_warn "Unknown role ${GAP_ANALYSIS_PLANNER_ROLE} for gap-analysis planner task."
    return 0
  fi

  local dest="${STATE_DIR}/task-assigned/${GAP_ANALYSIS_PLANNER_TASK}.md"
  cp "${GAP_ANALYSIS_PLANNER_TEMPLATE}" "${dest}"
  annotate_assignment "${dest}" "${GAP_ANALYSIS_PLANNER_ROLE}"
  log_task_event "${GAP_ANALYSIS_PLANNER_TASK}" "created gap-analysis planner task"

  git -C "${ROOT_DIR}" add "${dest}" "${AUDIT_LOG}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Create gap-analysis planner task"
  git_push_default_branch
  return 0
}

# assign_pending_tasks
# Purpose: Assign backlog tasks according to role suffix and caps.
# Args: None.
# Output: Logs task assignment decisions and blocking reasons.
# Returns: 0 on completion.
assign_pending_tasks() {
  touch_logs
  require_project_mode
  ensure_bootstrap_task_exists
  complete_bootstrap_task_if_ready || true

  if ! bootstrap_gate_allows_assignment; then
    return 0
  fi

  ensure_unblock_planner_task || true
  ensure_gap_analysis_planner_task || true
  if gap_analysis_planner_active; then
    log_verbose "Gap-analysis planner active; skipping backlog assignment"
    return 0
  fi

  handle_completion_check_when_idle
  assign_backlog_tasks
}

# bootstrap_gate_allows_assignment
# Purpose: Gate task assignment until bootstrap is complete.
# Args: None.
# Output: Logs gating decisions and bootstrap task assignment.
# Returns: 0 when assignment can continue; 1 when gating stops assignment.
bootstrap_gate_allows_assignment() {
  if architecture_bootstrap_complete; then
    return 0
  fi
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
  return 1
}

# queues_are_empty
# Purpose: Check whether all task queues are empty.
# Args: None.
# Output: None.
# Returns: 0 if empty; 1 otherwise.
queues_are_empty() {
  if [[ "$(count_task_files "${STATE_DIR}/task-backlog")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-assigned")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-worked")" -gt 0 ]] ||
    [[ "$(count_task_files "${STATE_DIR}/task-blocked")" -gt 0 ]]; then
    return 1
  fi
  return 0
}

# log_completion_check_cooldown
# Purpose: Log remaining completion-check cooldown time.
# Args: None.
# Output: Logs cooldown status.
# Returns: 0 always.
log_completion_check_cooldown() {
  local last_run
  last_run="$(read_completion_check_last_run)"
  local cooldown
  cooldown="$(read_completion_check_cooldown_seconds)"
  local now
  now="$(date +%s)"
  local remaining=$((cooldown - (now - last_run)))
  if [[ "${remaining}" -lt 0 ]]; then
    remaining=0
  fi
  log_verbose "Completion check cooldown active (${remaining}s remaining)"
}

# handle_completion_check_when_idle
# Purpose: Run completion-check logic when queues are empty.
# Args: None.
# Output: Logs completion-check decisions and triggers tasks as needed.
# Returns: 0 always.
handle_completion_check_when_idle() {
  if ! queues_are_empty; then
    log_verbose "Tasks pending; skipping completion check"
    return 0
  fi

  log_verbose "All queues empty"
  if governator_hash_mismatch; then
    if completion_check_due; then
      create_completion_check_task || true
    else
      log_completion_check_cooldown
    fi
  else
    log_verbose "Completion check not needed (planning.gov_hash matches GOVERNATOR.md)"
  fi
}

# assign_backlog_tasks
# Purpose: Assign backlog tasks to workers based on metadata and caps.
# Args: None.
# Output: Logs assignment decisions and blocking reasons.
# Returns: 0 on completion.
assign_backlog_tasks() {
  local active_milestone
  active_milestone="$(earliest_incomplete_milestone)"
  if [[ -n "${active_milestone}" ]]; then
    log_verbose "Active milestone gate: ${active_milestone}"
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
      move_task_to_blocked "${task_file}" "Missing required role in filename suffix."
      continue
    fi
    local metadata=()
    mapfile -t metadata <<< "${metadata_text}"
    local task_name="${metadata[0]}"
    local worker="${metadata[2]}"

    if in_flight_has_task "${task_name}"; then
      continue
    fi

    local milestone_note
    if ! milestone_note="$(milestone_gate_allows_task "${task_file}" "${active_milestone}")"; then
      log_verbose "Skipping ${task_name}; ${milestone_note}"
      continue
    fi

    local dependency_note
    if ! dependency_note="$(task_dependencies_satisfied "${task_file}")"; then
      log_verbose "Skipping ${task_name}; ${dependency_note}"
      continue
    fi

    if ! role_exists "${worker}"; then
      log_warn "Unknown role ${worker} for ${task_name}, blocking."
      move_task_to_blocked "${task_file}" "Unknown role ${worker} referenced in filename suffix."
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

# ensure_unblock_planner_task
# Purpose: Create a planner task to analyze blocked work when needed.
# Args: None.
# Output: Logs task creation and commits changes.
# Returns: 0 on completion; 1 on failure to copy the template.
ensure_unblock_planner_task() {
  local pending=()
  local task_file
  while IFS= read -r task_file; do
    pending+=("${task_file}")
  done < <(blocked_tasks_needing_unblock)

  if [[ "${#pending[@]}" -eq 0 ]]; then
    return 0
  fi
  if task_exists "${UNBLOCK_PLANNER_TASK}"; then
    return 0
  fi
  if [[ ! -f "${UNBLOCK_PLANNER_TEMPLATE}" ]]; then
    log_error "Missing unblock planner template at ${UNBLOCK_PLANNER_TEMPLATE}."
    return 1
  fi
  if ! role_exists "${UNBLOCK_PLANNER_ROLE}"; then
    log_warn "Unknown role ${UNBLOCK_PLANNER_ROLE} for unblock planner task."
    return 0
  fi

  local dest="${STATE_DIR}/task-assigned/${UNBLOCK_PLANNER_TASK}.md"
  cp "${UNBLOCK_PLANNER_TEMPLATE}" "${dest}"
  annotate_assignment "${dest}" "${UNBLOCK_PLANNER_ROLE}"

  local body=""
  local entry
  for entry in "${pending[@]}"; do
    local task_name
    task_name="$(basename "${entry}" .md)"
    local reason
    reason="$(extract_block_reason "${entry}")"
    body+="- ${task_name}: ${reason}"$'\n'
  done
  if [[ -n "${body}" ]]; then
    body="${body%$'\n'}"
    append_section "${dest}" "## Blocked Tasks" "governator" "${body}"
  fi

  log_task_event "${UNBLOCK_PLANNER_TASK}" "created unblock planner task"
  git -C "${ROOT_DIR}" add "${dest}" "${AUDIT_LOG}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Create unblock planner task"
  git_push_default_branch
  return 0
}

# resume_assigned_tasks
# Purpose: Retry assigned tasks that are not currently in-flight.
# Args: None.
# Output: Logs retry decisions and dispatches workers.
# Returns: 0 on completion.
resume_assigned_tasks() {
  touch_logs
  require_project_mode

  ensure_gap_analysis_planner_task || true
  local planner_active=0
  if gap_analysis_planner_active; then
    planner_active=1
    log_verbose "Gap-analysis planner active; pausing non-planner dispatch"
  fi

  local active_milestone
  active_milestone="$(earliest_incomplete_milestone)"
  if [[ -n "${active_milestone}" ]]; then
    log_verbose "Active milestone gate: ${active_milestone}"
  fi

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
      move_task_to_blocked "${task_file}" "Missing required role in filename suffix."
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
    if [[ "${planner_active}" -eq 1 && "${task_name}" != "${GAP_ANALYSIS_PLANNER_TASK}" ]]; then
      log_verbose "Planner active; deferring ${task_name}"
      continue
    fi

    local milestone_note
    if ! milestone_note="$(milestone_gate_allows_task "${task_file}" "${active_milestone}")"; then
      log_verbose "Skipping ${task_name}; ${milestone_note}"
      continue
    fi

    local dependency_note
    if ! dependency_note="$(task_dependencies_satisfied "${task_file}")"; then
      log_verbose "Skipping ${task_name}; ${dependency_note}"
      continue
    fi

    if worker_process_get "${task_name}" "${worker}" > /dev/null 2>&1; then
      in_flight_add "${task_name}" "${worker}"
      log_verbose "Skipping task ${task_name}; worker process already recorded for ${worker}"
      continue
    fi

    if ! role_exists "${worker}"; then
      log_warn "Unknown role ${worker} for ${task_name}, blocking."
      move_task_to_blocked "${task_file}" "Unknown role ${worker} referenced in filename suffix."
      continue
    fi

    local cap_note
    if ! cap_note="$(can_assign_task "${worker}" "${task_name}")"; then
      log_warn "${cap_note}"
      continue
    fi

    log_verbose "Dispatching worker ${worker} for ${task_name}"
    warn_if_task_template_incomplete "${task_file}" "${task_name}"
    in_flight_add "${task_name}" "${worker}"
    spawn_worker_for_task "${task_file}" "${worker}" "retrying ${worker} task"
  done < <(list_task_files_in_dir "${STATE_DIR}/task-assigned")
}
