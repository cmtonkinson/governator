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

# find_task_files
# Purpose: Find task files across task-* directories by base name pattern.
# Args:
#   $1: Task base name pattern (string).
# Output: Prints matching file paths to stdout.
# Returns: 0 always.
find_task_files() {
  local pattern="$1"
  find "${STATE_DIR}" -maxdepth 2 -type f -path "${STATE_DIR}/task-*/${pattern}.md" 2> /dev/null | sort
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

# list_available_workers
# Purpose: List available non-reviewer worker roles.
# Args: None.
# Output: Prints role names to stdout.
# Returns: 0 always.
list_available_workers() {
  local worker
  while IFS= read -r path; do
    worker="$(basename "${path}" .md)"
    if [[ "${worker}" == "reviewer" ]]; then
      continue
    fi
    printf '%s\n' "${worker}"
  done < <(find "${WORKER_ROLES_DIR}" -maxdepth 1 -type f -name '*.md' | sort)
}

# role_exists
# Purpose: Check if a worker role exists.
# Args:
#   $1: Role name (string).
# Output: None.
# Returns: 0 if role file exists; 1 otherwise.
role_exists() {
  local role="$1"
  [[ -f "${WORKER_ROLES_DIR}/${role}.md" ]]
}

# special_role_exists
# Purpose: Check if a special role exists.
# Args:
#   $1: Role name (string).
# Output: None.
# Returns: 0 if role file exists; 1 otherwise.
special_role_exists() {
  local role="$1"
  [[ -f "${SPECIAL_ROLES_DIR}/${role}.md" ]]
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

# annotate_feedback
# Purpose: Append feedback annotation to a task file.
# Args:
#   $1: Task file path (string).
# Output: Writes to the task file.
# Returns: 0 on success.
annotate_feedback() {
  local task_file="$1"
  append_section "${task_file}" "## Feedback" "governator" "Moved back to task-assigned for follow-up."
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

# block_task_from_backlog
# Purpose: Move a backlog task to blocked and record the reason.
# Args:
#   $1: Task file path (string).
#   $2: Block reason (string).
# Output: Logs and commits the state change.
# Returns: 0 on success.
block_task_from_backlog() {
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

# block_task_from_assigned
# Purpose: Move an assigned task to blocked and record the reason.
# Args:
#   $1: Task file path (string).
#   $2: Block reason (string).
# Output: Logs and commits the state change.
# Returns: 0 on success.
block_task_from_assigned() {
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
# Output: Logs the task event.
# Returns: 0 on success.
move_task_file() {
  local task_file="$1"
  local dest_dir="$2"
  local task_name="$3"
  local audit_message="$4"
  mv "${task_file}" "${dest_dir}/$(basename "${task_file}")"
  log_task_event "${task_name}" "${audit_message}"
}

# move_task_file_renamed
# Purpose: Move a task file to a new queue with a new name and audit entry.
# Args:
#   $1: Task file path (string).
#   $2: Destination directory (string).
#   $3: Task name (string).
#   $4: New base name (string, without extension).
#   $5: Audit message (string).
# Output: Logs the task event.
# Returns: 0 on success.
move_task_file_renamed() {
  local task_file="$1"
  local dest_dir="$2"
  local task_name="$3"
  local new_name="$4"
  local audit_message="$5"
  mv "${task_file}" "${dest_dir}/${new_name}.md"
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
# Purpose: Read the next task id from disk, with a default fallback.
# Args: None.
# Output: Prints the next task id to stdout.
# Returns: 0 always.
read_next_task_id() {
  ensure_db_dir
  if [[ ! -f "${NEXT_TASK_FILE}" ]]; then
    printf '%s\n' "${DEFAULT_TASK_ID}"
    return 0
  fi

  local value
  value="$(tr -d '[:space:]' < "${NEXT_TASK_FILE}")"
  if [[ -z "${value}" ]]; then
    printf '%s\n' "${DEFAULT_TASK_ID}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# write_next_task_id
# Purpose: Persist the next task id to disk.
# Args:
#   $1: Task id value (string or integer).
# Output: None.
# Returns: 0 on success.
write_next_task_id() {
  local value="$1"
  ensure_db_dir
  printf '%s\n' "${value}" > "${NEXT_TASK_FILE}"
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

# create_task_file
# Purpose: Create a new task file using the template and allocated id.
# Args:
#   $1: Short task name (string).
#   $2: Role suffix (string).
#   $3: Target directory path (string).
# Output: Prints the new task file path to stdout.
# Returns: 0 on success; 1 if the template is missing.
create_task_file() {
  local short_name="$1"
  local role="$2"
  local target_dir="$3"

  local task_id
  task_id="$(allocate_task_id)"

  local id_segment
  id_segment="$(format_task_id "${task_id}")"
  local filename="${id_segment}-${short_name}-${role}.md"

  local template="${TEMPLATES_DIR}/task.md"
  if [[ ! -f "${template}" ]]; then
    log_error "Missing task template at ${template}."
    return 1
  fi

  local dest="${target_dir}/${filename}"
  cp "${template}" "${dest}"
  printf '%s\n' "${dest}"
}
