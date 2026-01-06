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

# format_task_label
# Purpose: Format a task label for status output.
# Args:
#   $1: Task file path (string).
# Output: Prints formatted label to stdout.
# Returns: 0 always.
format_task_label() {
  local path="$1"
  task_label "${path}"
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
  print_task_list "${title}" "${dir}" format_task_label "${limit}"
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
  total="$(count_in_flight_total)"
  printf 'In-flight workers (%s):\n' "${total}"
  local now
  now="$(date +%s)"
  local printed=0
  local task
  local worker
  while IFS='|' read -r task worker; do
    local branch="n/a"
    local pid="n/a"
    local age="n/a"
    local info=()
    mapfile -t info < <(worker_process_get "${task}" "${worker}" 2> /dev/null)
    if [[ "${#info[@]}" -gt 0 ]]; then
      pid="${info[0]:-n/a}"
      branch="${info[2]:-n/a}"
      local started="${info[3]:-}"
      if [[ "${started}" =~ ^[0-9]+$ ]]; then
        local elapsed=$((now - started))
        age="$(format_duration "${elapsed}")"
      fi
    fi
    printf '  %-28s %-12s %-28s PID:%-6s age:%s\n' "${task}" "${worker}" "${branch}" "${pid}" "${age}"
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
  print_inflight_summary
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
  print_task_queue_summary
  printf '\n'
  print_inflight_summary
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
