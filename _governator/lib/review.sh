# shellcheck shell=bash

# parse_review_json
# Purpose: Extract the decision and comments from a review.json file, normalizing
#   known approval/rejection/block variants.
# Args:
#   $1: Path to review.json (string).
# Output: Writes decision on first line, followed by zero or more comment lines.
# Returns: 0 always; emits "block" on missing or invalid JSON.
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
  local normalized
  normalized="$(printf '%s' "${result}" | tr '[:upper:]' '[:lower:]')"
  case "${normalized}" in
    accept | accepted | approve | approved | pass)
      printf '%s\n' "approve"
      ;;
    reject | rejected | deny | denied | fail)
      printf '%s\n' "reject"
      ;;
    block | blocked | stuck)
      printf '%s\n' "block"
      ;;
    *)
      printf '%s\n' "${normalized}"
      ;;
  esac
  jq -r '.comments // [] | if type == "array" then .[] else . end' "${file}"
}

# read_review_output_from_branch
# Purpose: Read and normalize reviewer output from a git branch.
# Args:
#   $1: Branch ref containing review.json (string).
# Output: Prints decision and comments to stdout, one per line.
# Returns: 0 always; falls back to "block" when output is missing.
read_review_output_from_branch() {
  local branch="$1"
  local tmp_file
  tmp_file="$(mktemp "/tmp/governator-${PROJECT_NAME}-review-XXXXXX.json")"
  local review_output=()
  if git -C "${ROOT_DIR}" show "${branch}:review.json" > "${tmp_file}" 2> /dev/null; then
    mapfile -t review_output < <(parse_review_json "${tmp_file}")
  else
    review_output=("block" "Review output missing at ${branch}:review.json")
  fi
  rm -f "${tmp_file}"
  if [[ "${#review_output[@]}" -eq 0 ]]; then
    review_output=("block" "Review output missing")
  fi
  printf '%s\n' "${review_output[@]}"
}

# apply_review_decision
# Purpose: Apply reviewer decision to task state, annotate, and commit changes.
# Args:
#   $1: Task name (string).
#   $2: Worker name (string).
#   $3: Decision string ("approve", "reject", or other).
#   $4: Block reason (string).
#   $5+: Review comment lines (strings).
# Output: Logs warnings, task events, and removes review.json from the repo root.
# Returns: 0 on success; 1 if the task file is missing.
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
  local gap_task="${GAP_ANALYSIS_PLANNER_TASK:-}"

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
            if [[ "${task_name}" == "${COMPLETION_CHECK_REVIEW_TASK}" ]]; then
              write_planning_gov_sha "$(governator_doc_sha)"
              move_task_file "${main_task_file}" "${STATE_DIR}/task-done" "${task_name}" "moved to task-done"
            else
              if [[ -n "${gap_task}" && "${task_name}" == "${gap_task}" ]]; then
                write_planning_gov_sha "$(governator_doc_sha)"
              fi
              move_task_file "${main_task_file}" "${STATE_DIR}/task-done" "${task_name}" "moved to task-done"
            fi
            ;;
          reject)
            if [[ "${task_name}" == "${COMPLETION_CHECK_REVIEW_TASK}" ]]; then
              write_planning_gov_sha ""
              move_completion_check_to_gap_analysis "${main_task_file}" "${task_name}"
            else
              if [[ -n "${gap_task}" && "${task_name}" == "${gap_task}" ]]; then
                write_planning_gov_sha ""
              fi
              move_task_file "${main_task_file}" "${STATE_DIR}/task-assigned" "${task_name}" "moved to task-assigned"
            fi
            ;;
          *)
            if [[ "${task_name}" == "${COMPLETION_CHECK_REVIEW_TASK}" ]]; then
              write_planning_gov_sha ""
              move_completion_check_to_gap_analysis "${main_task_file}" "${task_name}"
            else
              if [[ -n "${gap_task}" && "${task_name}" == "${gap_task}" ]]; then
                write_planning_gov_sha ""
              fi
              move_task_file "${main_task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
            fi
            ;;
        esac
      fi
      ;;
    *)
      log_warn "Unexpected task state ${task_dir} for ${task_name}, blocking."
      annotate_blocked "${main_task_file}" "${block_reason}"
      move_task_file "${main_task_file}" "${STATE_DIR}/task-blocked" "${task_name}" "moved to task-blocked"
      ;;
  esac

  if [[ -f "${ROOT_DIR}/review.json" ]]; then
    rm -f "${ROOT_DIR}/review.json"
  fi

  git -C "${ROOT_DIR}" add "${STATE_DIR}" "${AUDIT_LOG}"
  git -C "${ROOT_DIR}" add "${CONFIG_FILE}"
  if git -C "${ROOT_DIR}" ls-files --error-unmatch "review.json" > /dev/null 2>&1; then
    git -C "${ROOT_DIR}" add -u "review.json"
  fi
  git -C "${ROOT_DIR}" commit -q -m "[governator] Process task ${task_name}"
  return 0
}
