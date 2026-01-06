# shellcheck shell=bash

# Parse review.json for decision and comments.
parse_review_json() {
  local file="$1"
  if [[ ! -f "${file}" ]]; then
    printf 'block\nReview file missing at %s\n' "${file}"
    return 0
  fi

  if ! jq -e '.result' "${file}" > /dev/null 2>&1; then
    printf 'block\nFailed to parse review.json\n'
    return 0
  fi

  local result
  result="$(jq -r '.result // ""' "${file}")"
  printf '%s\n' "${result}"
  jq -r '.comments // [] | if type == "array" then .[] else . end' "${file}"
}

read_reviewer_output() {
  local tmp_dir="$1"
  log_verbose_file "Reviewer output file" "${tmp_dir}/review.json"
  local review_output=()
  mapfile -t review_output < <(parse_review_json "${tmp_dir}/review.json")
  if [[ "${#review_output[@]}" -eq 0 ]]; then
    review_output=("block" "Review output missing")
  fi
  printf '%s\n' "${review_output[@]}"
}

# Run reviewer flow in a clean clone and return parsed review output.
code_review() {
  local remote_branch="$1"
  local local_branch="$2"
  local task_relpath="$3"

  local tmp_dir
  tmp_dir="$(mktemp -d "/tmp/governator-${PROJECT_NAME}-reviewer-${local_branch//\//-}-XXXXXX")"

  local remote
  remote="$(read_remote_name)"
  git clone "$(git -C "${ROOT_DIR}" remote get-url "${remote}")" "${tmp_dir}" > /dev/null 2>&1
  git -C "${tmp_dir}" fetch "${remote}" > /dev/null 2>&1
  git -C "${tmp_dir}" checkout -B "${local_branch}" "${remote_branch}" > /dev/null 2>&1

  # Seed with a template to guide reviewers toward the expected schema.
  if [[ -f "${TEMPLATES_DIR}/review.json" ]]; then
    cp "${TEMPLATES_DIR}/review.json" "${tmp_dir}/review.json"
  fi

  local log_dir
  log_dir="${DB_DIR}/logs"
  mkdir -p "${log_dir}"
  local task_base
  task_base="$(basename "${task_relpath}" .md)"
  local log_file
  log_file="${log_dir}/${task_base}-reviewer.log"
  append_worker_log_separator "${log_file}"

  local prompt
  prompt="$(build_special_prompt "reviewer" "${task_relpath}")"

  log_task_event "${task_base}" "starting review for ${local_branch}"

  if ! run_codex_reviewer "${tmp_dir}" "${prompt}" "${log_file}"; then
    log_warn "Reviewer command failed for ${local_branch}."
  fi

  local review_output=()
  mapfile -t review_output < <(read_reviewer_output "${tmp_dir}")
  cleanup_tmp_dir "${tmp_dir}"

  if [[ "${#review_output[@]}" -eq 0 ]]; then
    printf 'block\nReview output missing\n'
    return 0
  fi

  printf '%s\n' "${review_output[@]}"
}

# Apply a reviewer decision to the task file and commit the state update.
apply_review_decision() {
  local task_name="$1"
  local worker_name="$2"
  local decision="$3"
  local block_reason="$4"
  shift 4
  local review_lines=("$@")

  local main_task_file
  if ! main_task_file="$(task_file_for_name "${task_name}")"; then
    log_warn "Task file missing for ${task_name} after review; skipping state update."
    return 1
  fi

  local task_dir
  task_dir="$(basename "$(dirname "${main_task_file}")")"

  case "${task_dir}" in
    task-worked | task-assigned)
      if [[ "${task_dir}" == "task-assigned" && ! ("${worker_name}" == "reviewer" && "${task_name}" == 000-*) ]]; then
        log_warn "Unexpected task state ${task_dir} for ${task_name}, blocking."
        annotate_blocked "${main_task_file}" "${block_reason}"
        move_task_file "${main_task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
      else
        annotate_review "${main_task_file}" "${decision}" "${review_lines[@]}"
        log_task_event "${task_name}" "review decision: ${decision}"
        case "${decision}" in
          approve)
            if [[ "${task_name}" == "${DONE_CHECK_REVIEW_TASK}" ]]; then
              write_project_done_sha "$(governator_doc_sha)"
              move_task_file "${main_task_file}" "${STATE_DIR}/task-done" "${task_name}" "moved to task-done"
            else
              move_task_file "${main_task_file}" "${STATE_DIR}/task-done" "${task_name}" "moved to task-done"
            fi
            ;;
          reject)
            if [[ "${task_name}" == "${DONE_CHECK_REVIEW_TASK}" ]]; then
              write_project_done_sha ""
              move_done_check_to_planner "${main_task_file}" "${task_name}"
            else
              move_task_file "${main_task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "moved to task-assigned"
            fi
            ;;
          *)
            if [[ "${task_name}" == "${DONE_CHECK_REVIEW_TASK}" ]]; then
              write_project_done_sha ""
              move_done_check_to_planner "${main_task_file}" "${task_name}"
            else
              move_task_file "${main_task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
            fi
            ;;
        esac
      fi
      ;;
    task-feedback)
      annotate_feedback "${main_task_file}"
      move_task_file "${main_task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "moved to task-assigned"
      ;;
    *)
      log_warn "Unexpected task state ${task_dir} for ${task_name}, blocking."
      annotate_blocked "${main_task_file}" "${block_reason}"
      move_task_file "${main_task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
      ;;
  esac

  git -C "${ROOT_DIR}" add "${STATE_DIR}" "${AUDIT_LOG}"
  if [[ -f "${PROJECT_DONE_FILE}" ]]; then
    git -C "${ROOT_DIR}" add "${PROJECT_DONE_FILE}"
  fi
  git -C "${ROOT_DIR}" commit -q -m "[governator] Process task ${task_name}"
  return 0
}
