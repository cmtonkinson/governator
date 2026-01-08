# shellcheck shell=bash

# build_codex_command
# Purpose: Assemble the Codex CLI command array and log string for a worker.
# Args:
#   $1: Role name (string).
#   $2: Prompt text (string).
# Output: Sets CODEX_COMMAND (array) and CODEX_COMMAND_LOG (string).
# Returns: 0 always.
build_codex_command() {
  local role="$1"
  local prompt="$2"
  local reasoning
  reasoning="$(read_reasoning_effort "${role}")"
  local escaped_prompt
  escaped_prompt="$(escape_log_value "${prompt}")"

  CODEX_COMMAND=(
    codex
    --full-auto
    --search
    -c sandbox_workspace_write.network_access=true
    -c model_reasoning_effort="${reasoning}"
    exec
    --sandbox=workspace-write
    "${prompt}"
  )
  CODEX_COMMAND_LOG="codex --full-auto --search -c sandbox_workspace_write.network_access=true -c model_reasoning_effort=\"${reasoning}\" exec --sandbox=workspace-write \"${escaped_prompt}\""
}

# run_codex_worker_detached
# Purpose: Launch a Codex worker in the background and return its PID.
# Args:
#   $1: Working directory (string).
#   $2: Prompt text (string).
#   $3: Log file path (string).
#   $4: Role name (string).
# Output: Prints the spawned PID to stdout.
# Returns: 0 on success; propagates errors from child command.
run_codex_worker_detached() {
  local dir="$1"
  local prompt="$2"
  local log_file="$3"
  local role="$4"
  if [[ -n "${CODEX_WORKER_CMD:-}" ]]; then
    log_verbose "Worker command: GOV_PROMPT=${prompt} nohup bash -c ${CODEX_WORKER_CMD}"
    (
      cd "${dir}"
      GOV_PROMPT="${prompt}" nohup bash -c "${CODEX_WORKER_CMD}" >> "${log_file}" 2>&1 &
      echo $!
    )
    return 0
  fi

  # Use nohup to prevent worker exit from being tied to this process.
  build_codex_command "${role}" "${prompt}"
  (
    cd "${dir}"
    log_verbose "Worker command: ${CODEX_COMMAND_LOG}"
    nohup "${CODEX_COMMAND[@]}" >> "${log_file}" 2>&1 &
    echo $!
  )
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
# Purpose: List remote worker branch refs.
# Args: None.
# Output: Prints branch refs to stdout, one per line.
# Returns: 0 always.
list_worker_branches() {
  local remote
  remote="$(read_remote_name)"
  git -C "${ROOT_DIR}" for-each-ref --format='%(refname:short)' "refs/remotes/${remote}/worker/*/*" || true
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
# Purpose: Commit and push reviewer output when review.json exists but no branch was pushed.
# Args:
#   $1: Task name (string).
#   $2: Temp dir path (string).
# Output: None.
# Returns: 0 if a reviewer branch was created and pushed; 1 otherwise.
recover_reviewer_output() {
  local task_name="$1"
  local tmp_dir="$2"
  if [[ -z "${tmp_dir}" || ! -d "${tmp_dir}" ]]; then
    return 1
  fi
  if [[ ! -f "${tmp_dir}/review.json" ]]; then
    return 1
  fi
  if ! git -C "${tmp_dir}" rev-parse --git-dir > /dev/null 2>&1; then
    return 1
  fi

  local branch="worker/reviewer/${task_name}"
  git -C "${tmp_dir}" checkout -q "${branch}" > /dev/null 2>&1 || true
  git -C "${tmp_dir}" add "review.json"
  if git -C "${tmp_dir}" diff --cached --quiet; then
    return 1
  fi

  if [[ -z "$(git -C "${tmp_dir}" config user.email || true)" ]]; then
    local email
    email="$(git -C "${ROOT_DIR}" config user.email || true)"
    if [[ -n "${email}" ]]; then
      git -C "${tmp_dir}" config user.email "${email}"
    fi
  fi
  if [[ -z "$(git -C "${tmp_dir}" config user.name || true)" ]]; then
    local name
    name="$(git -C "${ROOT_DIR}" config user.name || true)"
    if [[ -n "${name}" ]]; then
      git -C "${tmp_dir}" config user.name "${name}"
    fi
  fi

  if ! git -C "${tmp_dir}" commit -q -m "Review ${task_name}" > /dev/null 2>&1; then
    return 1
  fi
  if ! git -C "${tmp_dir}" push -q origin "${branch}" > /dev/null 2>&1; then
    return 1
  fi
  return 0
}

# cleanup_tmp_dir
# Purpose: Remove a worker temporary directory if it exists.
# Args:
#   $1: Directory path (string).
# Output: None.
# Returns: 0 on completion.
cleanup_tmp_dir() {
  local dir="$1"
  if [[ -n "${dir}" && -d "${dir}" ]]; then
    rm -rf "${dir}"
  fi
}

# cleanup_worker_tmp_dirs
# Purpose: Remove worker temp dirs from known tmp roots.
# Args:
#   $1: Worker name (string).
#   $2: Task name (string).
# Output: None.
# Returns: 0 on completion.
cleanup_worker_tmp_dirs() {
  local worker="$1"
  local task_name="$2"
  if [[ -z "${worker}" || -z "${task_name}" ]]; then
    return 0
  fi
  local roots=(/tmp)
  if [[ -d "/private/tmp" ]]; then
    roots+=(/private/tmp)
  fi

  local root
  for root in "${roots[@]}"; do
    find "${root}" -maxdepth 1 -type d -name "governator-${PROJECT_NAME}-${worker}-${task_name}-*" -exec rm -rf {} + > /dev/null 2>&1 || true
  done
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

# cleanup_stale_worker_dirs
# Purpose: Remove stale worker temp directories not tracked as active.
# Args:
#   $1: Optional "--dry-run" to list candidates without deleting.
# Output: Prints stale directories when running in dry-run mode.
# Returns: 0 on completion.
cleanup_stale_worker_dirs() {
  local tmp_root="/tmp"
  if [[ -d "/private/tmp" ]]; then
    tmp_root="/private/tmp"
  fi

  local dry_run="${1:-}"
  local timeout
  timeout="$(read_worker_timeout_seconds)"
  local now
  now="$(date +%s)"

  local active_dirs=()
  if [[ -f "${WORKER_PROCESSES_LOG}" ]]; then
    while IFS=' | ' read -r task_name worker pid tmp_dir branch started_at; do
      if [[ -n "${tmp_dir}" ]]; then
        active_dirs+=("$(normalize_tmp_path "${tmp_dir}")")
      fi
    done < "${WORKER_PROCESSES_LOG}"
  fi

  local dir
  while IFS= read -r dir; do
    local normalized
    normalized="$(normalize_tmp_path "${dir}")"
    local active=0
    local candidate
    for candidate in "${active_dirs[@]}"; do
      if [[ "${candidate}" == "${normalized}" ]]; then
        active=1
        break
      fi
    done
    if [[ "${active}" -eq 1 ]]; then
      continue
    fi

    local mtime
    mtime="$(file_mtime_epoch "${dir}")"
    if [[ -z "${mtime}" || ! "${mtime}" =~ ^[0-9]+$ ]]; then
      continue
    fi
    local age=$((now - mtime))
    if [[ "${age}" -ge "${timeout}" ]]; then
      if [[ "${dry_run}" == "--dry-run" ]]; then
        printf '%s\n' "${dir}"
      else
        cleanup_tmp_dir "${dir}"
      fi
    fi
  done < <(find "${tmp_root}" -maxdepth 1 -type d -name "governator-${PROJECT_NAME}-*" 2> /dev/null)
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
# Purpose: Retry or block a task when a worker fails to push a branch.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Temp dir path (string, optional).
# Output: Logs warnings and task transitions.
# Returns: 0 on completion.
handle_zombie_failure() {
  local task_name="$1"
  local worker="$2"
  local tmp_dir="${3:-}"

  cleanup_tmp_dir "${tmp_dir}"

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

  local tmp_dir
  tmp_dir="$(mktemp -d "/tmp/governator-${PROJECT_NAME}-${worker}-${task_name}-XXXXXX")"
  log_verbose "Worker tmp dir: ${tmp_dir}"

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

  local remote
  local branch
  remote="$(read_remote_name)"
  branch="$(read_default_branch)"
  git clone "$(git -C "${ROOT_DIR}" remote get-url "${remote}")" "${tmp_dir}" > /dev/null 2>&1
  if [[ -z "${base_ref}" ]]; then
    base_ref="${remote}/${branch}"
  fi
  git -C "${tmp_dir}" checkout -b "worker/${worker}/${task_name}" "${base_ref}" > /dev/null 2>&1

  local task_relpath="${task_file#"${ROOT_DIR}/"}"
  local prompt
  prompt="$(build_worker_prompt "${worker}" "${task_relpath}")"

  if [[ "${worker}" == "reviewer" && -f "${TEMPLATES_DIR}/review.json" && ! -f "${tmp_dir}/review.json" ]]; then
    cp "${TEMPLATES_DIR}/review.json" "${tmp_dir}/review.json"
  fi

  local branch_name="worker/${worker}/${task_name}"
  local pid
  local started_at
  started_at="$(date +%s)"
  pid="$(run_codex_worker_detached "${tmp_dir}" "${prompt}" "${log_file}" "${worker}")"
  if [[ -n "${pid}" ]]; then
    worker_process_set "${task_name}" "${worker}" "${pid}" "${tmp_dir}" "${branch_name}" "${started_at}"
    if [[ -n "${audit_message}" ]]; then
      log_task_event "${task_name}" "${audit_message}"
    fi
    log_task_event "${task_name}" "worker ${worker} started"
  else
    log_task_warn "${task_name}" "failed to capture worker pid"
  fi
}

# check_zombie_workers
# Purpose: Detect in-flight workers missing branches and retry or block tasks across all entries.
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

    local remote
    remote="$(read_remote_name)"
    if git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/remotes/${remote}/${branch}"; then
      continue
    fi

    local proc_info=()
    if ! mapfile -t proc_info < <(worker_process_get "${task_name}" "${worker}"); then
      log_task_warn "${task_name}" "missing worker process record for ${worker}; treating as zombie"
      handle_zombie_failure "${task_name}" "${worker}"
      continue
    fi

    local pid="${proc_info[0]:-}"
    local tmp_dir="${proc_info[1]:-}"
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
      if recover_reviewer_output "${task_name}" "${tmp_dir}"; then
        log_task_event "${task_name}" "recovered reviewer output and pushed branch"
        git_fetch_remote
        continue
      fi
    fi

    log_task_warn "${task_name}" "worker ${worker} exited before pushing branch"

    handle_zombie_failure "${task_name}" "${worker}" "${tmp_dir}"
  done < <(in_flight_entries)
}
