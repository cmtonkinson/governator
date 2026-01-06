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

#############################################################################
# Configuration
#############################################################################
# shellcheck disable=SC2034
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="${ROOT_DIR}/_governator"
DB_DIR="${ROOT_DIR}/.governator"

PROJECT_MODE_FILE="${DB_DIR}/project_mode"
DEFAULT_BRANCH_FILE="${DB_DIR}/default_branch"
REMOTE_NAME_FILE="${DB_DIR}/remote_name"

NEXT_TASK_FILE="${DB_DIR}/next_task_id"
GLOBAL_CAP_FILE="${DB_DIR}/global_worker_cap"
WORKER_CAPS_FILE="${DB_DIR}/worker_caps"
WORKER_TIMEOUT_FILE="${DB_DIR}/worker_timeout_seconds"
REASONING_EFFORT_FILE="${DB_DIR}/reasoning_effort"
DONE_CHECK_COOLDOWN_FILE="${DB_DIR}/done_check_cooldown_seconds"
DONE_CHECK_LAST_RUN_FILE="${DB_DIR}/last_done_check"
PROJECT_DONE_FILE="${DB_DIR}/project_done"
MANIFEST_FILE="${DB_DIR}/manifest.json"

AUDIT_LOG="${DB_DIR}/audit.log"
WORKER_PROCESSES_LOG="${DB_DIR}/worker-processes.log"
RETRY_COUNTS_LOG="${DB_DIR}/retry-counts.log"

WORKER_ROLES_DIR="${STATE_DIR}/roles-worker"
SPECIAL_ROLES_DIR="${STATE_DIR}/roles-special"
TEMPLATES_DIR="${STATE_DIR}/templates"
LOCK_FILE="${DB_DIR}/governator.lock"
FAILED_MERGES_LOG="${DB_DIR}/failed-merges.log"
IN_FLIGHT_LOG="${DB_DIR}/in-flight.log"
SYSTEM_LOCK_FILE="${DB_DIR}/governator.locked"
SYSTEM_LOCK_PATH="${SYSTEM_LOCK_FILE#"${ROOT_DIR}/"}"
GITIGNORE_PATH="${ROOT_DIR}/.gitignore"

GOV_QUIET=0
GOV_VERBOSE=0

DEFAULT_GLOBAL_CAP=1
DEFAULT_WORKER_CAP=1
DEFAULT_TASK_ID=1
DEFAULT_WORKER_TIMEOUT_SECONDS=900
DEFAULT_REMOTE_NAME="origin"
DEFAULT_BRANCH_NAME="main"

PROJECT_NAME="$(basename "${ROOT_DIR}")"

BOOTSTRAP_ROLE="architect"
BOOTSTRAP_TASK_NAME="000-architecture-bootstrap-${BOOTSTRAP_ROLE}"
BOOTSTRAP_NEW_TEMPLATE="${TEMPLATES_DIR}/000-architecture-bootstrap.md"
BOOTSTRAP_EXISTING_TEMPLATE="${TEMPLATES_DIR}/000-architecture-discovery.md"
BOOTSTRAP_DOCS_DIR="${ROOT_DIR}/_governator/docs"
BOOTSTRAP_NEW_REQUIRED_ARTIFACTS=("asr.md" "arc42.md")
BOOTSTRAP_NEW_OPTIONAL_ARTIFACTS=("personas.md" "wardley.md")
BOOTSTRAP_EXISTING_REQUIRED_ARTIFACTS=("existing-system-discovery.md")
BOOTSTRAP_EXISTING_OPTIONAL_ARTIFACTS=()

DONE_CHECK_REVIEW_ROLE="reviewer"
DONE_CHECK_REVIEW_TASK="000-done-check-${DONE_CHECK_REVIEW_ROLE}"
DONE_CHECK_PLANNER_ROLE="planner"
DONE_CHECK_PLANNER_TASK="000-done-check-${DONE_CHECK_PLANNER_ROLE}"
DONE_CHECK_REVIEW_TEMPLATE="${TEMPLATES_DIR}/000-done-check-reviewer.md"
DONE_CHECK_PLANNER_TEMPLATE="${TEMPLATES_DIR}/000-done-check-planner.md"

GITIGNORE_ENTRIES=(
  ".governator/governator.lock"
  ".governator/governator.locked"
  ".governator/failed-merges.log"
  ".governator/in-flight.log"
  ".governator/logs/"
)

LIB_DIR="${STATE_DIR}/lib"

# shellcheck disable=SC1090,SC1091
# shellcheck source=_governator/lib/utils.sh
source "${LIB_DIR}/utils.sh"
# shellcheck source=_governator/lib/logging.sh
source "${LIB_DIR}/logging.sh"
# shellcheck source=_governator/lib/config.sh
source "${LIB_DIR}/config.sh"
# shellcheck source=_governator/lib/git.sh
source "${LIB_DIR}/git.sh"
# shellcheck source=_governator/lib/core.sh
source "${LIB_DIR}/core.sh"
# shellcheck source=_governator/lib/locks.sh
source "${LIB_DIR}/locks.sh"
# shellcheck source=_governator/lib/tasks.sh
source "${LIB_DIR}/tasks.sh"
# shellcheck source=_governator/lib/workers.sh
source "${LIB_DIR}/workers.sh"
# shellcheck source=_governator/lib/review.sh
source "${LIB_DIR}/review.sh"
# shellcheck source=_governator/lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"
# shellcheck source=_governator/lib/branches.sh
source "${LIB_DIR}/branches.sh"
# shellcheck source=_governator/lib/queues.sh
source "${LIB_DIR}/queues.sh"
# shellcheck source=_governator/lib/status.sh
source "${LIB_DIR}/status.sh"
# shellcheck source=_governator/lib/internal.sh
source "${LIB_DIR}/internal.sh"
# shellcheck source=_governator/lib/update.sh
source "${LIB_DIR}/update.sh"

ensure_gitignore_entries() {
  if [[ ! -f "${GITIGNORE_PATH}" ]]; then
    printf '# Governator\n' > "${GITIGNORE_PATH}"
  fi
  local entry
  for entry in "${GITIGNORE_ENTRIES[@]}"; do
    if ! grep -Fqx -- "${entry}" "${GITIGNORE_PATH}" 2> /dev/null; then
      printf '%s\n' "${entry}" >> "${GITIGNORE_PATH}"
    fi
  done
}

init_governator() {
  ensure_db_dir
  ensure_gitignore_entries
  if read_project_mode > /dev/null 2>&1; then
    log_error "Governator is already initialized. Re-run init after clearing ${PROJECT_MODE_FILE}."
    exit 1
  fi

  local project_mode=""
  while true; do
    read -r -p "Is this a new or existing project? (new/existing): " project_mode
    project_mode="$(trim_whitespace "${project_mode}")"
    project_mode="$(printf '%s' "${project_mode}" | tr '[:upper:]' '[:lower:]')"
    if [[ "${project_mode}" == "new" || "${project_mode}" == "existing" ]]; then
      break
    fi
    printf 'Please enter "new" or "existing".\n'
  done

  local remote_name
  read -r -p "Default remote [${DEFAULT_REMOTE_NAME}]: " remote_name
  remote_name="$(trim_whitespace "${remote_name}")"
  if [[ -z "${remote_name}" ]]; then
    remote_name="${DEFAULT_REMOTE_NAME}"
  fi

  local default_branch
  read -r -p "Default branch [${DEFAULT_BRANCH_NAME}]: " default_branch
  default_branch="$(trim_whitespace "${default_branch}")"
  if [[ -z "${default_branch}" ]]; then
    default_branch="${DEFAULT_BRANCH_NAME}"
  fi

  printf '%s\n' "${project_mode}" > "${PROJECT_MODE_FILE}"
  printf '%s\n' "${remote_name}" > "${REMOTE_NAME_FILE}"
  printf '%s\n' "${default_branch}" > "${DEFAULT_BRANCH_FILE}"

  write_manifest "${ROOT_DIR}" "${STATE_DIR}" "${MANIFEST_FILE}"

  printf 'Governator initialized:\n'
  printf '  project mode: %s\n' "${project_mode}"
  printf '  default remote: %s\n' "${remote_name}"
  printf '  default branch: %s\n' "${default_branch}"

  git -C "${ROOT_DIR}" add -A
  if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain 2> /dev/null)" ]]; then
    git -C "${ROOT_DIR}" commit -q -m "[governator] Initialize configuration"
  fi
}

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

# Script entrypoint.
main() {
  ensure_clean_git
  ensure_dependencies
  ensure_db_dir
  require_governator_doc
  require_project_mode
  if handle_locked_state "run"; then
    return 0
  fi
  ensure_lock
  log_verbose "Run start (branch: $(read_default_branch))"
  sync_default_branch
  log_verbose "Sync complete"
  process_worker_branches
  resume_assigned_tasks
  assign_pending_tasks
  commit_audit_log_if_dirty
  git_checkout_default_branch
  log_verbose "Run complete (branch: $(read_default_branch))"
}

dispatch_subcommand "$@"
