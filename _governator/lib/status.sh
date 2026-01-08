# shellcheck shell=bash

# print_task_queue_summary
# Purpose: Print queue counts for all task states.
# Args: None.
# Output: Writes queue counts to stdout.
# Returns: 0 on completion.
print_task_queue_summary() {
  local entries=(
    "task-backlog:Backlog"
    "task-assigned:Assigned"
    "task-worked:Awaiting review"
    "task-blocked:Blocked"
    "task-done:Done"
  )
  printf 'Task queues:\n'
  local pair
  for pair in "${entries[@]}"; do
    local dir="${pair%%:*}"
    local label="${pair##*:}"
    local count
    count="$(count_task_files "${STATE_DIR}/${dir}")"
    printf '  %-22s %s\n' "${label}:" "${count}"
  done
}

# print_project_status
# Purpose: Print a project-level completion status summary.
# Args: None.
# Output: Writes the project status line to stdout.
# Returns: 0 on completion.
print_project_status() {
  local backlog_count
  backlog_count="$(count_task_files "${STATE_DIR}/task-backlog")"
  local assigned_count
  assigned_count="$(count_task_files "${STATE_DIR}/task-assigned")"
  local worked_count
  worked_count="$(count_task_files "${STATE_DIR}/task-worked")"
  local blocked_count
  blocked_count="$(count_task_files "${STATE_DIR}/task-blocked")"
  local pending_total=$((backlog_count + assigned_count + worked_count + blocked_count))

  local completion_needed=0
  local completion_label="up-to-date"
  if governator_hash_mismatch; then
    completion_needed=1
    if completion_check_due; then
      completion_label="due"
    else
      completion_label="cooldown"
    fi
  fi

  if [[ "${pending_total}" -eq 0 && "${completion_needed}" -eq 0 ]]; then
    printf 'Project status: DONE (completion check %s; pending tasks %s)\n' "${completion_label}" "${pending_total}"
  else
    printf 'Project status: IN PROGRESS (completion check %s; pending tasks %s)\n' "${completion_label}" "${pending_total}"
  fi
}

# inflight_pid_status
# Purpose: Report whether a worker PID is active.
# Args:
#   $1: PID value (string).
# Output: Prints "active" or "inactive" to stdout.
# Returns: 0 always.
inflight_pid_status() {
  local pid="$1"
  if [[ -n "${pid}" && "${pid}" =~ ^[0-9]+$ ]]; then
    if kill -0 "${pid}" > /dev/null 2>&1; then
      printf '%s\n' "active"
      return 0
    fi
  fi
  printf '%s\n' "inactive"
}

# milestone_epic_rows
# Purpose: Emit milestone and epic status rows from task frontmatter.
# Args: None.
# Output: Writes "milestone|epic|done" rows to stdout.
# Returns: 0 on completion.
milestone_epic_rows() {
  local task_file
  while IFS= read -r task_file; do
    local task_name
    task_name="$(basename "${task_file}" .md)"
    if [[ "${task_name}" == 000-* ]]; then
      continue
    fi
    local milestone
    milestone="$(frontmatter_value "${task_file}" "milestone")"
    if [[ -z "${milestone}" ]]; then
      continue
    fi
    local epic
    epic="$(frontmatter_value "${task_file}" "epic")"
    if [[ -z "${epic}" ]]; then
      epic="unscoped"
    fi
    local done=0
    if [[ "$(basename "$(dirname "${task_file}")")" == "task-done" ]]; then
      done=1
    fi
    printf '%s|%s|%s\n' "${milestone}" "${epic}" "${done}"
  done < <(
    list_task_files_in_dir "${STATE_DIR}/task-backlog"
    list_task_files_in_dir "${STATE_DIR}/task-assigned"
    list_task_files_in_dir "${STATE_DIR}/task-worked"
    list_task_files_in_dir "${STATE_DIR}/task-blocked"
    list_task_files_in_dir "${STATE_DIR}/task-done"
  )
}

# print_milestone_epic_summary
# Purpose: Summarize milestone and epic completion from task frontmatter.
# Args: None.
# Output: Writes milestone and epic completion summary to stdout.
# Returns: 0 on completion.
print_milestone_epic_summary() {
  printf 'Milestone progress:\n'
  local summary
  summary="$(
    milestone_epic_rows | sort -t '|' -k1,1 -k2,2 | awk -F'|' '
      {
        m=$1; e=$2; done=$3
        if (m == "") next
        total[m SUBSEP e]++
        if (done == "1") done_count[m SUBSEP e]++
        m_total[m]++
        if (done == "1") m_done[m]++
        if (!(m in m_order)) { m_order[m]=++m_idx; m_list[m_idx]=m }
        if (!(m SUBSEP e in e_order)) { e_order[m SUBSEP e]=++e_idx; e_list[e_idx]=m SUBSEP e }
      }
      END {
        for (i=1; i<=m_idx; i++) {
          m=m_list[i]
          pct=int((m_done[m]*100)/m_total[m])
          printf "Milestone %s: %d%%\n", m, pct
          for (j=1; j<=e_idx; j++) {
            split(e_list[j], parts, SUBSEP)
            if (parts[1] != m) continue
            e=parts[2]
            epct=int((done_count[e_list[j]]*100)/total[e_list[j]])
            printf "\tEpic %s: %d%%\n", e, epct
          }
        }
      }
    '
  )"
  if [[ -z "${summary}" ]]; then
    printf '  (none)\n'
  else
    printf '%s\n' "${summary}"
  fi
}

# format_blocked_task
# Purpose: Format a blocked task with its block reason.
# Args:
#   $1: Task file path (string).
# Output: Prints formatted blocked task label to stdout.
# Returns: 0 always.
format_blocked_task() {
  local path="$1"
  printf '%s (%s)' "$(task_label "${path}")" "$(extract_block_reason "${path}")"
}

# print_task_list
# Purpose: Print a labeled list of tasks from a directory.
# Args:
#   $1: Title label (string).
#   $2: Directory path (string).
#   $3: Formatter function name (string).
#   $4: Max items to print (integer, optional).
# Output: Writes list to stdout.
# Returns: 0 on completion.
print_task_list() {
  local title="$1"
  local dir="$2"
  local formatter="$3"
  local limit="${4:-0}"
  printf '%s:\n' "${title}"
  local printed=0
  local path
  while IFS= read -r path; do
    printed=$((printed + 1))
    printf '  - %s\n' "$("${formatter}" "${path}")"
    if [[ "${limit}" -gt 0 && "${printed}" -ge "${limit}" ]]; then
      break
    fi
  done < <(list_task_files_in_dir "${dir}")
  if [[ "${printed}" -eq 0 ]]; then
    printf '  (none)\n'
  fi
}

# print_stage_task_list
# Purpose: Print a task list for a specific stage with a default limit.
# Args:
#   $1: Title label (string).
#   $2: Directory path (string).
#   $3: Max items to print (integer, optional).
# Output: Writes list to stdout.
# Returns: 0 on completion.
print_stage_task_list() {
  local title="$1"
  local dir="$2"
  local limit="${3:-5}"
  print_task_list "${title}" "${dir}" task_label "${limit}"
}

# print_blocked_tasks_summary
# Purpose: Print the list of blocked tasks with reasons.
# Args: None.
# Output: Writes list to stdout.
# Returns: 0 on completion.
print_blocked_tasks_summary() {
  print_task_list "Blocked tasks" "${STATE_DIR}/task-blocked" format_blocked_task
}

# print_pending_reviewer_branches
# Purpose: Print tasks awaiting a reviewer branch.
# Args: None.
# Output: Writes list to stdout.
# Returns: 0 on completion.
print_pending_reviewer_branches() {
  printf 'Reviews awaiting reviewer branch:\n'
  local remote
  remote="$(read_remote_name)"
  local printed=0
  local task_file
  while IFS= read -r task_file; do
    if [[ "${task_file}" == *"/.keep" ]]; then
      continue
    fi
    local task_name
    task_name="$(basename "${task_file}" .md)"
    local reviewer_ref="refs/remotes/${remote}/worker/reviewer/${task_name}"
    if ! git -C "${ROOT_DIR}" show-ref --verify --quiet "${reviewer_ref}"; then
      printf '  - %s\n' "${task_name}"
      printed=1
    fi
  done < <(list_task_files_in_dir "${STATE_DIR}/task-worked")
  if [[ "${printed}" -eq 0 ]]; then
    printf '  (none)\n'
  fi
}

# print_pending_branches
# Purpose: Print the list of pending worker branches.
# Args: None.
# Output: Writes branch list to stdout.
# Returns: 0 on completion.
print_pending_branches() {
  printf 'Pending worker branches:\n'
  local remote
  remote="$(read_remote_name)"
  local printed=0
  local task
  local worker
  while IFS='|' read -r task worker; do
    if [[ -z "${task}" || -z "${worker}" ]]; then
      continue
    fi
    local branch
    branch="worker/${worker}/${task}"
    printf '  - %s/%s\n' "${remote}" "${branch}"
    printed=1
  done < <(in_flight_entries)
  if [[ "${printed}" -eq 0 ]]; then
    printf '  (none)\n'
  fi
}

# print_inflight_summary
# Purpose: Print current in-flight workers with metadata.
# Args: None.
# Output: Writes in-flight list to stdout.
# Returns: 0 on completion.
print_inflight_summary() {
  local total
  total="$(count_in_flight)"
  printf 'In-flight workers (%s):\n' "${total}"
  local now
  now="$(date +%s)"
  local printed=0
  printf '  %-8s %-8s %-10s %-12s %s\n' "PID" "STATUS" "AGE" "ROLE" "TASK"
  local task
  local worker
  while IFS='|' read -r task worker; do
    local pid="n/a"
    local age="n/a"
    local status="inactive"
    local info=()
    mapfile -t info < <(worker_process_get "${task}" "${worker}" 2> /dev/null)
    if [[ "${#info[@]}" -gt 0 ]]; then
      pid="${info[0]:-n/a}"
      status="$(inflight_pid_status "${pid}")"
      local started="${info[3]:-}"
      if [[ "${started}" =~ ^[0-9]+$ ]]; then
        local elapsed=$((now - started))
        age="$(format_duration "${elapsed}")"
      fi
    fi
    printf '  %-8s %-8s %-10s %-12s %s\n' "${pid}" "${status}" "${age}" "${worker}" "${task}"
    printed=$((printed + 1))
  done < <(in_flight_entries)
  if [[ "${printed}" -eq 0 ]]; then
    printf '  (none)\n'
  fi
}

# print_activity_snapshot
# Purpose: Print a snapshot of active work and queues.
# Args: None.
# Output: Writes snapshot to stdout.
# Returns: 0 on completion.
print_activity_snapshot() {
  print_project_status
  printf '\n'
  print_inflight_summary
  printf '\n'
  print_milestone_epic_summary
  printf '\n'
  print_stage_task_list "Pending reviews" "${STATE_DIR}/task-worked"
  printf '\n'
  print_pending_reviewer_branches
  printf '\n'
  print_blocked_tasks_summary
  printf '\n'
  print_pending_branches
}

# status_dashboard
# Purpose: Print the full status dashboard including queues and workers.
# Args: None.
# Output: Writes status dashboard to stdout.
# Returns: 0 on completion.
status_dashboard() {
  local locked_note=''
  if system_locked; then
    local since
    if since="$(locked_since)"; then
      locked_note=" (LOCKED since ${since})"
    else
      locked_note=' (LOCKED)'
    fi
  fi
  printf 'Governator Status%s\n' "${locked_note}"
  if git_fetch_remote > /dev/null 2>&1; then
    :
  else
    log_warn 'Failed to fetch remote refs for status'
  fi
  print_project_status
  printf '\n'
  print_task_queue_summary
  printf '\n'
  print_inflight_summary
  printf '\n'
  print_milestone_epic_summary
  printf '\n'
  print_stage_task_list "Pending reviews" "${STATE_DIR}/task-worked"
  printf '\n'
  print_pending_reviewer_branches
  printf '\n'
  print_blocked_tasks_summary
  printf '\n'
  print_pending_branches
  if system_locked; then
    printf '\nNOTE: Governator is locked; no new activity will start and data may be stale.\n'
  else
    :
  fi
  return 0
}

# handle_locked_state
# Purpose: Print lock notice and snapshot when governator is locked.
# Args:
#   $1: Context string (string).
# Output: Writes lock notice and snapshot to stdout.
# Returns: 0 if locked; 1 if not locked.
handle_locked_state() {
  local context="$1"
  if system_locked; then
    printf 'Governator is locked; skipping %s. Active work snapshot:\n' "${context}"
    print_activity_snapshot
    return 0
  fi
  return 1
}
