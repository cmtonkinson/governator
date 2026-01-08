# shellcheck shell=bash

# list_task_files_in_dir
# Purpose: List task markdown files within a directory.
# Args:
#   $1: Directory path (string).
# Output: Prints matching file paths to stdout.
# Returns: 0 always.
list_task_files_in_dir() {
  local dir="$1"
  if [[ ! -d "${dir}" ]]; then
    return 0
  fi
  local path
  while IFS= read -r path; do
    local base
    base="$(basename "${path}")"
    if [[ "${base}" == ".keep" ]]; then
      continue
    fi
    printf '%s\n' "${path}"
  done < <(find "${dir}" -maxdepth 1 -type f -name '*.md' 2> /dev/null | sort)
}

# count_task_files
# Purpose: Count task markdown files within a directory.
# Args:
#   $1: Directory path (string).
# Output: Prints the count to stdout.
# Returns: 0 always.
count_task_files() {
  local dir="$1"
  local count=0
  local path
  while IFS= read -r path; do
    count=$((count + 1))
  done < <(list_task_files_in_dir "${dir}")
  printf '%s\n' "${count}"
}

# task_label
# Purpose: Build a display label for a task file.
# Args:
#   $1: Task file path (string).
# Output: Prints label to stdout.
# Returns: 0 always.
task_label() {
  local file="$1"
  local name
  name="$(basename "${file}" .md)"
  local role
  if role="$(extract_worker_from_task "${file}" 2> /dev/null)"; then
    printf '%s (%s)' "${name}" "${role}"
  else
    printf '%s' "${name}"
  fi
}

# frontmatter_value
# Purpose: Extract a YAML frontmatter value from a task file.
# Args:
#   $1: Task file path (string).
#   $2: Key name (string).
# Output: Prints the value when present.
# Returns: 0 always.
frontmatter_value() {
  local file="$1"
  local key="$2"
  awk -v want="${key}" '
    NR == 1 && $0 == "---" { in_frontmatter=1; next }
    in_frontmatter == 1 {
      if ($0 == "---") exit
      if ($0 ~ /^[A-Za-z0-9_-]+:[[:space:]]*/) {
        split($0, parts, ":")
        k = parts[1]
        v = substr($0, index($0, ":") + 1)
        sub(/^[ \t]+/, "", v)
        sub(/[ \t]+$/, "", v)
        if (k == want) { print v; exit }
      }
    }
  ' "${file}"
}

# frontmatter_list
# Purpose: Extract a YAML frontmatter list value into one entry per line.
# Args:
#   $1: Task file path (string).
#   $2: Key name (string).
# Output: Prints list entries, one per line.
# Returns: 0 always.
frontmatter_list() {
  local file="$1"
  local key="$2"
  local raw
  raw="$(
    awk -v want="${key}" '
      NR == 1 && $0 == "---" { in_frontmatter=1; next }
      in_frontmatter == 1 {
        if ($0 == "---") exit
        if (!found) {
          if ($0 ~ ("^" want ":[[:space:]]*")) {
            found=1
            value=$0
            sub("^" want ":[[:space:]]*", "", value)
            if (value == "") {
              list_mode=1
              next
            }
            print value
            exit
          }
        } else if (list_mode) {
          if ($0 ~ /^[[:space:]]*-[[:space:]]*/) {
            item=$0
            sub(/^[[:space:]]*-[[:space:]]*/, "", item)
            print item
            next
          }
          exit
        }
      }
    ' "${file}"
  )"
  if [[ -z "${raw}" ]]; then
    return 0
  fi
  printf '%s\n' "${raw}" | awk '
    {
      line=$0
      gsub(/^\[/, "", line)
      gsub(/\]$/, "", line)
      count=split(line, parts, ",")
      for (i=1; i<=count; i++) {
        item=parts[i]
        gsub(/^[ \t]+/, "", item)
        gsub(/[ \t]+$/, "", item)
        if (item ~ /^".*"$/ || item ~ /^'\''.*'\''$/) {
          item=substr(item, 2, length(item) - 2)
        }
        if (item != "") print item
      }
    }
  '
}

# normalize_dependency_id
# Purpose: Normalize dependency ids to a zero-padded task id.
# Args:
#   $1: Dependency id (string).
# Output: Prints normalized id to stdout.
# Returns: 0 if id is numeric; 1 otherwise.
normalize_dependency_id() {
  local dep_id="$1"
  if [[ -z "${dep_id}" || ! "${dep_id}" =~ ^[0-9]+$ ]]; then
    return 1
  fi
  printf '%03d' "${dep_id}"
}

# task_dependency_ids
# Purpose: Extract numeric dependency ids from task frontmatter.
# Args:
#   $1: Task file path (string).
# Output: Prints dependency ids, one per line.
# Returns: 0 always.
task_dependency_ids() {
  local task_file="$1"
  local raw
  while IFS= read -r raw; do
    if dep_id="$(normalize_dependency_id "${raw}")"; then
      printf '%s\n' "${dep_id}"
    fi
  done < <(frontmatter_list "${task_file}" "depends_on")
}

# task_dependency_done
# Purpose: Check if a dependency id has a matching done task.
# Args:
#   $1: Dependency id (string).
# Output: None.
# Returns: 0 if a matching task is done; 1 otherwise.
task_dependency_done() {
  local dep_id="$1"
  if compgen -G "${STATE_DIR}/task-done/${dep_id}-*.md" > /dev/null; then
    return 0
  fi
  return 1
}

# task_id_exists
# Purpose: Check if any task file exists for a numeric id prefix.
# Args:
#   $1: Dependency id (string).
# Output: None.
# Returns: 0 if any task exists; 1 otherwise.
task_id_exists() {
  local dep_id="$1"
  if find_task_files "${dep_id}-*" | grep -q .; then
    return 0
  fi
  return 1
}

# task_dependencies_satisfied
# Purpose: Determine whether all dependencies for a task are done.
# Args:
#   $1: Task file path (string).
# Output: Prints a blocking reason when dependencies are missing or incomplete.
# Returns: 0 if all dependencies are satisfied; 1 otherwise.
task_dependencies_satisfied() {
  local task_file="$1"
  local missing=()
  local incomplete=()
  local dep_id
  while IFS= read -r dep_id; do
    if task_dependency_done "${dep_id}"; then
      continue
    fi
    if task_id_exists "${dep_id}"; then
      incomplete+=("${dep_id}")
    else
      missing+=("${dep_id}")
    fi
  done < <(task_dependency_ids "${task_file}")

  if [[ "${#missing[@]}" -eq 0 && "${#incomplete[@]}" -eq 0 ]]; then
    return 0
  fi
  if [[ "${#missing[@]}" -gt 0 ]]; then
    printf 'Depends on missing task id(s): %s.' "${missing[*]}"
    return 1
  fi
  printf 'Depends on incomplete task id(s): %s.' "${incomplete[*]}"
  return 1
}

# list_milestone_ids
# Purpose: List unique milestone ids from task frontmatter.
# Args: None.
# Output: Prints milestone ids, one per line.
# Returns: 0 always.
list_milestone_ids() {
  local task_file
  local -A seen=()
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
    if [[ -z "${seen[${milestone}]+x}" ]]; then
      seen["${milestone}"]=1
      printf '%s\n' "${milestone}"
    fi
  done < <(
    list_task_files_in_dir "${STATE_DIR}/task-backlog"
    list_task_files_in_dir "${STATE_DIR}/task-assigned"
    list_task_files_in_dir "${STATE_DIR}/task-worked"
    list_task_files_in_dir "${STATE_DIR}/task-blocked"
    list_task_files_in_dir "${STATE_DIR}/task-done"
  )
}

# sorted_milestone_ids
# Purpose: Sort milestone ids with numeric order for M-number patterns.
# Args: None.
# Output: Prints sorted milestone ids, one per line.
# Returns: 0 always.
sorted_milestone_ids() {
  list_milestone_ids | awk '
    {
      id=$0
      if (id ~ /^M[0-9]+$/) {
        num=substr(id, 2) + 0
        printf "0|%010d|%s\n", num, id
      } else {
        printf "1|%s|%s\n", id, id
      }
    }
  ' | sort -t '|' -k1,1 -k2,2 | cut -d '|' -f3
}

# milestone_is_complete
# Purpose: Check whether all tasks for a milestone are in task-done.
# Args:
#   $1: Milestone id (string).
# Output: None.
# Returns: 0 if complete; 1 if any task is incomplete.
milestone_is_complete() {
  local milestone="$1"
  local task_file
  while IFS= read -r task_file; do
    local task_name
    task_name="$(basename "${task_file}" .md)"
    if [[ "${task_name}" == 000-* ]]; then
      continue
    fi
    local current
    current="$(frontmatter_value "${task_file}" "milestone")"
    if [[ "${current}" != "${milestone}" ]]; then
      continue
    fi
    local dir
    dir="$(basename "$(dirname "${task_file}")")"
    if [[ "${dir}" != "task-done" ]]; then
      return 1
    fi
  done < <(
    list_task_files_in_dir "${STATE_DIR}/task-backlog"
    list_task_files_in_dir "${STATE_DIR}/task-assigned"
    list_task_files_in_dir "${STATE_DIR}/task-worked"
    list_task_files_in_dir "${STATE_DIR}/task-blocked"
    list_task_files_in_dir "${STATE_DIR}/task-done"
  )
  return 0
}

# earliest_incomplete_milestone
# Purpose: Return the earliest milestone id that is not complete.
# Args: None.
# Output: Prints the milestone id when present.
# Returns: 0 always.
earliest_incomplete_milestone() {
  local milestone
  while IFS= read -r milestone; do
    if ! milestone_is_complete "${milestone}"; then
      printf '%s\n' "${milestone}"
      return 0
    fi
  done < <(sorted_milestone_ids)
}

# milestone_gate_allows_task
# Purpose: Enforce sequential milestone dispatch for a task.
# Args:
#   $1: Task file path (string).
#   $2: Active milestone id (string, optional).
# Output: Prints a blocking reason when the task is outside the active milestone.
# Returns: 0 if task is eligible; 1 otherwise.
milestone_gate_allows_task() {
  local task_file="$1"
  local active_milestone="$2"
  if [[ -z "${active_milestone}" ]]; then
    return 0
  fi
  local task_name
  task_name="$(basename "${task_file}" .md)"
  if [[ "${task_name}" == 000-* ]]; then
    return 0
  fi
  local milestone
  milestone="$(frontmatter_value "${task_file}" "milestone")"
  if [[ -z "${milestone}" ]]; then
    return 0
  fi
  if [[ "${milestone}" == "${active_milestone}" ]]; then
    return 0
  fi
  printf 'Milestone %s is not active (current %s).' "${milestone}" "${active_milestone}"
  return 1
}

# extract_block_reason
# Purpose: Extract the block reason from a task file.
# Args:
#   $1: Task file path (string).
# Output: Prints the extracted reason or fallback text.
# Returns: 0 always.
extract_block_reason() {
  local file="$1"
  local reason
  reason="$(
    awk '
      /^## Governator Block/ {
        while (getline && $0 ~ /^[[:space:]]*$/) {}
        if ($0 != "") {
          print
          exit
        }
      }
    ' "${file}" 2> /dev/null
  )"
  if [[ -z "${reason}" ]]; then
    reason="$(
      awk '
        /^## Merge Failure/ {
          while (getline && $0 ~ /^[[:space:]]*$/) {}
          if ($0 != "") {
            print
            exit
          }
        }
      ' "${file}" 2> /dev/null
    )"
  fi
  if [[ -z "${reason}" ]]; then
    reason="reason unavailable"
  fi
  printf '%s\n' "${reason}"
}

# blocked_task_has_unblock_marker
# Purpose: Check if a blocked task already has an unblock note or analysis.
# Args:
#   $1: Task file path (string).
# Output: None.
# Returns: 0 if the task has an unblock marker; 1 otherwise.
blocked_task_has_unblock_marker() {
  local file="$1"
  if grep -Fq "## Unblock Note" "${file}" 2> /dev/null; then
    return 0
  fi
  if grep -Fq "## Unblock Analysis" "${file}" 2> /dev/null; then
    return 0
  fi
  return 1
}

# blocked_tasks_needing_unblock
# Purpose: List blocked tasks that have not been analyzed or unblocked yet.
# Args: None.
# Output: Prints matching task file paths to stdout.
# Returns: 0 on completion.
blocked_tasks_needing_unblock() {
  local task_file
  while IFS= read -r task_file; do
    if blocked_task_has_unblock_marker "${task_file}"; then
      continue
    fi
    printf '%s\n' "${task_file}"
  done < <(list_task_files_in_dir "${STATE_DIR}/task-blocked")
}

# find_task_files
# Purpose: Find task files across task-* directories by base name pattern.
# Args:
#   $1: Task base name pattern (string).
# Output: Prints matching file paths to stdout.
# Returns: 0 always.
find_task_files() {
  local pattern="$1"
  find "${STATE_DIR}" -maxdepth 2 -type f -path "${STATE_DIR}/task-*/${pattern}.md" \
    ! -path "${STATE_DIR}/task-archive/*" 2> /dev/null | sort
}

# task_exists
# Purpose: Check whether a task exists anywhere in task-* directories.
# Args:
#   $1: Task name (string).
# Output: None.
# Returns: 0 if task exists; 1 otherwise.
task_exists() {
  local task_name="$1"
  if find_task_files "${task_name}" | grep -q .; then
    return 0
  fi
  return 1
}

# task_file_for_name
# Purpose: Resolve a task name to its file path.
# Args:
#   $1: Task name (string).
# Output: Prints the first matching file path to stdout.
# Returns: 0 if found; 1 if not found.
task_file_for_name() {
  local task_name="$1"
  local matches=()
  while IFS= read -r path; do
    matches+=("${path}")
  done < <(find_task_files "${task_name}" || true)

  if [[ "${#matches[@]}" -eq 0 ]]; then
    return 1
  fi
  if [[ "${#matches[@]}" -gt 1 ]]; then
    log_warn "Multiple task files found for ${task_name}, using ${matches[0]}"
  fi
  printf '%s\n' "${matches[0]}"
}

# ensure_task_archive_dir
# Purpose: Ensure the task archive directory and keep file exist.
# Args: None.
# Output: None.
# Returns: 0 on completion.
ensure_task_archive_dir() {
  local archive_dir="${STATE_DIR}/task-archive"
  if [[ ! -d "${archive_dir}" ]]; then
    mkdir -p "${archive_dir}"
  fi
  if [[ ! -f "${archive_dir}/.keep" ]]; then
    touch "${archive_dir}/.keep"
  fi
}

# archive_done_system_tasks
# Purpose: Move done 000- tasks into the archive with a timestamp suffix.
# Args: None.
# Output: Logs task events and commits when moves occur.
# Returns: 0 on completion; 1 if git commit fails.
archive_done_system_tasks() {
  ensure_task_archive_dir

  local archive_dir="${STATE_DIR}/task-archive"
  local done_dir="${STATE_DIR}/task-done"
  local moved=0
  local task_file
  while IFS= read -r task_file; do
    local task_name
    task_name="$(basename "${task_file}" .md)"
    if [[ "${task_name}" != 000-* ]]; then
      continue
    fi
    local timestamp
    timestamp="$(date +%Y-%m-%d-%H-%M-%S)"
    local archived_name="${task_name}-${timestamp}"
    move_task_file "${task_file}" "${archive_dir}" "${task_name}" "archived to task-archive" "${archived_name}"
    moved=1
  done < <(list_task_files_in_dir "${done_dir}")

  if [[ "${moved}" -eq 1 ]]; then
    git -C "${ROOT_DIR}" add "${STATE_DIR}" "${AUDIT_LOG}"
    git -C "${ROOT_DIR}" commit -q -m "[governator] Archive system tasks"
  fi
}

# task_dir_for_branch
# Purpose: Determine the task directory for a task within a branch.
# Args:
#   $1: Branch name (string).
#   $2: Task name (string).
# Output: Prints the task directory name to stdout.
# Returns: 0 if found; 1 if missing.
task_dir_for_branch() {
  local branch="$1"
  local task_name="$2"
  local path
  path="$(
    git -C "${ROOT_DIR}" ls-tree -r --name-only "${branch}" "${STATE_DIR}" 2> /dev/null |
      awk -v task="${task_name}.md" '$0 ~ ("/" task "$") { print; exit }'
  )"
  if [[ -z "${path}" ]]; then
    return 1
  fi
  basename "$(dirname "${path}")"
}

# task_file_for_prefix
# Purpose: Resolve a unique task file matching a prefix.
# Args:
#   $1: Task name prefix (string).
# Output: Prints the matching file path to stdout.
# Returns: 0 if unique match; 1 otherwise.
task_file_for_prefix() {
  local prefix="$1"
  if [[ -z "${prefix}" ]]; then
    return 1
  fi
  local matches=()
  local path
  while IFS= read -r path; do
    matches+=("${path}")
  done < <(find_task_files "${prefix}*" || true)

  if [[ "${#matches[@]}" -eq 0 ]]; then
    return 1
  fi
  if [[ "${#matches[@]}" -gt 1 ]]; then
    log_error "Multiple task files match prefix ${prefix}; please be more specific."
    return 1
  fi
  printf '%s\n' "${matches[0]}"
}

# abort_task
# Purpose: Abort a task by killing its worker and blocking the task.
# Args:
#   $1: Task prefix (string).
# Output: Logs task state changes and cleanup actions.
# Returns: 0 on completion; exits 1 if task is not found.
abort_task() {
  local prefix="$1"
  if [[ -z "${prefix:-}" ]]; then
    log_error "Usage: abort <task-prefix>"
    exit 1
  fi

  local task_file
  if ! task_file="$(task_file_for_prefix "${prefix}")"; then
    log_error "No task matches prefix ${prefix}"
    exit 1
  fi

  local task_name
  task_name="$(basename "${task_file}" .md)"
  local worker
  if ! worker="$(extract_worker_from_task "${task_file}" 2> /dev/null)"; then
    worker=""
  fi

  local worker_info=()
  local pid=""
  local tmp_dir=""
  local branch=""
  if mapfile -t worker_info < <(worker_process_get "${task_name}" "${worker}" 2> /dev/null); then
    pid="${worker_info[0]:-}"
    tmp_dir="${worker_info[1]:-}"
    branch="${worker_info[2]:-}"
  fi
  local expected_branch="worker/${worker}/${task_name}"
  if [[ -z "${branch}" ]]; then
    branch="${expected_branch}"
  fi

  if [[ -n "${pid}" ]]; then
    if kill -0 "${pid}" > /dev/null 2>&1; then
      kill -9 "${pid}" > /dev/null 2>&1 || true
    fi
  fi

  if [[ -n "${tmp_dir}" && -d "${tmp_dir}" ]]; then
    cleanup_tmp_dir "${tmp_dir}"
  fi
  cleanup_worker_tmp_dirs "${worker}" "${task_name}"

  delete_worker_branch "${branch}"

  in_flight_remove "${task_name}" "${worker}"

  local blocked_dest="${STATE_DIR}/task-blocked/${task_name}.md"
  sync_default_branch
  if [[ "${task_file}" != "${blocked_dest}" ]]; then
    move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "aborted by operator"
  else
    log_task_event "${task_name}" "aborted by operator"
  fi

  local abort_meta
  abort_meta="Aborted by operator.
Worker: ${worker:-n/a}
PID: ${pid:-n/a}
Branch: ${branch:-n/a}"
  annotate_abort "${blocked_dest}" "${abort_meta}"
  annotate_blocked "${blocked_dest}" "Aborted by operator command."

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Abort task ${task_name}"
  git_push_default_branch
}

# unblock_task
# Purpose: Move a blocked task back to assigned with an operator note.
# Args:
#   $1: Task prefix (string).
#   $2+: Unblock note (string).
# Output: Logs state changes and appends the note to the task file.
# Returns: 0 on completion; exits 1 if task is not found or not blocked.
unblock_task() {
  local prefix="$1"
  shift || true
  if [[ -z "${prefix:-}" || -z "${1:-}" ]]; then
    log_error "Usage: unblock <task-prefix> <note>"
    exit 1
  fi
  local note="$*"

  local task_file
  if ! task_file="$(task_file_for_prefix "${prefix}")"; then
    log_error "No task matches prefix ${prefix}"
    exit 1
  fi

  local task_name
  task_name="$(basename "${task_file}" .md)"
  local task_dir
  task_dir="$(basename "$(dirname "${task_file}")")"
  if [[ "${task_dir}" != "task-blocked" ]]; then
    log_error "Task ${task_name} is not blocked; cannot unblock."
    exit 1
  fi

  local worker=""
  if worker="$(extract_worker_from_task "${task_file}" 2> /dev/null)"; then
    :
  else
    worker=""
  fi

  sync_default_branch
  local assigned_file="${STATE_DIR}/task-assigned/${task_name}.md"
  annotate_unblocked "${task_file}" "${note}"
  move_task_file "${task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "moved to task-assigned"

  in_flight_remove "${task_name}" ""
  if [[ -n "${worker}" ]]; then
    worker_process_clear "${task_name}" "${worker}"
    cleanup_worker_tmp_dirs "${worker}" "${task_name}"
  fi

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Unblock task ${task_name}"
  git_push_default_branch
}

# truncate_task_notes
# Purpose: Truncate a task file to the Notes section heading.
# Args:
#   $1: Task file path (string).
# Output: None.
# Returns: 0 on success.
truncate_task_notes() {
  local task_file="$1"
  if [[ -z "${task_file}" || ! -f "${task_file}" ]]; then
    return 0
  fi
  local tmp_file
  tmp_file="$(mktemp "${task_file}.tmp.XXXXXX")"
  awk '
    { print }
    $0 ~ /^## Notes[[:space:]]*$/ { exit }
  ' "${task_file}" > "${tmp_file}"
  mv "${tmp_file}" "${task_file}"
}

# restart_cleanup_worker
# Purpose: Stop a worker process and clean up artifacts for a task.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
# Output: None.
# Returns: 0 on completion.
restart_cleanup_worker() {
  local task_name="$1"
  local worker="$2"
  if [[ -z "${task_name}" || -z "${worker}" ]]; then
    return 0
  fi

  local worker_info=()
  local pid=""
  local tmp_dir=""
  local branch=""
  if mapfile -t worker_info < <(worker_process_get "${task_name}" "${worker}" 2> /dev/null); then
    pid="${worker_info[0]:-}"
    tmp_dir="${worker_info[1]:-}"
    branch="${worker_info[2]:-}"
  fi

  if [[ -n "${pid}" ]]; then
    if kill -0 "${pid}" > /dev/null 2>&1; then
      kill -9 "${pid}" > /dev/null 2>&1 || true
    fi
  fi

  if [[ -n "${tmp_dir}" && -d "${tmp_dir}" ]]; then
    cleanup_tmp_dir "${tmp_dir}"
  fi
  cleanup_worker_tmp_dirs "${worker}" "${task_name}"

  if [[ -z "${branch}" ]]; then
    branch="worker/${worker}/${task_name}"
  fi
  delete_worker_branch "${branch}"
}

# restart_cleanup_in_flight
# Purpose: Stop any in-flight workers for a task and clear tracking entries.
# Args:
#   $1: Task name (string).
# Output: None.
# Returns: 0 on completion.
restart_cleanup_in_flight() {
  local task_name="$1"
  if [[ -z "${task_name}" ]]; then
    return 0
  fi

  local workers=()
  local task=""
  local worker=""
  while IFS='|' read -r task worker; do
    if [[ "${task}" == "${task_name}" ]]; then
      workers+=("${worker}")
    fi
  done < <(in_flight_entries)

  for worker in "${workers[@]}"; do
    restart_cleanup_worker "${task_name}" "${worker}"
    in_flight_remove "${task_name}" "${worker}"
  done

  in_flight_remove "${task_name}" ""
}

# restart_tasks
# Purpose: Reset tasks to backlog and remove all notes/annotations.
# Args:
#   $@: Task prefixes (string) and optional --dry-run.
# Output: Logs task state changes and cleanup actions.
# Returns: 0 on completion; exits 1 on invalid input.
restart_tasks() {
  if [[ "$#" -lt 1 ]]; then
    log_error "Usage: restart [--dry-run] <task-prefix> [task-prefix ...]"
    exit 1
  fi

  local dry_run=0
  local prefixes=()
  local arg=""
  while [[ "$#" -gt 0 ]]; do
    arg="$1"
    case "${arg}" in
      --dry-run)
        dry_run=1
        ;;
      *)
        prefixes+=("${arg}")
        ;;
    esac
    shift
  done

  if [[ "${#prefixes[@]}" -eq 0 ]]; then
    log_error "Usage: restart [--dry-run] <task-prefix> [task-prefix ...]"
    exit 1
  fi

  local prefix
  for prefix in "${prefixes[@]}"; do
    if [[ ! "${prefix}" =~ ^[0-9]+$ ]]; then
      log_error "Invalid task prefix ${prefix}; must be numeric."
      exit 1
    fi
  done

  local task_files=()
  local task_names=()
  local task_dirs=()
  local -A seen=()
  for prefix in "${prefixes[@]}"; do
    local task_file
    if ! task_file="$(task_file_for_prefix "${prefix}")"; then
      log_error "No task matches prefix ${prefix}"
      exit 1
    fi
    local task_name
    task_name="$(basename "${task_file}" .md)"
    if [[ -n "${seen[${task_name}]+x}" ]]; then
      continue
    fi
    seen["${task_name}"]=1
    task_files+=("${task_file}")
    task_names+=("${task_name}")
    task_dirs+=("$(basename "$(dirname "${task_file}")")")
  done

  if [[ "${dry_run}" -eq 1 ]]; then
    log_info "Dry run: restart ${task_names[*]}"
  else
    sync_default_branch
  fi

  local idx=0
  local task_file=""
  local task_name=""
  local task_dir=""
  local dest_file=""
  for idx in "${!task_files[@]}"; do
    task_file="${task_files[${idx}]}"
    task_name="${task_names[${idx}]}"
    task_dir="${task_dirs[${idx}]}"

    if [[ "${dry_run}" -eq 1 ]]; then
      if in_flight_has_task "${task_name}"; then
        log_info "Dry run: would stop in-flight workers for ${task_name}"
      fi
    else
      restart_cleanup_in_flight "${task_name}"
    fi

    if [[ "${dry_run}" -eq 1 ]]; then
      log_info "Dry run: would move ${task_name} from ${task_dir} to task-backlog"
      log_info "Dry run: would truncate notes for ${task_name}"
      continue
    fi

    if [[ "${task_dir}" != "task-backlog" ]]; then
      move_task_file "${task_file}" "${STATE_DIR}/task-backlog" "${task_name}" "restarted by operator"
      dest_file="${STATE_DIR}/task-backlog/${task_name}.md"
    else
      log_task_event "${task_name}" "restarted by operator"
      dest_file="${task_file}"
    fi

    truncate_task_notes "${dest_file}"
  done

  if [[ "${dry_run}" -eq 1 ]]; then
    return 0
  fi

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Restart tasks ${task_names[*]}"
  git_push_default_branch
}

# list_available_workers
# Purpose: List available worker roles.
# Args: None.
# Output: Prints role names to stdout.
# Returns: 0 always.
list_available_workers() {
  local worker
  while IFS= read -r path; do
    worker="$(basename "${path}" .md)"
    printf '%s\n' "${worker}"
  done < <(find "${ROLES_DIR}" -maxdepth 1 -type f -name '*.md' | sort)
}

# role_exists
# Purpose: Check if a role exists.
# Args:
#   $1: Role name (string).
# Output: None.
# Returns: 0 if role file exists; 1 otherwise.
role_exists() {
  local role="$1"
  [[ -f "${ROLES_DIR}/${role}.md" ]]
}

# append_section
# Purpose: Append a timestamped section to a task file.
# Args:
#   $1: File path (string).
#   $2: Section title (string).
#   $3: Author label (string).
#   $4: Body text (string).
# Output: Writes to the file.
# Returns: 0 on success.
append_section() {
  local file="$1"
  local title="$2"
  local author="$3"
  local body="$4"
  local prefix
  prefix="$(timestamp_utc_seconds) [${author}]: "
  {
    printf '\n%s\n\n' "${title}"
    while IFS= read -r line; do
      printf '%s%s\n' "${prefix}" "${line}"
    done <<< "${body}"
  } >> "${file}"
}

# annotate_assignment
# Purpose: Append an assignment annotation to a task file.
# Args:
#   $1: Task file path (string).
#   $2: Worker name (string).
# Output: Writes to the task file.
# Returns: 0 on success.
annotate_assignment() {
  local task_file="$1"
  local worker="$2"
  append_section "${task_file}" "## Assignment" "governator" "Assigned to ${worker}."
}

# annotate_review
# Purpose: Append a review decision and comments to a task file.
# Args:
#   $1: Task file path (string).
#   $2: Decision string (string).
#   $3+: Review comments (strings).
# Output: Writes to the task file.
# Returns: 0 on success.
annotate_review() {
  local task_file="$1"
  local decision="$2"
  local comments=("$@")
  comments=("${comments[@]:2}")

  local body="Decision: ${decision}"
  if [[ "${#comments[@]}" -gt 0 ]]; then
    body+=$'\nComments:'
    local comment
    for comment in "${comments[@]}"; do
      body+=$'\n- '"${comment}"
    done
  fi
  append_section "${task_file}" "## Review Result" "reviewer" "${body}"
}

# annotate_blocked
# Purpose: Append a block reason to a task file.
# Args:
#   $1: Task file path (string).
#   $2: Block reason (string).
# Output: Writes to the task file.
# Returns: 0 on success.
annotate_blocked() {
  local task_file="$1"
  local reason="$2"
  append_section "${task_file}" "## Governator Block" "governator" "${reason}"
}

# annotate_unblocked
# Purpose: Append an unblock note to a task file.
# Args:
#   $1: Task file path (string).
#   $2: Unblock note (string).
# Output: Writes to the task file.
# Returns: 0 on success.
annotate_unblocked() {
  local task_file="$1"
  local note="$2"
  append_section "${task_file}" "## Unblock Note" "governator" "${note}"
}

# move_task_to_blocked
# Purpose: Move a task file to blocked, annotate the reason, and commit.
# Args:
#   $1: Task file path (string).
#   $2: Block reason (string).
# Output: Logs task state changes and updates git.
# Returns: 0 on success.
move_task_to_blocked() {
  local task_file="$1"
  local reason="$2"
  sync_default_branch
  local task_name
  task_name="$(basename "${task_file}" .md)"
  local blocked_file="${STATE_DIR}/task-blocked/${task_name}.md"
  move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
  annotate_blocked "${blocked_file}" "${reason}"
  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Block task ${task_name}"
  git_push_default_branch
}

# annotate_abort
# Purpose: Append an abort annotation to a task file.
# Args:
#   $1: Task file path (string).
#   $2: Abort metadata (string).
# Output: Writes to the task file.
# Returns: 0 on success.
annotate_abort() {
  local task_file="$1"
  local abort_metadata="$2"
  append_section "${task_file}" "## Abort" "governator" "${abort_metadata}"
}

# annotate_merge_failure
# Purpose: Append a merge failure annotation for reviewer visibility.
# Args:
#   $1: Task file path (string).
#   $2: Branch name (string).
# Output: Writes to the task file.
# Returns: 0 on success.
annotate_merge_failure() {
  local task_file="$1"
  local branch="$2"
  local base_branch
  base_branch="$(read_default_branch)"
  append_section "${task_file}" "## Merge Failure" "governator" "Unable to fast-forward merge ${branch} into ${base_branch}."
}

# move_task_file
# Purpose: Move a task file to a new queue and record an audit entry.
# Args:
#   $1: Task file path (string).
#   $2: Destination directory (string).
#   $3: Task name (string).
#   $4: Audit message (string).
#   $5: New base name (string, optional, without extension).
# Output: Logs the task event.
# Returns: 0 on success.
move_task_file() {
  local task_file="$1"
  local dest_dir="$2"
  local task_name="$3"
  local audit_message="$4"
  local new_name="${5:-}"
  local dest_name
  if [[ -n "${new_name}" ]]; then
    dest_name="${new_name}.md"
  else
    dest_name="$(basename "${task_file}")"
  fi
  mv "${task_file}" "${dest_dir}/${dest_name}"
  log_task_event "${task_name}" "${audit_message}"
}

# warn_if_task_template_incomplete
# Purpose: Warn when a task file is missing required template sections.
# Args:
#   $1: Task file path (string).
#   $2: Task name (string).
# Output: Logs warning when sections are missing.
# Returns: 0 always.
warn_if_task_template_incomplete() {
  local task_file="$1"
  local task_name="$2"
  if [[ "${task_name}" == 000-* ]]; then
    return 0
  fi

  local sections=(
    "## Objective"
    "## Context"
    "## Requirements"
    "## Non-Goals"
    "## Constraints"
    "## Acceptance Criteria"
  )
  local missing=()
  local section
  for section in "${sections[@]}"; do
    if ! grep -Fq "${section}" "${task_file}"; then
      missing+=("${section}")
    fi
  done
  if [[ "${#missing[@]}" -gt 0 ]]; then
    log_warn "Task ${task_name} missing template sections: ${missing[*]}"
  fi
}

# parse_task_metadata
# Purpose: Parse task filename into task name, short name, and role.
# Args:
#   $1: Task file path (string).
# Output: Prints task_name, short_name, and role, one per line.
# Returns: 0 if role suffix is present; 1 otherwise.
parse_task_metadata() {
  local task_file="$1"
  local task_name
  task_name="$(basename "${task_file}" .md)"

  local role="${task_name##*-}"
  if [[ -z "${role}" || "${role}" == "${task_name}" ]]; then
    return 1
  fi
  local short_name="${task_name%-"${role}"}"
  printf '%s\n' "${task_name}" "${short_name}" "${role}"
}

# extract_worker_from_task
# Purpose: Extract the worker role suffix from a task filename.
# Args:
#   $1: Task file path (string).
# Output: Prints role name to stdout.
# Returns: 0 if role extracted; 1 otherwise.
extract_worker_from_task() {
  local task_file="$1"
  local metadata_text
  if ! metadata_text="$(parse_task_metadata "${task_file}")"; then
    return 1
  fi
  local metadata=()
  mapfile -t metadata <<< "${metadata_text}"
  printf '%s' "${metadata[2]}"
}

# read_next_task_id
# Purpose: Read the next task id from config.json, with a default fallback.
# Args: None.
# Output: Prints the next task id to stdout.
# Returns: 0 always.
read_next_task_id() {
  ensure_db_dir
  local value
  value="$(config_json_read_value "next_task_id" "${DEFAULT_TASK_ID}")"
  if [[ -z "${value}" || ! "${value}" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "${DEFAULT_TASK_ID}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# write_next_task_id
# Purpose: Persist the next task id to config.json.
# Args:
#   $1: Task id value (string or integer).
# Output: None.
# Returns: 0 on success.
write_next_task_id() {
  local value="$1"
  ensure_db_dir
  config_json_write_value "next_task_id" "${value}" "number"
}

# format_task_id
# Purpose: Format a numeric task id as zero-padded 3 digits.
# Args:
#   $1: Task id value (string or integer).
# Output: Prints the formatted id to stdout.
# Returns: 0 always.
format_task_id() {
  local value="$1"
  printf '%03d' "${value}"
}

# allocate_task_id
# Purpose: Allocate the current task id and increment the stored value.
# Args: None.
# Output: Prints the allocated id to stdout.
# Returns: 0 always.
allocate_task_id() {
  local current
  current="$(read_next_task_id)"
  if ! [[ "${current}" =~ ^[0-9]+$ ]]; then
    log_warn "Invalid task id value '${current}', resetting to 1."
    current=1
  fi

  local next=$((current + 1))
  write_next_task_id "${next}"
  printf '%s\n' "${current}"
}
