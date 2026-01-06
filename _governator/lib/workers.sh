# shellcheck shell=bash

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

# Start the worker without blocking this script.
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

# Run the worker synchronously (blocking) for special roles.
run_codex_worker_blocking() {
  local dir="$1"
  local prompt="$2"
  local log_file="$3"
  local role="$4"
  if [[ -n "${CODEX_WORKER_CMD:-}" ]]; then
    log_verbose "Worker command: GOV_PROMPT=${prompt} bash -c ${CODEX_WORKER_CMD}"
    (cd "${dir}" && GOV_PROMPT="${prompt}" bash -c "${CODEX_WORKER_CMD}" >> "${log_file}" 2>&1)
    return $?
  fi

  build_codex_command "${role}" "${prompt}"
  log_verbose "Worker command: ${CODEX_COMMAND_LOG}"
  (cd "${dir}" && "${CODEX_COMMAND[@]}" >> "${log_file}" 2>&1)
}

# Run the reviewer synchronously so a review.json is produced.
run_codex_reviewer() {
  local dir="$1"
  local prompt="$2"
  local log_file="${3:-}"
  if [[ -n "${CODEX_REVIEW_CMD:-}" ]]; then
    log_verbose "Reviewer command: GOV_PROMPT=${prompt} bash -c ${CODEX_REVIEW_CMD}"
    if [[ -n "${log_file}" ]]; then
      (cd "${dir}" && GOV_PROMPT="${prompt}" bash -c "${CODEX_REVIEW_CMD}" >> "${log_file}" 2>&1)
    else
      (cd "${dir}" && GOV_PROMPT="${prompt}" bash -c "${CODEX_REVIEW_CMD}")
    fi
    return 0
  fi

  build_codex_command "reviewer" "${prompt}"
  log_verbose "Reviewer command: ${CODEX_COMMAND_LOG}"
  if [[ -n "${log_file}" ]]; then
    (cd "${dir}" && "${CODEX_COMMAND[@]}" >> "${log_file}" 2>&1)
  else
    (cd "${dir}" && "${CODEX_COMMAND[@]}")
  fi
}

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

build_worker_prompt() {
  local role="$1"
  local task_relpath="$2"
  local prompt_files=()
  prompt_files+=("_governator/worker-contract.md")
  prompt_files+=("_governator/roles-worker/${role}.md")
  prompt_files+=("_governator/custom-prompts/_global.md")
  prompt_files+=("_governator/custom-prompts/${role}.md")
  prompt_files+=("${task_relpath}")

  local prompt
  prompt="Read and follow the instructions in the following files, in this order: $(format_prompt_files "${prompt_files[@]}")."
  printf '%s' "${prompt}"
}

build_special_prompt() {
  local role="$1"
  local task_relpath="$2"
  local prompt_files=()
  prompt_files+=("_governator/worker-contract.md")
  prompt_files+=("${SPECIAL_ROLES_DIR#"${ROOT_DIR}/"}/${role}.md")
  prompt_files+=("_governator/custom-prompts/_global.md")
  prompt_files+=("_governator/custom-prompts/${role}.md")
  prompt_files+=("${task_relpath}")

  local prompt
  prompt="Read and follow the instructions in the following files, in this order: $(format_prompt_files "${prompt_files[@]}")."
  printf '%s' "${prompt}"
}

# List remote worker branches.
list_worker_branches() {
  local remote
  remote="$(read_remote_name)"
  git -C "${ROOT_DIR}" for-each-ref --format='%(refname:short)' "refs/remotes/${remote}/worker/*/*" || true
}

# Count in-flight tasks (all roles).
in_flight_entries() {
  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 0
  fi
  awk -F ' -> ' 'NF == 2 { print $1 "|" $2 }' "${IN_FLIGHT_LOG}"
}

count_in_flight_total() {
  local count=0
  local task
  local worker
  while IFS='|' read -r task worker; do
    count=$((count + 1))
  done < <(in_flight_entries)
  printf '%s\n' "${count}"
}

# Count in-flight tasks for a specific role.
count_in_flight_role() {
  local role="$1"
  local count=0
  local task
  local worker
  while IFS='|' read -r task worker; do
    if [[ "${worker}" == "${role}" ]]; then
      count=$((count + 1))
    fi
  done < <(in_flight_entries)
  printf '%s\n' "${count}"
}

# Add an in-flight record.
in_flight_add() {
  local task_name="$1"
  local worker_name="$2"
  printf '%s -> %s\n' "${task_name}" "${worker_name}" >> "${IN_FLIGHT_LOG}"
}

# Remove an in-flight record when a task completes or is blocked.
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

# Check whether a task is already in flight.
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

# Check whether a worker is already in flight.
in_flight_has_worker() {
  local worker_name="$1"
  local task
  local worker
  while IFS='|' read -r task worker; do
    if [[ "${worker}" == "${worker_name}" ]]; then
      return 0
    fi
  done < <(in_flight_entries)
  return 1
}

cleanup_tmp_dir() {
  local dir="$1"
  if [[ -n "${dir}" && -d "${dir}" ]]; then
    rm -rf "${dir}"
  fi
}

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

# Filter worker process log entries by task and worker.
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

# Filter retry count entries by task.
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

# Filter in-flight entries by task and optional worker.
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

# Record the worker process that owns a task.
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

# Remove a worker process record.
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

# Lookup a worker process record.
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

# Remove stale worker tmp dirs that are not tracked as active.
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

# Read the retry count for a task (defaults to 0).
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

# Write the retry count for a task.
retry_count_set() {
  local task_name="$1"
  local count="$2"

  local tmp_file
  tmp_file="$(filter_retry_counts_log "${task_name}")"
  printf '%s | %s\n' "${task_name}" "${count}" >> "${tmp_file}"
  mv "${tmp_file}" "${RETRY_COUNTS_LOG}"
}

# Clear the retry count for a task.
retry_count_clear() {
  local task_name="$1"
  if [[ ! -f "${RETRY_COUNTS_LOG}" ]]; then
    return 0
  fi

  local tmp_file
  tmp_file="$(filter_retry_counts_log "${task_name}")"
  mv "${tmp_file}" "${RETRY_COUNTS_LOG}"
}

# Spawn a worker for a task file with shared setup.
spawn_worker_for_task() {
  local task_file="$1"
  local worker="$2"
  local audit_message="$3"

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local tmp_dir
  tmp_dir="$(mktemp -d "/tmp/governator-${PROJECT_NAME}-${worker}-${task_name}-XXXXXX")"
  log_verbose "Worker tmp dir: ${tmp_dir}"

  local log_dir
  log_dir="${DB_DIR}/logs"
  mkdir -p "${log_dir}"
  local log_file
  log_file="${log_dir}/${task_name}.log"
  append_worker_log_separator "${log_file}"

  local remote
  local branch
  remote="$(read_remote_name)"
  branch="$(read_default_branch)"
  git clone "$(git -C "${ROOT_DIR}" remote get-url "${remote}")" "${tmp_dir}" > /dev/null 2>&1
  git -C "${tmp_dir}" checkout -b "worker/${worker}/${task_name}" "${remote}/${branch}" > /dev/null 2>&1

  local task_relpath="${task_file#"${ROOT_DIR}/"}"
  local prompt
  prompt="$(build_worker_prompt "${worker}" "${task_relpath}")"

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

spawn_special_worker_for_task() {
  local task_file="$1"
  local worker="$2"
  local audit_message="$3"

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
  git -C "${tmp_dir}" checkout -b "worker/${worker}/${task_name}" "${remote}/${branch}" > /dev/null 2>&1

  local task_relpath="${task_file#"${ROOT_DIR}/"}"
  local prompt
  prompt="$(build_special_prompt "${worker}" "${task_relpath}")"

  if [[ "${worker}" == "reviewer" && -f "${TEMPLATES_DIR}/review.json" ]]; then
    cp "${TEMPLATES_DIR}/review.json" "${tmp_dir}/review.json"
  fi

  local started_at
  started_at="$(date +%s)"
  if [[ -n "${audit_message}" ]]; then
    log_task_event "${task_name}" "${audit_message}"
  fi

  log_task_event "${task_name}" "starting special worker ${worker}"
  local worker_status=0
  if [[ "${worker}" == "reviewer" ]]; then
    if ! run_codex_reviewer "${tmp_dir}" "${prompt}" "${log_file}"; then
      worker_status=1
    fi
  elif ! run_codex_worker_blocking "${tmp_dir}" "${prompt}" "${log_file}" "${worker}"; then
    worker_status=1
  fi

  if [[ "${worker_status}" -eq 0 ]]; then
    log_task_event "${task_name}" "special worker ${worker} completed"
  else
    log_task_warn "${task_name}" "special worker ${worker} exited with error"
  fi

  if [[ "${worker}" == "reviewer" ]]; then
    local review_output=()
    mapfile -t review_output < <(read_reviewer_output "${tmp_dir}")
    local decision="${review_output[0]}"
    local review_lines=("${review_output[@]:1}")
    local block_reason="Unexpected task state during processing."
    git_checkout_default_branch
    apply_review_decision "${task_name}" "${worker}" "${decision}" "${block_reason}" "${review_lines[@]}"
    git_push_default_branch
    delete_worker_branch "worker/${worker}/${task_name}"
    rm -f "${tmp_dir}/review.json"
  elif [[ "${worker_status}" -eq 0 ]]; then
    process_special_worker_branch "${task_name}" "${worker}"
  fi
  local finished_at
  finished_at="$(date +%s)"
  if [[ "${finished_at}" -ge "${started_at}" ]]; then
    log_task_event "${task_name}" "worker elapsed ${worker}: $((finished_at - started_at))s"
  fi
  cleanup_tmp_dir "${tmp_dir}"
}

# Handle missing branches with dead workers.
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

    log_task_warn "${task_name}" "worker ${worker} exited before pushing branch"

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
  done < <(in_flight_entries)
}
