#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

#############################################################################
# The Governator
#############################################################################
#
# Single-file implementation of the orchestrator. The script enforces a lock,
# requires a clean git state, processes worker branches, and assigns backlog
# tasks. It is intentionally explicit about filesystem and git transitions.
#
#############################################################################
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="${ROOT_DIR}/_governator"
DB_DIR="${ROOT_DIR}/.governator"
NEXT_TICKET_FILE="${DB_DIR}/next_ticket_id"
GLOBAL_CAP_FILE="${DB_DIR}/global_worker_cap"
WORKER_CAPS_FILE="${DB_DIR}/worker_caps"
WORKER_ROLES_DIR="${STATE_DIR}/worker-roles"
SPECIAL_ROLES_DIR="${STATE_DIR}/special-roles"
TEMPLATES_DIR="${STATE_DIR}/templates"
LOCK_FILE="${STATE_DIR}/governator.lock"
FAILED_MERGES_LOG="${STATE_DIR}/failed-merges.log"
IN_FLIGHT_LOG="${STATE_DIR}/in-flight.log"
EXPERTISE_FILE="${STATE_DIR}/expertise.md"

CODEX_BIN="${CODEX_BIN:-codex}"
CODEX_WORKER_ARGS="${CODEX_WORKER_ARGS:---non-interactive}"
CODEX_REVIEW_ARGS="${CODEX_REVIEW_ARGS:---non-interactive}"

PROJECT_NAME="$(basename "${ROOT_DIR}")"
USE_RG=0

if command -v rg >/dev/null 2>&1; then
  USE_RG=1
fi

# Log with a consistent UTC timestamp prefix.
log_info() {
  printf '[%s] INFO  %s\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "$*"
}

# Log a warning with timestamp.
log_warn() {
  printf '[%s] WARN  %s\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "$*"
}

# Log an error with timestamp.
log_error() {
  printf '[%s] ERROR %s\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "$*"
}

# Fixed-string match abstraction so we can fall back to grep if rg is missing.
match_fixed() {
  local pattern="$1"
  local file="$2"
  if [[ "${USE_RG}" -eq 1 ]]; then
    rg -q -F "${pattern}" "${file}"
    return $?
  fi
  grep -qF "${pattern}" "${file}"
}

# First regex match helper with rg/grep fallback.
first_match_regex() {
  local pattern="$1"
  local file="$2"
  if [[ "${USE_RG}" -eq 1 ]]; then
    rg -i -m1 "${pattern}" "${file}"
    return $?
  fi
  grep -i -m1 -E "${pattern}" "${file}"
}

# Filter out fixed-string matches with rg/grep fallback.
filter_out_fixed() {
  local pattern="$1"
  local file="$2"
  if [[ "${USE_RG}" -eq 1 ]]; then
    rg -v -F "${pattern}" "${file}"
    return $?
  fi
  grep -vF "${pattern}" "${file}"
}

# Remove lock on exit.
cleanup_lock() {
  if [[ -f "${LOCK_FILE}" ]]; then
    rm -f "${LOCK_FILE}"
  fi
}

# Ensure we don't run two governators simultaneously.
ensure_lock() {
  if [[ -f "${LOCK_FILE}" ]]; then
    log_warn "Lock file exists at ${LOCK_FILE}, exiting."
    exit 0
  fi
  printf '%s\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" > "${LOCK_FILE}"
  trap cleanup_lock EXIT
}

# Avoid processing while the repo has local edits.
ensure_clean_git() {
  if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain)" ]]; then
    log_warn "Local git changes detected, exiting."
    exit 0
  fi
}

# Checkout main quietly.
git_checkout_main() {
  git -C "${ROOT_DIR}" checkout main >/dev/null 2>&1
}

# Pull main from origin.
git_pull_main() {
  git -C "${ROOT_DIR}" pull origin main
}

# Fetch and prune remote refs.
git_fetch_origin() {
  git -C "${ROOT_DIR}" fetch origin --prune
}

# Ensure state logs exist so reads do not fail.
touch_logs() {
  touch "${FAILED_MERGES_LOG}" "${IN_FLIGHT_LOG}"
}

# Read a numeric value from a file or return a default.
read_numeric_file() {
  local file="$1"
  local fallback="$2"
  if [[ ! -f "${file}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi

  local value
  value="$(tr -d '[:space:]' < "${file}")"
  if [[ -z "${value}" || ! "${value}" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi
  printf '%s\n' "${value}"
}

# Read the global concurrency cap (defaults to 1).
read_global_cap() {
  read_numeric_file "${GLOBAL_CAP_FILE}" "1"
}

# Read per-worker cap from worker_caps (defaults to 1).
read_worker_cap() {
  local role="$1"
  if [[ ! -f "${WORKER_CAPS_FILE}" ]]; then
    printf '1\n'
    return 0
  fi

  local cap
  cap="$(
    awk -v role="${role}" '
      $0 ~ /^[[:space:]]*#/ { next }
      $0 ~ /^[[:space:]]*$/ { next }
      $0 ~ /^[[:space:]]*[^:]+[[:space:]]*:[[:space:]]*[0-9]+[[:space:]]*$/ {
        split($0, parts, ":")
        key = parts[1]
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", key)
        if (key == role) {
          val = parts[2]
          gsub(/^[[:space:]]+|[[:space:]]+$/, "", val)
          print val
          exit 0
        }
      }
      END { exit 1 }
    ' "${WORKER_CAPS_FILE}" || true
  )"

  if [[ -z "${cap}" ]]; then
    printf '1\n'
    return 0
  fi
  printf '%s\n' "${cap}"
}

# Count in-flight tasks (all roles).
count_in_flight_total() {
  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    printf '0\n'
    return 0
  fi
  awk '
    $0 ~ / -> / { count += 1 }
    END { print count + 0 }
  ' "${IN_FLIGHT_LOG}"
}

# Count in-flight tasks for a specific role.
count_in_flight_role() {
  local role="$1"
  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    printf '0\n'
    return 0
  fi
  awk -v role="${role}" '
    $0 ~ / -> / {
      split($0, parts, " -> ")
      if (parts[2] == role) {
        count += 1
      }
    }
    END { print count + 0 }
  ' "${IN_FLIGHT_LOG}"
}

# Ensure the simple DB directory exists.
ensure_db_dir() {
  if [[ ! -d "${DB_DIR}" ]]; then
    mkdir -p "${DB_DIR}"
  fi
}

# Read the next ticket id from disk, defaulting to 1.
read_next_ticket_id() {
  ensure_db_dir
  if [[ ! -f "${NEXT_TICKET_FILE}" ]]; then
    printf '1\n'
    return 0
  fi

  local value
  value="$(tr -d '[:space:]' < "${NEXT_TICKET_FILE}")"
  if [[ -z "${value}" ]]; then
    printf '1\n'
    return 0
  fi
  printf '%s\n' "${value}"
}

# Persist the next ticket id.
write_next_ticket_id() {
  local value="$1"
  ensure_db_dir
  printf '%s\n' "${value}" > "${NEXT_TICKET_FILE}"
}

# Format a numeric ticket id as zero-padded 3 digits.
format_ticket_id() {
  local value="$1"
  printf '%03d' "${value}"
}

# Allocate the next ticket id and increment the stored value.
allocate_ticket_id() {
  local current
  current="$(read_next_ticket_id)"
  if ! [[ "${current}" =~ ^[0-9]+$ ]]; then
    log_warn "Invalid ticket id value '${current}', resetting to 1."
    current=1
  fi

  local next=$((current + 1))
  write_next_ticket_id "${next}"
  printf '%s\n' "${current}"
}

# Create a new ticket file using the template and allocated id.
create_ticket_file() {
  local short_name="$1"
  local role="$2"
  local target_dir="$3"

  local ticket_id
  ticket_id="$(allocate_ticket_id)"

  local id_segment
  id_segment="$(format_ticket_id "${ticket_id}")"
  local filename="${id_segment}-${short_name}-${role}.md"

  local template="${TEMPLATES_DIR}/ticket.md"
  if [[ ! -f "${template}" ]]; then
    log_error "Missing ticket template at ${template}."
    return 1
  fi

  local dest="${target_dir}/${filename}"
  cp "${template}" "${dest}"
  printf '%s\n' "${dest}"
}
# Append a titled section to a task file.
append_section() {
  local file="$1"
  local title="$2"
  local body="$3"
  {
    printf '\n%s\n\n' "${title}"
    printf '%s\n' "${body}"
  } >> "${file}"
}

# Join arguments by a delimiter.
join_by() {
  local delimiter="$1"
  shift
  local first=1
  local item
  for item in "$@"; do
    if [[ "${first}" -eq 1 ]]; then
      printf '%s' "${item}"
      first=0
    else
      printf '%s%s' "${delimiter}" "${item}"
    fi
  done
}

# Materialize is_expert.md if expertise modifiers exist.
create_expert_file() {
  local dir="$1"
  if [[ ! -f "${EXPERTISE_FILE}" ]]; then
    return 0
  fi

  # Strip comments and whitespace so the prompt is stable and predictable.
  mapfile -t expertise_lines < <(
    awk '
      /^[[:space:]]*#/ { next }
      /^[[:space:]]*$/ { next }
      { gsub(/^[[:space:]]+|[[:space:]]+$/, "", $0); print }
    ' "${EXPERTISE_FILE}"
  )

  if [[ "${#expertise_lines[@]}" -eq 0 ]]; then
    return 0
  fi

  local joined
  joined="$(join_by ", " "${expertise_lines[@]}")"
  printf 'You have deep expertise in %s.\n' "${joined}" > "${dir}/is_expert.md"
}

# Start the worker without blocking this script.
run_codex_worker_detached() {
  local dir="$1"
  local prompt="$2"
  if [[ -n "${CODEX_WORKER_CMD:-}" ]]; then
    (
      cd "${dir}"
      GOV_PROMPT="${prompt}" nohup bash -c "${CODEX_WORKER_CMD}" >/dev/null 2>&1 &
    )
    return 0
  fi

  # Use nohup to prevent worker exit from being tied to this process.
  local args=()
  read -r -a args <<< "${CODEX_WORKER_ARGS}"
  (
    cd "${dir}"
    nohup "${CODEX_BIN}" exec "${args[@]}" --message "${prompt}" >/dev/null 2>&1 &
  )
}

# Run the reviewer synchronously so a review.json is produced.
run_codex_reviewer() {
  local dir="$1"
  local prompt="$2"
  if [[ -n "${CODEX_REVIEW_CMD:-}" ]]; then
    (cd "${dir}" && GOV_PROMPT="${prompt}" bash -c "${CODEX_REVIEW_CMD}")
    return 0
  fi

  local args=()
  read -r -a args <<< "${CODEX_REVIEW_ARGS}"
  (cd "${dir}" && "${CODEX_BIN}" exec "${args[@]}" --message "${prompt}")
}

# List remote worker branches.
list_worker_branches() {
  git -C "${ROOT_DIR}" for-each-ref --format='%(refname:short)' refs/remotes/origin/worker/* || true
}

# Check whether a branch is recorded as a failed merge.
is_failed_merge_branch() {
  local branch="$1"
  if [[ ! -f "${FAILED_MERGES_LOG}" ]]; then
    return 1
  fi
  if awk -v branch="${branch}" '
    NF >= 2 {
      if ($2 == branch) { found=1 }
    }
    END { exit found ? 0 : 1 }
  ' "${FAILED_MERGES_LOG}"; then
    return 0
  fi
  return 1
}

# Remove an in-flight record when a task completes or is blocked.
remove_in_flight_entry() {
  local task_name="$1"
  local worker_name="$2"
  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 0
  fi

  local tmp_file
  tmp_file="$(mktemp)"
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
  mv "${tmp_file}" "${IN_FLIGHT_LOG}"
}

# Find a task file in any task-* directory by base name.
task_file_for_name() {
  local task_name="$1"
  local matches=()
  while IFS= read -r path; do
    matches+=("${path}")
  done < <(find "${STATE_DIR}" -maxdepth 2 -type f -path "${STATE_DIR}/task-*/${task_name}.md" 2>/dev/null || true)

  if [[ "${#matches[@]}" -eq 0 ]]; then
    return 1
  fi
  if [[ "${#matches[@]}" -gt 1 ]]; then
    log_warn "Multiple task files found for ${task_name}, using ${matches[0]}"
  fi
  printf '%s\n' "${matches[0]}"
}

# Enumerate non-reviewer worker roles.
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

# Extract the required worker role from the task filename suffix.
extract_worker_from_task() {
  local task_file="$1"
  local task_name
  task_name="$(basename "${task_file}" .md)"

  local suffix="${task_name##*-}"
  if [[ -z "${suffix}" || "${suffix}" == "${task_name}" ]]; then
    return 1
  fi

  local role
  while IFS= read -r role; do
    if [[ "${suffix}" == "${role}" ]]; then
      printf '%s' "${role}"
      return 0
    fi
  done < <(list_available_workers)

  return 1
}

# Check whether a task is already in flight.
in_flight_has_task() {
  local task_name="$1"
  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 1
  fi
  if awk -v task="${task_name}" '
    $0 ~ / -> / {
      split($0, parts, " -> ")
      if (parts[1] == task) { found=1 }
    }
    END { exit found ? 0 : 1 }
  ' "${IN_FLIGHT_LOG}"; then
    return 0
  fi
  return 1
}

# Check whether a worker is already in flight.
in_flight_has_worker() {
  local worker_name="$1"
  if [[ ! -f "${IN_FLIGHT_LOG}" ]]; then
    return 1
  fi
  if awk -v worker="${worker_name}" '
    $0 ~ / -> / {
      split($0, parts, " -> ")
      if (parts[2] == worker) { found=1 }
    }
    END { exit found ? 0 : 1 }
  ' "${IN_FLIGHT_LOG}"; then
    return 0
  fi
  return 1
}

# Block a task when required metadata is missing or invalid.
block_task_from_backlog() {
  local task_file="$1"
  local reason="$2"

  git_checkout_main

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local blocked_file="${STATE_DIR}/task-blocked/${task_name}.md"
  mv "${task_file}" "${blocked_file}"
  annotate_blocked "${blocked_file}" "${reason}"

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -m "Block task ${task_name}"
  git -C "${ROOT_DIR}" push origin main
}

# Record assignment details in the task file.
annotate_assignment() {
  local task_file="$1"
  local worker="$2"
  append_section "${task_file}" "## Assignment" "Assigned to ${worker} by Governator on $(date -u +"%Y-%m-%dT%H:%M:%SZ")."
}

# Record review decision and comments in the task file.
annotate_review() {
  local task_file="$1"
  local decision="$2"
  local comments=("$@")
  comments=("${comments[@]:2}")

  append_section "${task_file}" "## Review Result" "Decision: ${decision}"
  if [[ "${#comments[@]}" -gt 0 ]]; then
    {
      printf '\nComments:\n'
      for comment in "${comments[@]}"; do
        printf '- %s\n' "${comment}"
      done
    } >> "${task_file}"
  fi
}

# Add feedback to a task file before reassigning.
annotate_feedback() {
  local task_file="$1"
  append_section "${task_file}" "## Feedback" "Moved back to task-assigned for follow-up on $(date -u +"%Y-%m-%dT%H:%M:%SZ")."
}

# Capture a blocking reason in the task file.
annotate_blocked() {
  local task_file="$1"
  local reason="$2"
  append_section "${task_file}" "## Governator Block" "${reason}"
}

# Record a merge failure for reviewer visibility.
annotate_merge_failure() {
  local task_file="$1"
  local branch="$2"
  append_section "${task_file}" "## Merge Failure" "Unable to fast-forward merge ${branch} into main on $(date -u +"%Y-%m-%dT%H:%M:%SZ")."
}

# Parse review.json for decision and comments.
parse_review_json() {
  local file="$1"
  if [[ ! -f "${file}" ]]; then
    printf 'block\nReview file missing at %s\n' "${file}"
    return 0
  fi

  # Use Python for strict JSON parsing; shell parsing is error-prone.
  if command -v python3 >/dev/null 2>&1; then
    if ! python3 - "${file}" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)

result = str(data.get("result", "")).strip()
comments = data.get("comments") or []
if not isinstance(comments, list):
    comments = [str(comments)]

print(result)
for comment in comments:
    print(comment)
PY
    then
      printf 'block\nFailed to parse review.json\n'
    fi
    return 0
  fi

  printf 'block\nPython3 unavailable to parse review.json\n'
}

# Run reviewer flow in a clean clone and return parsed review output.
code_review() {
  local remote_branch="$1"
  local local_branch="$2"
  local task_relpath="$3"

  local tmp_dir
  tmp_dir="$(mktemp -d "/tmp/governator-${PROJECT_NAME}-reviewer-${local_branch//\//-}-XXXXXX")"

  git clone "$(git -C "${ROOT_DIR}" remote get-url origin)" "${tmp_dir}" >/dev/null 2>&1
  git -C "${tmp_dir}" fetch origin >/dev/null 2>&1
  git -C "${tmp_dir}" checkout -B "${local_branch}" "${remote_branch}" >/dev/null 2>&1

  # Seed with a template to guide reviewers toward the expected schema.
  if [[ -f "${TEMPLATES_DIR}/review.json" ]]; then
    cp "${TEMPLATES_DIR}/review.json" "${tmp_dir}/review.json"
  fi

  create_expert_file "${tmp_dir}"

  local prompt
  if [[ -f "${tmp_dir}/is_expert.md" ]]; then
    prompt="Read and follow the instructions in the following files, in this order: _governator/special-roles/reviewer.md, is_expert.md. The task given was ${task_relpath}."
  else
    prompt="Read and follow the instructions in the following files, in this order: _governator/special-roles/reviewer.md. The task given was ${task_relpath}."
  fi

  if ! run_codex_reviewer "${tmp_dir}" "${prompt}"; then
    log_warn "Reviewer command failed for ${local_branch}."
  fi

  local review_output=()
  mapfile -t review_output < <(parse_review_json "${tmp_dir}/review.json")
  rm -rf "${tmp_dir}"

  if [[ "${#review_output[@]}" -eq 0 ]]; then
    printf 'block\nReview output missing\n'
    return 0
  fi

  printf '%s\n' "${review_output[@]}"
}

# Move task to assigned, commit, push, then spawn a worker.
assign_task() {
  local task_file="$1"
  local worker="$2"

  git_checkout_main

  local task_name
  task_name="$(basename "${task_file}" .md)"

  local assigned_file="${STATE_DIR}/task-assigned/${task_name}.md"
  mv "${task_file}" "${assigned_file}"
  annotate_assignment "${assigned_file}" "${worker}"

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -m "Assign task ${task_name}"
  git -C "${ROOT_DIR}" push origin main

  local tmp_dir
  tmp_dir="$(mktemp -d "/tmp/governator-${PROJECT_NAME}-${worker}-${task_name}-XXXXXX")"

  git clone "$(git -C "${ROOT_DIR}" remote get-url origin)" "${tmp_dir}" >/dev/null 2>&1
  git -C "${tmp_dir}" checkout -b "worker/${worker}/${task_name}" origin/main >/dev/null 2>&1

  create_expert_file "${tmp_dir}"

  local prompt
  if [[ -f "${tmp_dir}/is_expert.md" ]]; then
    prompt="Read and follow the instructions in the following files, in this order: _governator/worker_contract.md, _governator/worker-roles/${worker}.md, is_expert.md, _governator/task-assigned/${task_name}.md."
  else
    prompt="Read and follow the instructions in the following files, in this order: _governator/worker_contract.md, _governator/worker-roles/${worker}.md, _governator/task-assigned/${task_name}.md."
  fi

  run_codex_worker_detached "${tmp_dir}" "${prompt}"

  log_info "Worker started for ${task_name} on ${worker} in ${tmp_dir}"
}

# Assign tasks in backlog based on role prefix/suffix in filename.
assign_pending_tasks() {
  touch_logs

  local task_file
  while IFS= read -r task_file; do
    if [[ "${task_file}" == *"/.keep" ]]; then
      continue
    fi

    local task_name
    task_name="$(basename "${task_file}" .md)"

    if in_flight_has_task "${task_name}"; then
      continue
    fi

    local worker
    if ! worker="$(extract_worker_from_task "${task_file}")"; then
      log_warn "Missing required role for ${task_name}, blocking."
      block_task_from_backlog "${task_file}" "Missing required role in frontmatter."
      continue
    fi

    if [[ ! -f "${WORKER_ROLES_DIR}/${worker}.md" ]]; then
      log_warn "Unknown role ${worker} for ${task_name}, blocking."
      block_task_from_backlog "${task_file}" "Unknown role ${worker} referenced in frontmatter."
      continue
    fi
    local total_count
    total_count="$(count_in_flight_total)"
    local global_cap
    global_cap="$(read_global_cap)"
    if [[ "${total_count}" -ge "${global_cap}" ]]; then
      log_warn "Global worker cap reached (${total_count}/${global_cap}), skipping ${task_name}."
      continue
    fi

    local role_count
    role_count="$(count_in_flight_role "${worker}")"
    local role_cap
    role_cap="$(read_worker_cap "${worker}")"
    if [[ "${role_count}" -ge "${role_cap}" ]]; then
      log_warn "Role ${worker} at cap (${role_count}/${role_cap}) for ${task_name}, skipping."
      continue
    fi

    assign_task "${task_file}" "${worker}"
    printf '%s -> %s\n' "${task_name}" "${worker}" >> "${IN_FLIGHT_LOG}"
  done < <(find "${STATE_DIR}/task-backlog" -maxdepth 1 -type f -name '*.md' | sort)
}

# Process a single worker branch: review, move task, merge, cleanup.
process_worker_branch() {
  local remote_branch="$1"
  local local_branch="${remote_branch#origin/}"
  local worker_name="${local_branch#worker/}"
  worker_name="${worker_name%%/*}"

  git_fetch_origin
  git -C "${ROOT_DIR}" checkout -B "${local_branch}" "${remote_branch}" >/dev/null 2>&1

  local task_name
  task_name="${local_branch##*/}"

  local task_file
  if ! task_file="$(task_file_for_name "${task_name}")"; then
    # No task to annotate; record and drop the branch.
    log_warn "No task file found for ${task_name}, skipping merge."
    printf '%s %s missing task file\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "${local_branch}" >> "${FAILED_MERGES_LOG}"
    remove_in_flight_entry "${task_name}" "${worker_name}"
    git -C "${ROOT_DIR}" branch -D "${local_branch}" >/dev/null 2>&1 || true
    git -C "${ROOT_DIR}" push origin --delete "${local_branch}" >/dev/null 2>&1 || true
    find /tmp -maxdepth 1 -type d -name "governator-${PROJECT_NAME}-${worker_name}-${task_name}-*" -exec rm -rf {} + >/dev/null 2>&1 || true
    return 0
  fi

  local task_dir
  task_dir="$(basename "$(dirname "${task_file}")")"

  case "${task_dir}" in
    task-worked)
      local review_lines=()
      mapfile -t review_lines < <(code_review "${remote_branch}" "${local_branch}" "${task_file#"${ROOT_DIR}/"}")
      local decision="${review_lines[0]:-block}"
      annotate_review "${task_file}" "${decision}" "${review_lines[@]:1}"

      case "${decision}" in
        approve)
          mv "${task_file}" "${STATE_DIR}/task-done/$(basename "${task_file}")"
          ;;
        reject)
          mv "${task_file}" "${STATE_DIR}/task-assigned/$(basename "${task_file}")"
          ;;
        *)
          mv "${task_file}" "${STATE_DIR}/task-blocked/$(basename "${task_file}")"
          ;;
      esac
      ;;
    task-feedback)
      annotate_feedback "${task_file}"
      mv "${task_file}" "${STATE_DIR}/task-assigned/$(basename "${task_file}")"
      ;;
    *)
      log_warn "Unexpected task state ${task_dir} for ${task_name}, blocking."
      annotate_blocked "${task_file}" "Unexpected task state ${task_dir} during processing."
      mv "${task_file}" "${STATE_DIR}/task-blocked/$(basename "${task_file}")"
      ;;
  esac

  git -C "${ROOT_DIR}" add "${STATE_DIR}"
  git -C "${ROOT_DIR}" commit -m "Process task ${task_name}"

  git_checkout_main

  if git -C "${ROOT_DIR}" merge --ff-only "${local_branch}"; then
    git -C "${ROOT_DIR}" push origin main
  else
    log_error "Failed to fast-forward merge ${local_branch} into main."

    local main_task_file
    if main_task_file="$(task_file_for_name "${task_name}")"; then
      # Keep main's task state authoritative; block and surface the failure.
      annotate_merge_failure "${main_task_file}" "${local_branch}"
      mv "${main_task_file}" "${STATE_DIR}/task-blocked/$(basename "${main_task_file}")"
      git -C "${ROOT_DIR}" add "${STATE_DIR}"
      git -C "${ROOT_DIR}" commit -m "Block task ${task_name} on merge failure"
      git -C "${ROOT_DIR}" push origin main
    fi

    printf '%s %s\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "${local_branch}" >> "${FAILED_MERGES_LOG}"
  fi

  remove_in_flight_entry "${task_name}" "${worker_name}"

  git -C "${ROOT_DIR}" branch -D "${local_branch}" >/dev/null 2>&1 || true
  git -C "${ROOT_DIR}" push origin --delete "${local_branch}" >/dev/null 2>&1 || true

  find /tmp -maxdepth 1 -type d -name "governator-${PROJECT_NAME}-${worker_name}-${task_name}-*" -exec rm -rf {} + >/dev/null 2>&1 || true
}

# Iterate all worker branches, skipping those logged as failed merges.
process_worker_branches() {
  touch_logs
  git_fetch_origin

  local branch
  while IFS= read -r branch; do
    if [[ -z "${branch}" ]]; then
      continue
    fi
    if is_failed_merge_branch "${branch}"; then
      continue
    fi
    process_worker_branch "${branch}"
  done < <(list_worker_branches)
}

# Script entrypoint.
main() {
  ensure_lock
  ensure_clean_git
  ensure_db_dir
  git_checkout_main
  git_pull_main
  process_worker_branches
  assign_pending_tasks
}

main "$@"
