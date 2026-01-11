# shellcheck shell=bash

# chat_session_stamp
# Purpose: Generate a timestamp string for chat session filenames.
# Args: None.
# Output: Prints timestamp to stdout.
# Returns: 0 always.
chat_session_stamp() {
  date -u +"%Y%m%dT%H%M%SZ"
}

# ensure_chat_docs_dir
# Purpose: Ensure the chat docs directory exists.
# Args: None.
# Output: None.
# Returns: 0 on success.
ensure_chat_docs_dir() {
  if [[ ! -d "${CHAT_DOCS_DIR}" ]]; then
    mkdir -p "${CHAT_DOCS_DIR}"
  fi
}

# render_chat_template
# Purpose: Render a chat template with simple placeholders.
# Args:
#   $1: Template file path (string).
# Output: Prints rendered template to stdout.
# Returns: 0 on success; 1 if template missing.
render_chat_template() {
  local template_path="$1"
  if [[ ! -f "${template_path}" ]]; then
    log_error "Missing chat template at ${template_path}."
    return 1
  fi
  local content
  content="$(cat "${template_path}")"
  content="${content//\{\{PROJECT_NAME\}\}/${PROJECT_NAME}}"
  content="${content//\{\{ROOT_DIR\}\}/${ROOT_DIR}}"
  printf '%s' "${content}"
}

# render_chat_prompt
# Purpose: Render a chat prompt with transcript and summary placeholders.
# Args:
#   $1: Template file path (string).
#   $2: Transcript file path (string).
#   $3: Summary file path (string).
#   $4: GOVERNATOR.md path (string).
# Output: Prints rendered prompt to stdout.
# Returns: 0 on success; 1 if template missing.
render_chat_prompt() {
  local template_path="$1"
  local transcript_path="$2"
  local summary_path="$3"
  local governator_path="$4"
  local content
  if ! content="$(render_chat_template "${template_path}")"; then
    return 1
  fi
  content="${content//\{\{TRANSCRIPT_PATH\}\}/${transcript_path}}"
  content="${content//\{\{SUMMARY_PATH\}\}/${summary_path}}"
  content="${content//\{\{GOVERNATOR_PATH\}\}/${governator_path}}"
  printf '%s' "${content}"
}

# build_chat_command
# Purpose: Assemble the interactive chat CLI command array and log string.
# Args: None.
# Output: Sets CHAT_COMMAND (array) and CHAT_COMMAND_LOG (string).
# Returns: 0 on success; returns 1 when provider/bin lookup fails.
build_chat_command() {
  local provider
  if ! provider="$(read_agent_provider "chat")"; then
    return 1
  fi
  local bin
  if ! bin="$(read_agent_provider_bin "${provider}")"; then
    return 1
  fi

  local args=()
  if mapfile -t args < <(read_agent_provider_chat_args "${provider}"); then
    :
  fi

  CHAT_COMMAND=("${bin}" "${args[@]}")

  local log_parts=()
  local part
  for part in "${CHAT_COMMAND[@]}"; do
    local escaped
    escaped="$(escape_log_value "${part}")"
    log_parts+=("\"${escaped}\"")
  done
  CHAT_COMMAND_LOG="$(join_by " " "${log_parts[@]}")"
}

# run_chat_session
# Purpose: Run an interactive chat session under screen with transcript logging and prompt injection retries.
# Args:
#   $1: Session name (string).
#   $2: Prompt text to inject (string).
#   $3: Transcript log path (string).
# Output: Logs command and session info.
# Returns: 0 on completion; 1 on failure.
run_chat_session() {
  local session_name="$1"
  local prompt="$2"
  local transcript_path="$3"

  ensure_chat_docs_dir
  if ! build_chat_command; then
    return 1
  fi

  log_verbose "Chat command: ${CHAT_COMMAND_LOG}"
  if ! screen -dmS "${session_name}" -L -Logfile "${transcript_path}" "${CHAT_COMMAND[@]}"; then
    log_error "Failed to start chat session ${session_name}."
    return 1
  fi
  local attempt
  local sent=0
  for attempt in 1 2 3 4 5; do
    if screen -S "${session_name}" -X stuff "$(printf '%s\n' "${prompt}")"; then
      sent=1
      break
    fi
    sleep 0.1
  done
  if [[ "${sent}" -ne 1 ]]; then
    log_error "Failed to send prompt to chat session ${session_name}."
    return 1
  fi
  if ! screen -r "${session_name}"; then
    log_error "Chat session ${session_name} exited with error."
    return 1
  fi
}

# run_chat_completion
# Purpose: Run a non-interactive completion to generate chat artifacts.
# Args:
#   $1: Prompt text (string).
#   $2: Output file path (string).
# Output: Writes model output to the file.
# Returns: 0 on success; 1 on failure.
run_chat_completion() {
  local prompt="$1"
  local output_path="$2"

  if ! build_worker_command "generalist" "${prompt}"; then
    return 1
  fi
  if ! "${WORKER_COMMAND[@]}" > "${output_path}"; then
    log_error "Failed to generate chat output at ${output_path}."
    return 1
  fi
  return 0
}

# discovery_governator
# Purpose: Run the discovery clarification chat and generate GOVERNATOR.md.
# Args:
#   --refinement: Request refinement after planning.
# Output: Writes chat transcript, summary, and GOVERNATOR.md.
# Returns: 0 on completion; exits on failure.
discovery_governator() {
  ensure_ready_for_discovery

  local request_refinement=0
  local arg
  for arg in "$@"; do
    case "${arg}" in
      --refinement)
        request_refinement=1
        ;;
      *)
        log_error "Unknown discovery option: ${arg}"
        exit 1
        ;;
    esac
  done

  if [[ "${request_refinement}" -eq 1 ]]; then
    write_refinement_requested "true"
  fi

  local stamp
  stamp="$(chat_session_stamp)"
  local transcript_path="${CHAT_DOCS_DIR}/discovery-${stamp}.log"
  local summary_path="${CHAT_DOCS_DIR}/discovery-${stamp}-summary.md"
  local session_name="governator-discovery-${stamp}-$$"

  local chat_prompt
  if ! chat_prompt="$(render_chat_template "${DISCOVERY_CHAT_TEMPLATE}")"; then
    exit 1
  fi

  printf '%s\n' "What would you like to build?"
  run_chat_session "${session_name}" "${chat_prompt}" "${transcript_path}"

  local summary_prompt
  if ! summary_prompt="$(render_chat_prompt "${DISCOVERY_SUMMARY_TEMPLATE}" "${transcript_path}" "${summary_path}" "${ROOT_DIR}/GOVERNATOR.md")"; then
    exit 1
  fi
  run_chat_completion "${summary_prompt}" "${summary_path}"

  local governator_prompt
  if ! governator_prompt="$(render_chat_prompt "${DISCOVERY_GOV_TEMPLATE}" "${transcript_path}" "${summary_path}" "${ROOT_DIR}/GOVERNATOR.md")"; then
    exit 1
  fi
  run_chat_completion "${governator_prompt}" "${ROOT_DIR}/GOVERNATOR.md"

  git -C "${ROOT_DIR}" add "${ROOT_DIR}/GOVERNATOR.md" "${summary_path}" "${transcript_path}" "${CONFIG_FILE}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Capture discovery chat"
  git_push_default_branch
}

# refinement_governator
# Purpose: Run the refinement clarification chat and queue a review.
# Args: None.
# Output: Writes chat transcript and summary, queues review task.
# Returns: 0 on completion; exits on failure.
refinement_governator() {
  ensure_ready_with_lock

  if ! pipeline_state_is_set "epics_complete"; then
    log_error "Refinement requires epics to be complete."
    exit 1
  fi

  write_refinement_requested "true"

  local stamp
  stamp="$(chat_session_stamp)"
  local transcript_path="${CHAT_DOCS_DIR}/refinement-${stamp}.log"
  local summary_path="${CHAT_DOCS_DIR}/refinement-${stamp}-summary.md"
  local session_name="governator-refinement-${stamp}-$$"

  local chat_prompt
  if ! chat_prompt="$(render_chat_template "${REFINEMENT_CHAT_TEMPLATE}")"; then
    exit 1
  fi

  run_chat_session "${session_name}" "${chat_prompt}" "${transcript_path}"

  local summary_prompt
  if ! summary_prompt="$(render_chat_prompt "${REFINEMENT_SUMMARY_TEMPLATE}" "${transcript_path}" "${summary_path}" "${ROOT_DIR}/GOVERNATOR.md")"; then
    exit 1
  fi
  run_chat_completion "${summary_prompt}" "${summary_path}"

  create_refinement_review_task "${summary_path}" "${transcript_path}"

  git -C "${ROOT_DIR}" add "${summary_path}" "${transcript_path}" "${CONFIG_FILE}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Capture refinement chat"
  git_push_default_branch
}

# create_refinement_review_task
# Purpose: Create and dispatch the refinement review task.
# Args:
#   $1: Summary file path (string).
#   $2: Transcript file path (string).
# Output: Logs task creation and spawns the reviewer.
# Returns: 0 on completion; 1 on failure.
create_refinement_review_task() {
  local summary_path="$1"
  local transcript_path="$2"

  if task_exists "${REFINEMENT_REVIEW_TASK}"; then
    log_warn "Refinement review task already exists; skipping creation."
    return 0
  fi
  if [[ ! -f "${REFINEMENT_REVIEW_TEMPLATE}" ]]; then
    log_error "Missing refinement review template at ${REFINEMENT_REVIEW_TEMPLATE}."
    return 1
  fi
  if ! role_exists "${REFINEMENT_REVIEW_ROLE}"; then
    log_warn "Unknown role ${REFINEMENT_REVIEW_ROLE} for refinement review task."
    return 0
  fi

  local dest="${STATE_DIR}/task-assigned/${REFINEMENT_REVIEW_TASK}.md"
  cp "${REFINEMENT_REVIEW_TEMPLATE}" "${dest}"
  append_section "${dest}" "## Refinement Inputs" "governator" \
    "$(printf '%s\n%s\n' "Summary: ${summary_path}" "Transcript: ${transcript_path}")"
  annotate_assignment "${dest}" "${REFINEMENT_REVIEW_ROLE}"
  log_task_event "${REFINEMENT_REVIEW_TASK}" "created refinement review task"

  git -C "${ROOT_DIR}" add "${dest}" "${AUDIT_LOG}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Create refinement review task"
  git_push_default_branch

  in_flight_add "${REFINEMENT_REVIEW_TASK}" "${REFINEMENT_REVIEW_ROLE}"
  spawn_worker_for_task "${dest}" "${REFINEMENT_REVIEW_ROLE}" "reviewing refinement outputs"
  return 0
}
