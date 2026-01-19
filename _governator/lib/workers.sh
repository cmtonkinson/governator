# shellcheck shell=bash

# build_worker_command
# Purpose: Assemble the worker CLI command array and log string for a worker.
# Args:
#   $1: Role name (string).
#   $2: Prompt text (string).
# Output: Sets WORKER_COMMAND (array) and WORKER_COMMAND_LOG (string).
# Returns: 0 on success; returns 1 when provider/bin lookup fails.
build_worker_command() {
  local role="$1"
  local prompt="$2"
  local provider
  if ! provider="$(read_agent_provider "${role}")"; then
    return 1
  fi
  local reasoning
  reasoning="$(read_reasoning_effort "${role}")"
  local bin
  if ! bin="$(read_agent_provider_bin "${provider}")"; then
    return 1
  fi

  local args=()
  if mapfile -t args < <(read_agent_provider_args "${provider}"); then
    :
  fi

  local i
  for i in "${!args[@]}"; do
    args[i]="${args[i]//\{REASONING_EFFORT\}/${reasoning}}"
  done

  WORKER_COMMAND=("${bin}" "${args[@]}" "${prompt}")

  local log_parts=()
  local part
  for part in "${WORKER_COMMAND[@]}"; do
    local escaped
    escaped="$(escape_log_value "${part}")"
    log_parts+=("\"${escaped}\"")
  done
  WORKER_COMMAND_LOG="$(join_by " " "${log_parts[@]}")"
}

# run_worker_detached
# Purpose: Launch a worker in the background and return its PID.
# Args:
#   $1: Working directory (string).
#   $2: Prompt text (string).
#   $3: Log file path (string).
#   $4: Role name (string).
#   $5: Wrapper script path (string, optional).
# Output: Prints the spawned PID to stdout.
# Returns: 0 on success; returns 1 when worker command build or spawn fails.
run_worker_detached() {
  local dir="$1"
  local prompt="$2"
  local log_file="$3"
  local role="$4"
  local wrapper="${5:-}"

  # Use nohup to prevent worker exit from being tied to this process.
  if ! build_worker_command "${role}" "${prompt}"; then
    return 1
  fi
  (
    cd "${dir}"
    if [[ -n "${wrapper}" ]]; then
      log_verbose "Worker wrapper: ${wrapper}"
    fi
    log_verbose "Worker command: ${WORKER_COMMAND_LOG}"
    if [[ -n "${wrapper}" ]]; then
      nohup "${wrapper}" "${WORKER_COMMAND[@]}" >> "${log_file}" 2>&1 &
    else
      nohup "${WORKER_COMMAND[@]}" >> "${log_file}" 2>&1 &
    fi
    echo $!
  )
}

# resolve_worktree_git_dir
# Purpose: Resolve the absolute git dir path for a worktree.
# Args:
#   $1: Worktree directory (string).
# Output: Prints the absolute git dir path to stdout.
# Returns: 0 on success; 1 if git dir cannot be resolved.
resolve_worktree_git_dir() {
  local worktree_dir="$1"
  local git_dir
  if ! git_dir="$(git -C "${worktree_dir}" rev-parse --git-dir 2>/dev/null)"; then
    return 1
  fi
  if [[ "${git_dir}" != /* ]]; then
    git_dir="${worktree_dir}/${git_dir}"
  fi
  printf '%s\n' "${git_dir}"
}

# write_worker_env_wrapper
# Purpose: Write a wrapper script that pins git to the worker worktree (and is ignored by self-check status).
# Args:
#   $1: Worktree directory (string).
#   $2: Expected branch name (string).
# Output: Prints the wrapper path to stdout.
# Returns: 0 on success; 1 on failure.
write_worker_env_wrapper() {
  local worktree_dir="$1"
  local expected_branch="$2"
  local git_dir
  if ! git_dir="$(resolve_worktree_git_dir "${worktree_dir}")"; then
    return 1
  fi
  local default_branch
  default_branch="$(read_default_branch)"
  if [[ -z "${expected_branch}" ]]; then
    return 1
  fi
  local wrapper_dir="${worktree_dir}/_governator/${LOCAL_STATE_DIRNAME}"
  local wrapper="${wrapper_dir}/worker-env.sh"
  mkdir -p "${wrapper_dir}"
  cat > "${wrapper}" <<EOF
#!/usr/bin/env bash
set -euo pipefail
export GIT_DIR="${git_dir}"
export GIT_WORK_TREE="${worktree_dir}"
export GIT_INDEX_FILE="${git_dir}/index"
unset GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_OBJECT_DIRECTORY
export GIT_EDITOR=true
exit_code=0
"\$@" || exit_code=\$?
expected_branch="${expected_branch}"
current_branch="\$(git -C "${worktree_dir}" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
status="ok"
reason=""
if [[ -z "\${expected_branch}" ]]; then
  status="fail"
  reason="Missing expected branch name."
elif [[ "\${current_branch}" != "\${expected_branch}" ]]; then
  status="fail"
  reason="Current branch does not match expected branch."
elif ! git -C "${worktree_dir}" show-ref --verify --quiet "refs/heads/\${expected_branch}"; then
  status="fail"
  reason="Expected local branch is missing."
elif [[ -n "\$(git -C "${worktree_dir}" status --porcelain --untracked-files=normal -- . \\
  ":(exclude)_governator/${LOCAL_STATE_DIRNAME}/" \\
  ":(exclude).git-local" \\
  2>/dev/null)" ]]; then
  status="fail"
  reason="Worktree has uncommitted changes."
elif [[ "\$(git -C "${worktree_dir}" rev-list --count "${default_branch}..\${expected_branch}" 2>/dev/null || echo 0)" -eq 0 ]]; then
  status="fail"
  reason="No commits beyond base branch."
fi
mkdir -p "${worktree_dir}/_governator/${LOCAL_STATE_DIRNAME}"
jq -n --arg status "\${status}" \\
  --arg reason "\${reason}" \\
  --arg expected_branch "\${expected_branch}" \\
  --arg current_branch "\${current_branch}" \\
  --arg worktree_dir "${worktree_dir}" \\
  --argjson exit_code "\${exit_code}" \\
  '{status:\$status, reason:\$reason, expected_branch:\$expected_branch, current_branch:\$current_branch, worktree_dir:\$worktree_dir, exit_code:\$exit_code}' \\
  > "${worktree_dir}/_governator/${LOCAL_STATE_DIRNAME}/self-check.json"
exit "\${exit_code}"
EOF
  chmod +x "${wrapper}"
  printf '%s\n' "${wrapper}"
}

# worktree_has_uncommitted_changes
# Purpose: Check if a worktree has uncommitted changes.
# Args:
#   $1: Worktree directory (string).
# Output: None.
# Returns: 0 if changes exist; 1 otherwise.
worktree_has_uncommitted_changes() {
  local worktree_dir="$1"
  if [[ -z "${worktree_dir}" || ! -d "${worktree_dir}" ]]; then
    return 1
  fi
  if [[ ! -e "${worktree_dir}/.git" ]]; then
    return 1
  fi
  if ! git -C "${worktree_dir}" rev-parse --git-dir > /dev/null 2>&1; then
    return 1
  fi
  [[ -n "$(git -C "${worktree_dir}" status --porcelain --untracked-files=normal -- . \
    ":(exclude)_governator/${LOCAL_STATE_DIRNAME}/" \
    ":(exclude).git-local" \
    2>/dev/null || true)" ]]
}

# block_task_for_worker_failure
# Purpose: Block a task when a worker exits with a recoverable failure.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Worktree directory (string, optional).
#   $4: Reason text (string).
# Output: Logs warnings and task transitions.
# Returns: 0 on completion.
block_task_for_worker_failure() {
  local task_name="$1"
  local worker="$2"
  local worktree_dir="${3:-}"
  local reason="$4"
  log_task_warn "${task_name}" "${reason}"
  local task_file
  if task_file="$(task_file_for_name "${task_name}")"; then
    annotate_blocked "${task_file}" "${reason}"
    move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
    git -C "${ROOT_DIR}" add "${STATE_DIR}"
    git -C "${ROOT_DIR}" commit -q -m "[governator] Block task ${task_name} on dirty worktree"
    git_push_default_branch
  fi
  in_flight_remove "${task_name}" "${worker}"
  return 0
}

# read_worker_self_check
# Purpose: Read the worker self-check status and reason from the worktree.
# Args:
#   $1: Worktree directory (string).
# Output: Prints "status|reason" to stdout when available.
# Returns: 0 if a report was read; 1 otherwise.
read_worker_self_check() {
  local worktree_dir="$1"
  if [[ -z "${worktree_dir}" ]]; then
    return 1
  fi
  local report_path="${worktree_dir}/_governator/${LOCAL_STATE_DIRNAME}/self-check.json"
  if [[ ! -f "${report_path}" ]]; then
    return 1
  fi
  local status
  local reason
  status="$(jq -r '.status // empty' "${report_path}" 2>/dev/null)"
  reason="$(jq -r '.reason // empty' "${report_path}" 2>/dev/null)"
  if [[ -z "${status}" ]]; then
    return 1
  fi
  printf '%s|%s\n' "${status}" "${reason}"
}

# format_prompt_files
# Purpose: Join prompt file paths into a comma-separated string.
# Args:
#   $@: Prompt file paths (strings).
# Output: Prints the formatted list to stdout.
# Returns: 0 always.
format_prompt_files() {
  local result=""
  local item
  for item in "$@"; do
    if [[ -n "${result}" ]]; then
      result+=", "
    fi
    result+="${item}"
  done
  printf '%s' "${result}"
}

# build_worker_prompt
# Purpose: Build the full prompt string for a worker role.
# Args:
#   $1: Role name (string).
#   $2: Task relative path (string).
# Output: Prints the full prompt string to stdout.
# Returns: 0 always.
build_worker_prompt() {
  local role="$1"
  local task_relpath="$2"
  local prompt_files=()
  local provider
  provider="$(read_agent_provider "${role}")"
  if [[ "${provider}" != "codex" ]]; then
    local reasoning
    reasoning="$(read_reasoning_effort "${role}")"
    prompt_files+=("_governator/reasoning/${reasoning}.md")
  fi
  prompt_files+=("_governator/worker-contract.md")
  prompt_files+=("${ROLES_DIR#"${ROOT_DIR}/"}/${role}.md")
  prompt_files+=("_governator/custom-prompts/_global.md")
  prompt_files+=("_governator/custom-prompts/${role}.md")
  prompt_files+=("${task_relpath}")

  local prompt
  prompt="Read and follow the instructions in the following files, in this order: $(format_prompt_files "${prompt_files[@]}")."
  printf '%s' "${prompt}"
}


# list_worker_branches
# Purpose: List local worker branch refs.
# Args: None.
# Output: Prints branch names to stdout, one per line.
# Returns: 0 always.
list_worker_branches() {
  git -C "${ROOT_DIR}" for-each-ref --format='%(refname:short)' "refs/heads/worker/*/*" || true
}

# in_flight_entries
# Purpose: Read in-flight log entries as task|worker pairs.
# Args: None.
# Output: Prints "task|worker" lines to stdout.
# Returns: 0 always.
in_flight_entries() {
  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 0
  fi
  awk -F ' -> ' 'NF == 2 { print $1 "|" $2 }' "${IN_FLIGHT_LOG}"
}

# count_in_flight
# Purpose: Count in-flight tasks, optionally filtered by role.
# Args:
#   $1: Role name (string, optional).
# Output: Prints count to stdout.
# Returns: 0 always.
count_in_flight() {
  local role="${1:-}"
  local count=0
  local task
  local worker
  while IFS='|' read -r task worker; do
    if [[ -n "${role}" && "${worker}" != "${role}" ]]; then
      continue
    fi
    count=$((count + 1))
  done < <(in_flight_entries)
  printf '%s\n' "${count}"
}

# in_flight_add
# Purpose: Append a task/worker entry to the in-flight log.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
# Output: None.
# Returns: 0 on success.
in_flight_add() {
  local task_name="$1"
  local worker_name="$2"
  printf '%s -> %s\n' "${task_name}" "${worker_name}" >> "${IN_FLIGHT_LOG}"
}

# in_flight_remove
# Purpose: Remove a task/worker entry from the in-flight log.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string, optional).
# Output: None.
# Returns: 0 on success.
in_flight_remove() {
  local task_name="$1"
  local worker_name="$2"
  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 0
  fi

  local tmp_file
  tmp_file="$(filter_in_flight_log "${task_name}" "${worker_name}")"
  mv "${tmp_file}" "${IN_FLIGHT_LOG}"
  if [[ -n "${worker_name}" ]]; then
    worker_process_clear "${task_name}" "${worker_name}"
  fi
  retry_count_clear "${task_name}"
}

# in_flight_has_task
# Purpose: Check whether a task is already marked in-flight.
# Args:
#   $1: Task name (string).
# Output: None.
# Returns: 0 if task is in-flight; 1 otherwise.
in_flight_has_task() {
  local task_name="$1"
  local task
  local worker
  while IFS='|' read -r task worker; do
    if [[ "${task}" == "${task_name}" ]]; then
      return 0
    fi
  done < <(in_flight_entries)
  return 1
}

# in_flight_has_task_worker
# Purpose: Check whether a specific task/worker pair is in-flight.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
# Output: None.
# Returns: 0 if the task/worker pair is in-flight; 1 otherwise.
in_flight_has_task_worker() {
  local task_name="$1"
  local worker_name="$2"
  local task
  local worker
  while IFS='|' read -r task worker; do
    if [[ "${task}" == "${task_name}" && "${worker}" == "${worker_name}" ]]; then
      return 0
    fi
  done < <(in_flight_entries)
  return 1
}

# recover_reviewer_output
# Purpose: Commit reviewer output when review.json exists but no commit was made.
# Args:
#   $1: Task name (string).
#   $2: Worktree dir path (string).
# Output: None.
# Returns: 0 if a reviewer commit was created; 1 otherwise.
recover_reviewer_output() {
  local task_name="$1"
  local worktree_dir="$2"
  if [[ -z "${worktree_dir}" || ! -d "${worktree_dir}" ]]; then
    return 1
  fi
  if [[ ! -f "${worktree_dir}/review.json" ]]; then
    return 1
  fi
  if ! git -C "${worktree_dir}" rev-parse --git-dir > /dev/null 2>&1; then
    return 1
  fi

  local branch="worker/reviewer/${task_name}"
  git -C "${worktree_dir}" checkout -q "${branch}" > /dev/null 2>&1 || true
  git -C "${worktree_dir}" add "review.json"
  if git -C "${worktree_dir}" diff --cached --quiet; then
    return 1
  fi

  if [[ -z "$(git -C "${worktree_dir}" config user.email || true)" ]]; then
    local email
    email="$(git -C "${ROOT_DIR}" config user.email || true)"
    if [[ -n "${email}" ]]; then
      git -C "${worktree_dir}" config user.email "${email}"
    fi
  fi
  if [[ -z "$(git -C "${worktree_dir}" config user.name || true)" ]]; then
    local name
    name="$(git -C "${ROOT_DIR}" config user.name || true)"
    if [[ -n "${name}" ]]; then
      git -C "${worktree_dir}" config user.name "${name}"
    fi
  fi

  if ! git -C "${worktree_dir}" commit -q -m "Review ${task_name}" > /dev/null 2>&1; then
    return 1
  fi
  # With worktrees, the commit is directly on the local branch - no push needed
  return 0
}


# filter_worker_process_log
# Purpose: Create a filtered copy of the worker process log excluding a task/worker.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
# Output: Prints the path to a temp file containing filtered log content.
# Returns: 0 on success.
filter_worker_process_log() {
  local task_name="$1"
  local worker="$2"
  local tmp_file
  tmp_file="$(mktemp)"
  if [[ -f "${WORKER_PROCESSES_LOG}" ]]; then
    awk -v task="${task_name}" -v worker_name="${worker}" '
      $0 ~ / \\| / {
        split($0, parts, " \\| ")
        if (parts[1] == task && parts[2] == worker_name) next
      }
      { print }
    ' "${WORKER_PROCESSES_LOG}" > "${tmp_file}"
  fi
  printf '%s\n' "${tmp_file}"
}

# filter_retry_counts_log
# Purpose: Create a filtered copy of retry counts excluding a task.
# Args:
#   $1: Task name (string).
# Output: Prints the path to a temp file containing filtered log content.
# Returns: 0 on success.
filter_retry_counts_log() {
  local task_name="$1"
  local tmp_file
  tmp_file="$(mktemp)"
  if [[ -f "${RETRY_COUNTS_LOG}" ]]; then
    awk -v task="${task_name}" '
      $0 ~ / \\| / {
        split($0, parts, " \\| ")
        if (parts[1] == task) next
      }
      { print }
    ' "${RETRY_COUNTS_LOG}" > "${tmp_file}"
  fi
  printf '%s\n' "${tmp_file}"
}

# filter_in_flight_log
# Purpose: Create a filtered copy of in-flight entries excluding a task/worker.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string, optional).
# Output: Prints the path to a temp file containing filtered log content.
# Returns: 0 on success.
filter_in_flight_log() {
  local task_name="$1"
  local worker_name="${2:-}"
  local tmp_file
  tmp_file="$(mktemp)"
  if [[ -f "${IN_FLIGHT_LOG}" ]]; then
    if [[ -n "${worker_name}" ]]; then
      awk -v task="${task_name}" -v worker="${worker_name}" '
        $0 ~ / -> / {
          split($0, parts, " -> ")
          if (parts[1] == task && parts[2] == worker) next
        }
        { print }
      ' "${IN_FLIGHT_LOG}" > "${tmp_file}"
    else
      awk -v task="${task_name}" '
        $0 ~ / -> / {
          split($0, parts, " -> ")
          if (parts[1] == task) next
        }
        { print }
      ' "${IN_FLIGHT_LOG}" > "${tmp_file}"
    fi
  fi
  printf '%s\n' "${tmp_file}"
}

# worker_process_set
# Purpose: Record the worker process metadata for a task.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: PID (string or integer).
#   $4: Temp dir path (string).
#   $5: Branch name (string).
#   $6: Start timestamp (string or integer).
# Output: None.
# Returns: 0 on success.
worker_process_set() {
  local task_name="$1"
  local worker="$2"
  local pid="$3"
  local tmp_dir="$4"
  local branch="$5"
  local started_at="$6"

  local tmp_file
  tmp_file="$(filter_worker_process_log "${task_name}" "${worker}")"
  printf '%s | %s | %s | %s | %s | %s\n' "${task_name}" "${worker}" "${pid}" "${tmp_dir}" "${branch}" "${started_at}" >> "${tmp_file}"
  mv "${tmp_file}" "${WORKER_PROCESSES_LOG}"
}

# worker_process_clear
# Purpose: Remove a worker process record from the log.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
# Output: None.
# Returns: 0 on success.
worker_process_clear() {
  local task_name="$1"
  local worker="$2"

  if [[ ! -f "${WORKER_PROCESSES_LOG}" ]]; then
    return 0
  fi

  local tmp_file
  tmp_file="$(filter_worker_process_log "${task_name}" "${worker}")"
  mv "${tmp_file}" "${WORKER_PROCESSES_LOG}"
}

# worker_process_get
# Purpose: Lookup worker process metadata for a task and worker.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
# Output: Prints PID, temp dir, branch, and start timestamp (one per line).
# Returns: 0 if found; 1 if missing.
worker_process_get() {
  local task_name="$1"
  local worker="$2"

  if [[ ! -f "${WORKER_PROCESSES_LOG}" ]]; then
    return 1
  fi

  awk -v task="${task_name}" -v worker_name="${worker}" '
    $0 ~ / \\| / {
      split($0, parts, " \\| ")
      if (parts[1] == task && parts[2] == worker_name) {
        print parts[3]
        print parts[4]
        print parts[5]
        print parts[6]
        exit 0
      }
    }
    END { exit 1 }
  ' "${WORKER_PROCESSES_LOG}"
}


# retry_count_get
# Purpose: Read the retry count for a task.
# Args:
#   $1: Task name (string).
# Output: Prints the retry count to stdout.
# Returns: 0 always; defaults to 0 if missing/invalid.
retry_count_get() {
  local task_name="$1"
  if [[ ! -f "${RETRY_COUNTS_LOG}" ]]; then
    printf '0\n'
    return 0
  fi

  local count
  count="$(
    awk -v task="${task_name}" '
      $0 ~ / \\| / {
        split($0, parts, " \\| ")
        if (parts[1] == task) {
          print parts[2]
          exit 0
        }
      }
      END { exit 1 }
    ' "${RETRY_COUNTS_LOG}" || true
  )"

  if [[ -z "${count}" || ! "${count}" =~ ^[0-9]+$ ]]; then
    printf '0\n'
    return 0
  fi
  printf '%s\n' "${count}"
}

# retry_count_set
# Purpose: Write the retry count for a task.
# Args:
#   $1: Task name (string).
#   $2: Retry count (string or integer).
# Output: None.
# Returns: 0 on success.
retry_count_set() {
  local task_name="$1"
  local count="$2"

  local tmp_file
  tmp_file="$(filter_retry_counts_log "${task_name}")"
  printf '%s | %s\n' "${task_name}" "${count}" >> "${tmp_file}"
  mv "${tmp_file}" "${RETRY_COUNTS_LOG}"
}

# retry_count_clear
# Purpose: Remove the retry count entry for a task.
# Args:
#   $1: Task name (string).
# Output: None.
# Returns: 0 on success.
retry_count_clear() {
  local task_name="$1"
  if [[ ! -f "${RETRY_COUNTS_LOG}" ]]; then
    return 0
  fi

  local tmp_file
  tmp_file="$(filter_retry_counts_log "${task_name}")"
  mv "${tmp_file}" "${RETRY_COUNTS_LOG}"
}

# handle_zombie_failure
# Purpose: Retry or block a task when a worker fails to complete.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
# Output: Logs warnings and task transitions.
# Returns: 0 on completion; returns 1 when worktree creation fails.
handle_zombie_failure() {
  local task_name="$1"
  local worker="$2"

  remove_worktree "${task_name}" "${worker}"

  local retry_count
  retry_count="$(retry_count_get "${task_name}")"
  retry_count=$((retry_count + 1))
  retry_count_set "${task_name}" "${retry_count}"

  if [[ "${retry_count}" -ge 2 ]]; then
    local task_file
    if task_file="$(task_file_for_name "${task_name}")"; then
      annotate_blocked "${task_file}" "Worker exited before pushing branch twice; blocking task."
      move_task_file "${task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
      git -C "${ROOT_DIR}" add "${STATE_DIR}"
      git -C "${ROOT_DIR}" commit -q -m "[governator] Block task ${task_name} on retry failure"
      git_push_default_branch
    fi
    in_flight_remove "${task_name}" "${worker}"
    worker_process_clear "${task_name}" "${worker}"
    return 0
  fi

  local task_file
  if task_file="$(task_file_for_name "${task_name}")"; then
    spawn_worker_for_task "${task_file}" "${worker}" "retry started for ${worker}"
  fi
  return 0
}

# spawn_worker_for_task
# Purpose: Launch a worker for a task file and record metadata.
# Args:
#   $1: Task file path (string).
#   $2: Worker role (string).
#   $3: Audit log message (string, optional).
#   $4: Base ref to branch from (string, optional).
# Output: Logs task events and worker metadata.
# Returns: 0 on completion.
spawn_worker_for_task() {
  local task_file="$1"
  local worker="$2"
  local audit_message="$3"
  local base_ref="${4:-}"

  local task_name
  task_name="$(basename "${task_file}" .md)"

  # Create worktree directory for the worker
  local worktree_dir
  worktree_dir="$(worktree_path_for_task "${task_name}" "${worker}")"
  log_verbose "Worker worktree dir: ${worktree_dir}"

  local log_dir
  log_dir="${DB_DIR}/logs"
  mkdir -p "${log_dir}"
  local log_file
  if [[ "${worker}" == "reviewer" ]]; then
    log_file="${log_dir}/${task_name}-reviewer.log"
  else
    log_file="${log_dir}/${task_name}.log"
  fi
  append_worker_log_separator "${log_file}"

  # Set default base_ref to the default branch if not provided
  if [[ -z "${base_ref}" ]]; then
    base_ref="$(read_default_branch)"
  fi

  # Create the worktree with a new branch
  if ! create_worktree "${task_name}" "${worker}" "${base_ref}"; then
    log_task_warn "${task_name}" "failed to create worktree"
    return 1
  fi

  local task_relpath="${task_file#"${ROOT_DIR}/"}"
  local prompt
  prompt="$(build_worker_prompt "${worker}" "${task_relpath}")"

  if [[ "${worker}" == "reviewer" && -f "${TEMPLATES_DIR}/review.json" && ! -f "${worktree_dir}/review.json" ]]; then
    cp "${TEMPLATES_DIR}/review.json" "${worktree_dir}/review.json"
  fi

  local branch_name
  branch_name="$(worktree_branch_name "${task_name}" "${worker}")"
  local pid
  local started_at
  started_at="$(date +%s)"
  local wrapper
  if ! wrapper="$(write_worker_env_wrapper "${worktree_dir}" "${branch_name}")"; then
    log_task_warn "${task_name}" "failed to create worker git wrapper"
    return 1
  fi

  pid="$(run_worker_detached "${worktree_dir}" "${prompt}" "${log_file}" "${worker}" "${wrapper}")"
  if [[ -n "${pid}" ]]; then
    worker_process_set "${task_name}" "${worker}" "${pid}" "${worktree_dir}" "${branch_name}" "${started_at}"
    if [[ -n "${audit_message}" ]]; then
      log_task_event "${task_name}" "${audit_message}"
    fi
    log_task_event "${task_name}" "worker ${worker} started"
  else
    log_task_warn "${task_name}" "failed to capture worker pid"
  fi
}

# check_zombie_workers
# Purpose: Detect in-flight workers missing branches/commits and retry or block tasks.
# Args: None.
# Output: Logs warnings and task transitions.
# Returns: 0 on completion.
check_zombie_workers() {
  touch_logs

  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 0
  fi

  local task_name
  local worker
  while IFS='|' read -r task_name worker; do
    local branch="worker/${worker}/${task_name}"

    # Check if local branch exists and has commits (worker completed)
    if git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/heads/${branch}"; then
      # Branch exists - check if it has commits beyond the base
      if worktree_has_completed "${task_name}" "${worker}"; then
        continue
      fi
    fi

    local proc_info=()
    if ! mapfile -t proc_info < <(worker_process_get "${task_name}" "${worker}"); then
      log_task_warn "${task_name}" "missing worker process record for ${worker}; treating as zombie"
      handle_zombie_failure "${task_name}" "${worker}"
      continue
    fi

    local pid="${proc_info[0]:-}"
    local worktree_dir="${proc_info[1]:-}"
    local started_at="${proc_info[3]:-}"
    local timeout
    timeout="$(read_worker_timeout_seconds)"

    if [[ -n "${pid}" ]] && kill -0 "${pid}" > /dev/null 2>&1; then
      if [[ -n "${started_at}" && "${started_at}" =~ ^[0-9]+$ ]]; then
        local now
        now="$(date +%s)"
        local elapsed=$((now - started_at))
        if [[ "${elapsed}" -le "${timeout}" ]]; then
          continue
        fi
        log_task_warn "${task_name}" "worker ${worker} exceeded timeout (${elapsed}s)"
        kill -9 "${pid}" > /dev/null 2>&1 || true
      else
        continue
      fi
    fi

    if [[ "${worker}" == "reviewer" ]]; then
      if recover_reviewer_output "${task_name}" "${worktree_dir}"; then
        log_task_event "${task_name}" "recovered reviewer output"
        continue
      fi
    fi

    local self_check
    if self_check="$(read_worker_self_check "${worktree_dir}")"; then
      local self_status="${self_check%%|*}"
      local self_reason="${self_check#*|}"
      if [[ "${self_status}" != "ok" ]]; then
        local reason="Worker self-check failed: ${self_reason} Worktree preserved at ${worktree_dir}."
        block_task_for_worker_failure "${task_name}" "${worker}" "${worktree_dir}" "${reason}"
        continue
      fi
    fi

    if worktree_has_uncommitted_changes "${worktree_dir}"; then
      local reason="Worker exited without committing to the local branch; worktree has uncommitted changes. Worktree preserved at ${worktree_dir}."
      block_task_for_worker_failure "${task_name}" "${worker}" "${worktree_dir}" "${reason}"
      continue
    fi

    log_task_warn "${task_name}" "worker ${worker} exited without completing"

    handle_zombie_failure "${task_name}" "${worker}"
  done < <(in_flight_entries)
}
