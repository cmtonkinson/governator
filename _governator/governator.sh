#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

#############################################################################
#############################################################################
#
# The Governator
#
# "Come with me if you want to ship"
#
#############################################################################
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
LAST_UPDATE_FILE="${DB_DIR}/last_update_at"

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

# Script entrypoint.
# main
# Purpose: Run the standard governator loop (sync, process, assign).
# Args: None.
# Output: Logs run lifecycle and task activity.
# Returns: 0 on completion.
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
