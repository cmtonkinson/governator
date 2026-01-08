# shellcheck shell=bash

# bootstrap_template_path
# Purpose: Resolve the bootstrap task template based on project mode.
# Args: None.
# Output: Prints template path to stdout.
# Returns: 0 always.
bootstrap_template_path() {
  local mode
  if ! mode="$(read_project_mode)"; then
    printf '%s\n' "${BOOTSTRAP_NEW_TEMPLATE}"
    return 0
  fi
  if [[ "${mode}" == "existing" ]]; then
    printf '%s\n' "${BOOTSTRAP_EXISTING_TEMPLATE}"
    return 0
  fi
  printf '%s\n' "${BOOTSTRAP_NEW_TEMPLATE}"
}

# bootstrap_required_artifacts
# Purpose: List required bootstrap artifacts based on project mode.
# Args: None.
# Output: Prints artifact filenames to stdout, one per line.
# Returns: 0 always.
bootstrap_required_artifacts() {
  local mode
  if ! mode="$(read_project_mode)"; then
    printf '%s\n' "${BOOTSTRAP_NEW_REQUIRED_ARTIFACTS[@]}"
    return 0
  fi
  if [[ "${mode}" == "existing" ]]; then
    printf '%s\n' "${BOOTSTRAP_EXISTING_REQUIRED_ARTIFACTS[@]}"
    return 0
  fi
  printf '%s\n' "${BOOTSTRAP_NEW_REQUIRED_ARTIFACTS[@]}"
}

# bootstrap_optional_artifacts
# Purpose: List optional bootstrap artifacts based on project mode.
# Args: None.
# Output: Prints artifact filenames to stdout, one per line.
# Returns: 0 always.
bootstrap_optional_artifacts() {
  local mode
  if ! mode="$(read_project_mode)"; then
    printf '%s\n' "${BOOTSTRAP_NEW_OPTIONAL_ARTIFACTS[@]}"
    return 0
  fi
  if [[ "${mode}" == "existing" ]]; then
    printf '%s\n' "${BOOTSTRAP_EXISTING_OPTIONAL_ARTIFACTS[@]}"
    return 0
  fi
  printf '%s\n' "${BOOTSTRAP_NEW_OPTIONAL_ARTIFACTS[@]}"
}

# bootstrap_task_path
# Purpose: Locate the bootstrap task file path.
# Args: None.
# Output: Prints task file path to stdout.
# Returns: 0 if found; 1 if missing.
bootstrap_task_path() {
  local path
  while IFS= read -r path; do
    if [[ -n "${path}" ]]; then
      printf '%s\n' "${path}"
      return 0
    fi
  done < <(find_task_files "${BOOTSTRAP_TASK_NAME}" || true)

  local archived
  archived="$(
    find "${STATE_DIR}/task-archive" -maxdepth 1 -type f \
      -name "${BOOTSTRAP_TASK_NAME}-*.md" 2> /dev/null | sort | tail -n 1
  )"
  if [[ -n "${archived}" ]]; then
    printf '%s\n' "${archived}"
    return 0
  fi
  return 1
}

# bootstrap_task_dir
# Purpose: Resolve the task directory containing the bootstrap task.
# Args: None.
# Output: Prints directory name to stdout.
# Returns: 0 if found; 1 otherwise.
bootstrap_task_dir() {
  local task_file
  if ! task_file="$(bootstrap_task_path)"; then
    return 1
  fi
  basename "$(dirname "${task_file}")"
}

# ensure_bootstrap_task_exists
# Purpose: Create the bootstrap task if it is missing.
# Args: None.
# Output: Logs task creation and commits changes.
# Returns: 0 on success; 1 on failure.
ensure_bootstrap_task_exists() {
  if bootstrap_task_path > /dev/null 2>&1; then
    return 0
  fi
  local template
  template="$(bootstrap_template_path)"
  if [[ ! -f "${template}" ]]; then
    log_error "Missing bootstrap template at ${template}."
    return 1
  fi

  local dest="${STATE_DIR}/task-backlog/${BOOTSTRAP_TASK_NAME}.md"
  cp "${template}" "${dest}"
  log_task_event "${BOOTSTRAP_TASK_NAME}" "created bootstrap task"
  git -C "${ROOT_DIR}" add "${dest}" "${AUDIT_LOG}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Create architecture bootstrap task"
  git_push_default_branch
}

# completion_check_due
# Purpose: Determine whether the completion-check cooldown has elapsed.
# Args: None.
# Output: None.
# Returns: 0 if due; 1 otherwise.
completion_check_due() {
  local last_run
  last_run="$(read_completion_check_last_run)"
  local cooldown
  cooldown="$(read_completion_check_cooldown_seconds)"
  local now
  now="$(date +%s)"
  if [[ "${last_run}" -eq 0 ]]; then
    return 0
  fi
  if [[ $((now - last_run)) -ge "${cooldown}" ]]; then
    return 0
  fi
  return 1
}

# completion_check_hash_mismatch
# Purpose: Determine whether the stored planning hash differs from GOVERNATOR.md.
# Args: None.
# Output: None.
# Returns: 0 if mismatch or missing; 1 if up-to-date.
completion_check_hash_mismatch() {
  local gov_sha
  gov_sha="$(governator_doc_sha)"
  if [[ -z "${gov_sha}" ]]; then
    return 0
  fi
  local done_sha
  done_sha="$(read_planning_gov_sha)"
  if [[ "${done_sha}" != "${gov_sha}" ]]; then
    return 0
  fi
  return 1
}

# planning_hash_mismatch
# Purpose: Determine whether the stored planning hash differs from GOVERNATOR.md.
# Args: None.
# Output: None.
# Returns: 0 if mismatch or missing; 1 if up-to-date.
planning_hash_mismatch() {
  local gov_sha
  gov_sha="$(governator_doc_sha)"
  if [[ -z "${gov_sha}" ]]; then
    return 0
  fi
  local planning_sha
  planning_sha="$(read_planning_gov_sha)"
  if [[ "${planning_sha}" != "${gov_sha}" ]]; then
    return 0
  fi
  return 1
}

# completion_check_needed
# Purpose: Determine whether a completion-check should run based on hash and cooldown.
# Args: None.
# Output: None.
# Returns: 0 if needed; 1 otherwise.
completion_check_needed() {
  if ! completion_check_hash_mismatch; then
    return 1
  fi
  if ! completion_check_due; then
    return 1
  fi
  return 0
}

# create_completion_check_task
# Purpose: Create the reviewer completion-check task when needed.
# Args: None.
# Output: Logs creation and commits changes.
# Returns: 0 on success; 1 on failure.
create_completion_check_task() {
  if task_exists "${COMPLETION_CHECK_REVIEW_TASK}" || task_exists "${GAP_ANALYSIS_PLANNER_TASK}"; then
    return 0
  fi

  if [[ ! -f "${COMPLETION_CHECK_REVIEW_TEMPLATE}" ]]; then
    log_error "Missing completion-check template at ${COMPLETION_CHECK_REVIEW_TEMPLATE}."
    return 1
  fi

  local dest="${STATE_DIR}/task-assigned/${COMPLETION_CHECK_REVIEW_TASK}.md"
  cp "${COMPLETION_CHECK_REVIEW_TEMPLATE}" "${dest}"
  annotate_assignment "${dest}" "${COMPLETION_CHECK_REVIEW_ROLE}"
  log_task_event "${COMPLETION_CHECK_REVIEW_TASK}" "created completion check task"

  write_completion_check_last_run "$(date +%s)"

  git -C "${ROOT_DIR}" add "${dest}" "${AUDIT_LOG}" "${CONFIG_FILE}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Create completion check task"
  git_push_default_branch
}

# move_completion_check_to_gap_analysis
# Purpose: Move reviewer completion-check output into a gap-analysis follow-up task.
# Args:
#   $1: Task file path (string).
#   $2: Task name (string).
# Output: Logs task transitions.
# Returns: 0 on success; 1 on failure.
move_completion_check_to_gap_analysis() {
  local task_file="$1"
  local task_name="$2"
  local dest="${STATE_DIR}/task-assigned/${GAP_ANALYSIS_PLANNER_TASK}.md"
  if [[ ! -f "${GAP_ANALYSIS_PLANNER_TEMPLATE}" ]]; then
    log_error "Missing gap-analysis template at ${GAP_ANALYSIS_PLANNER_TEMPLATE}."
    return 1
  fi
  cp "${GAP_ANALYSIS_PLANNER_TEMPLATE}" "${dest}"
  append_section "${dest}" "## Reviewer Notes" "reviewer" "$(cat "${task_file}")"
  move_task_file "${task_file}" "${STATE_DIR}/task-done" "${task_name}" "moved to task-done"
  log_task_event "${GAP_ANALYSIS_PLANNER_TASK}" "created gap-analysis follow-up"
}

# artifact_present
# Purpose: Check whether a bootstrap artifact exists and is non-empty.
# Args:
#   $1: Artifact filename (string).
# Output: None.
# Returns: 0 if present and non-empty; 1 otherwise.
artifact_present() {
  local file="$1"
  [[ -f "${BOOTSTRAP_DOCS_DIR}/${file}" && -s "${BOOTSTRAP_DOCS_DIR}/${file}" ]]
}

# artifact_skipped_in_task
# Purpose: Determine whether an artifact is explicitly skipped in a task file.
# Args:
#   $1: Task file path (string).
#   $2: Artifact filename (string).
# Output: None.
# Returns: 0 if skipped; 1 otherwise.
artifact_skipped_in_task() {
  local task_file="$1"
  local artifact="$2"
  if [[ ! -f "${task_file}" ]]; then
    return 1
  fi
  local base="${artifact%.md}"
  grep -Eiq "(skip|omit|n/a|not needed).*${base}|${base}.*(skip|omit|n/a|not needed)" "${task_file}"
}

# bootstrap_required_artifacts_ok
# Purpose: Verify all required bootstrap artifacts exist.
# Args: None.
# Output: None.
# Returns: 0 if all required artifacts exist; 1 otherwise.
bootstrap_required_artifacts_ok() {
  local artifacts=()
  mapfile -t artifacts < <(bootstrap_required_artifacts)
  local artifact
  for artifact in "${artifacts[@]}"; do
    if ! artifact_present "${artifact}"; then
      return 1
    fi
  done
  return 0
}

# bootstrap_optional_artifacts_ok
# Purpose: Verify optional bootstrap artifacts are present or explicitly skipped.
# Args: None.
# Output: None.
# Returns: 0 if requirements are met; 1 otherwise.
bootstrap_optional_artifacts_ok() {
  local task_file
  if ! task_file="$(bootstrap_task_path)"; then
    return 1
  fi
  local artifacts=()
  mapfile -t artifacts < <(bootstrap_optional_artifacts)
  local artifact
  for artifact in "${artifacts[@]}"; do
    if artifact_present "${artifact}"; then
      continue
    fi
    if ! artifact_skipped_in_task "${task_file}" "${artifact}"; then
      return 1
    fi
  done
  return 0
}

# bootstrap_adrs_ok
# Purpose: Verify ADR expectations are met or explicitly waived.
# Args: None.
# Output: None.
# Returns: 0 if ADR requirement is satisfied; 1 otherwise.
bootstrap_adrs_ok() {
  local mode
  if mode="$(read_project_mode)"; then
    if [[ "${mode}" == "existing" ]]; then
      return 0
    fi
  fi
  if [[ -d "${BOOTSTRAP_DOCS_DIR}" ]]; then
    if find "${BOOTSTRAP_DOCS_DIR}" "${BOOTSTRAP_DOCS_DIR}/adr" -maxdepth 1 -type f -iname 'adr*.md' -print -quit 2> /dev/null | grep -q .; then
      return 0
    fi
  fi
  local task_file
  if task_file="$(bootstrap_task_path)"; then
    if grep -Eiq "no adr|no adrs|adr not required" "${task_file}"; then
      return 0
    fi
  fi
  return 1
}

# has_non_bootstrap_tasks
# Purpose: Detect task files that are not the bootstrap task.
# Args: None.
# Output: Prints the first non-bootstrap task path found.
# Returns: 0 if a non-bootstrap task exists; 1 otherwise.
has_non_bootstrap_tasks() {
  local path
  while IFS= read -r path; do
    local base
    base="$(basename "${path}")"
    if [[ "${base}" == ".keep" ]]; then
      continue
    fi
    if [[ "${base}" == "${BOOTSTRAP_TASK_NAME}.md" ]]; then
      continue
    fi
    printf '%s\n' "${path}"
    return 0
  done < <(find "${STATE_DIR}" -maxdepth 2 -type f -path "${STATE_DIR}/task-*/*" -name '*.md' 2> /dev/null | sort)
  return 1
}

# bootstrap_requirements_met
# Purpose: Check whether all bootstrap completion requirements are met.
# Args: None.
# Output: None.
# Returns: 0 if requirements are met; 1 otherwise.
bootstrap_requirements_met() {
  if ! bootstrap_task_path > /dev/null 2>&1; then
    return 1
  fi
  if ! bootstrap_required_artifacts_ok; then
    return 1
  fi
  if ! bootstrap_optional_artifacts_ok; then
    return 1
  fi
  if ! bootstrap_adrs_ok; then
    return 1
  fi
  return 0
}

# architecture_bootstrap_complete
# Purpose: Determine whether the bootstrap task is completed and valid.
# Args: None.
# Output: None.
# Returns: 0 if complete; 1 otherwise.
architecture_bootstrap_complete() {
  local task_dir
  if ! task_dir="$(bootstrap_task_dir)"; then
    return 1
  fi
  if [[ "${task_dir}" != "task-done" && "${task_dir}" != "task-archive" ]]; then
    return 1
  fi
  if ! bootstrap_requirements_met; then
    return 1
  fi
  return 0
}

# complete_bootstrap_task_if_ready
# Purpose: Auto-complete the bootstrap task when requirements are met.
# Args: None.
# Output: Logs task transition and commits changes.
# Returns: 0 on completion; 1 if not ready.
complete_bootstrap_task_if_ready() {
  if ! bootstrap_requirements_met; then
    return 1
  fi
  if has_non_bootstrap_tasks > /dev/null 2>&1; then
    return 1
  fi
  if in_flight_has_task "${BOOTSTRAP_TASK_NAME}"; then
    return 0
  fi
  local task_file
  if ! task_file="$(bootstrap_task_path)"; then
    return 0
  fi
  local task_dir
  task_dir="$(basename "$(dirname "${task_file}")")"
  if [[ "${task_dir}" == "task-done" || "${task_dir}" == "task-archive" ]]; then
    return 0
  fi
  move_task_file "${task_file}" "${STATE_DIR}/task-done" "${BOOTSTRAP_TASK_NAME}" "moved to task-done"
  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Complete architecture bootstrap"
  git_push_default_branch
  return 0
}

# assign_bootstrap_task
# Purpose: Assign the bootstrap task to the architect role.
# Args:
#   $1: Task file path (string).
# Output: Logs assignment and spawns worker.
# Returns: 0 on completion.
assign_bootstrap_task() {
  local task_file="$1"
  local worker="${BOOTSTRAP_ROLE}"

  sync_default_branch

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local assigned_file="${STATE_DIR}/task-assigned/${task_name}.md"
  annotate_assignment "${task_file}" "${worker}"
  move_task_file "${task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "assigned to ${worker}"

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -q -m "[governator] Assign task ${task_name}"
  git_push_default_branch

  in_flight_add "${task_name}" "${worker}"
  spawn_worker_for_task "${assigned_file}" "${worker}" ""
}
