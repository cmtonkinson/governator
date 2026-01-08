# shellcheck shell=bash

#############################################################################
# Internal subcommands (undocumented; intended for testing and ops drills)
#############################################################################
#
# These subcommands are not part of the public interface and may change without
# notice. They exist to make targeted testing and troubleshooting possible.
# Each subcommand still enforces the same safety checks (lock, clean git, deps)
# and operates on real state, so use with care.
#
# Usage:
#   governator.sh process-branches
#   governator.sh assign-backlog
#   governator.sh check-zombies
#   governator.sh cleanup-tmp [--dry-run]
#   governator.sh parse-review <file>
#   governator.sh list-workers
#   governator.sh extract-role <task-file>
#   governator.sh read-caps [role]
#   governator.sh count-in-flight [role]
#   governator.sh format-task-id <number>
#   governator.sh allocate-task-id
#   governator.sh normalize-tmp-path <path>
#   governator.sh audit-log <task> <message>
#
# Subcommand reference:
# - process-branches:
#   Processes only worker branches (including zombie detection and tmp cleanup).
#   This is useful to test review/merge behavior without assigning new work.
#
# - assign-backlog:
#   Assigns only backlog tasks. This is useful to validate filename parsing,
#   role caps, and in-flight handling without processing existing branches.
#
# - check-zombies:
#   Runs zombie detection logic against in-flight workers. If a worker's branch
#   is missing and the worker is dead or timed out, it retries once and blocks
#   on the second failure. Does not process branches or assign backlog.
#
# - cleanup-tmp:
#   Removes stale worker tmp directories in /tmp that are older than the worker
#   timeout and not referenced in the worker process log. Use --dry-run to list
#   candidates without removing them.
#
# - parse-review:
#   Prints the parsed review result and comments from a review.json file.
#
# - list-workers:
#   Prints the available worker roles, one per line.
#
# - extract-role:
#   Prints the role suffix extracted from a task filename (or exits non-zero).
#
# - read-caps:
#   Prints the global cap plus per-role caps. If a role is supplied, prints only
#   that role's cap.
#
# - count-in-flight:
#   Prints the total in-flight count. If a role is supplied, prints only that
#   role's in-flight count.
#
# - format-task-id:
#   Formats a numeric task id to zero-padded 3 digits.
#
# - allocate-task-id:
#   Reserves and prints the next task id (increments the stored counter).
#
# - normalize-tmp-path:
#   Normalizes /tmp paths to their /private/tmp equivalents.
#
# - audit-log:
#   Appends a line to the audit log with the provided task name and message.
#############################################################################

# run_locked_action
# Purpose: Run an internal action under lock and commit audit log if dirty.
# Args:
#   $1: Context string for logging (string).
#   $2+: Command and args to execute.
# Output: Logs lock status and task events.
# Returns: 0 on completion; propagates command exit status.
run_locked_action() {
  local context="$1"
  shift
  ensure_ready_with_lock
  if handle_locked_state "${context}"; then
    return 0
  fi
  "$@"
  commit_audit_log_if_dirty
}

# parse_run_args
# Purpose: Parse run subcommand flags.
# Args:
#   $@: Arguments passed to the run command.
# Output: Sets GOV_QUIET and GOV_VERBOSE globals.
# Returns: 0 on success; exits 1 on unknown options.
parse_run_args() {
  local arg
  while [[ "$#" -gt 0 ]]; do
    arg="$1"
    case "${arg}" in
      -q | --quiet)
        GOV_QUIET=1
        ;;
      -v | --verbose)
        GOV_VERBOSE=1
        ;;
      --)
        shift
        break
        ;;
      *)
        log_error "Unknown option for run: ${arg}"
        exit 1
        ;;
    esac
    shift
  done
}

# process_branches_action
# Purpose: Sync the default branch and process worker branches.
# Args: None.
# Output: Logs branch processing activity.
# Returns: 0 on completion.
process_branches_action() {
  sync_default_branch
  process_worker_branches
}

# assign_backlog_action
# Purpose: Sync the default branch and assign backlog tasks.
# Args: None.
# Output: Logs assignment activity.
# Returns: 0 on completion.
assign_backlog_action() {
  sync_default_branch
  assign_pending_tasks
}

# check_zombies_action
# Purpose: Sync the default branch and run zombie detection.
# Args: None.
# Output: Logs zombie handling activity.
# Returns: 0 on completion.
check_zombies_action() {
  sync_default_branch
  check_zombie_workers
}

# print_help
# Purpose: Print top-level CLI usage information.
# Args: None.
# Output: Writes help text to stdout.
# Returns: 0 always.
print_help() {
  cat << EOF
Usage: governator.sh <command>

Public commands:
  run      Run the normal full loop.
  init     Configure the project mode and defaults.
          Use --defaults for non-interactive defaults or --non-interactive
          with --project-mode/--remote/--branch.
  update   Replace governator.sh with the latest upstream version.
  status   Show queue counts, in-flight workers, and blocked tasks.
  lock     Prevent new activity from starting and show a work snapshot.
  unlock   Resume activity after a lock.
  unblock  Move a blocked task back to assigned with a note.
  restart  Reset tasks to backlog and remove notes/annotations (use --dry-run to preview).

Options:
  -h, --help   Show this help message.
  run -q, --quiet   Suppress stdout during run (errors still surface).
  run -v, --verbose  Print worker/reviewer command lines.

Note: You must run `governator.sh init` before using any other command.
Last updated at: $(read_last_update_at)
EOF
}

# dispatch_subcommand
# Purpose: Dispatch CLI subcommands to their handlers.
# Args:
#   $1: Subcommand name (string).
#   $2+: Subcommand arguments.
# Output: Writes help text and command output to stdout/stderr.
# Returns: 0 on success; exits 1 on unknown subcommand.
dispatch_subcommand() {
  local cmd="${1:-}"
  if [[ -z "${cmd}" ]]; then
    print_help
    return 1
  fi
  case "${cmd}" in
    -h | --help)
      print_help
      return 0
      ;;
  esac

  if [[ "${cmd}" != "init" && "${cmd}" != "update" ]]; then
    if ! require_project_mode; then
      return 1
    fi
  fi
  shift || true

  case "${cmd}" in
    run)
      parse_run_args "$@"
      main
      ;;
    init)
      init_governator "$@"
      ;;
    update)
      update_governator "$@"
      ;;
    status)
      ensure_db_dir
      status_dashboard
      ;;
    lock)
      ensure_db_dir
      local red="\033[1;31m"
      local reset="\033[0m"
      if system_locked; then
        local since
        if since="$(locked_since)"; then
          printf 'Governator already %blocked%b since %s\n' "${red}" "${reset}" "${since}"
        else
          printf 'Governator already %blocked%b\n' "${red}" "${reset}"
        fi
      else
        lock_governator
        printf 'Governator %blocked%b at %s\n' "${red}" "${reset}" "$(locked_since)"
      fi
      ;;
    unlock)
      ensure_db_dir
      local green="\033[1;32m"
      local reset="\033[0m"
      if system_locked; then
        unlock_governator
        printf 'Governator %bunlocked%b\n' "${green}" "${reset}"
      else
        printf 'Governator already %bunlocked%b\n' "${green}" "${reset}"
      fi
      ;;
    abort)
      ensure_ready_no_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: abort <task-prefix>"
        exit 1
      fi
      abort_task "${1}"
      ;;
    unblock)
      ensure_ready_no_lock
      if [[ -z "${1:-}" || -z "${2:-}" ]]; then
        log_error "Usage: unblock <task-prefix> <note>"
        exit 1
      fi
      local prefix="${1}"
      shift
      unblock_task "${prefix}" "$@"
      ;;
    restart)
      ensure_ready_no_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: restart [--dry-run] <task-prefix> [task-prefix ...]"
        exit 1
      fi
      restart_tasks "$@"
      ;;
    process-branches)
      run_locked_action "processing worker branches" process_branches_action
      ;;
    assign-backlog)
      run_locked_action "assigning backlog tasks" assign_backlog_action
      ;;
    check-zombies)
      run_locked_action "checking zombie workers" check_zombies_action
      ;;
    cleanup-tmp)
      ensure_ready_with_lock
      cleanup_stale_worker_dirs "${1:-}"
      ;;
    parse-review)
      ensure_ready_with_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: parse-review <file>"
        exit 1
      fi
      parse_review_json "${1}"
      ;;
    list-workers)
      ensure_ready_with_lock
      list_available_workers
      ;;
    extract-role)
      ensure_ready_with_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: extract-role <task-file>"
        exit 1
      fi
      if ! extract_worker_from_task "${1}"; then
        exit 1
      fi
      ;;
    read-caps)
      ensure_ready_read_only
      if [[ -n "${1:-}" ]]; then
        read_worker_cap "${1}"
      else
        local global_cap
        global_cap="$(read_global_cap)"
        printf 'global %s\n' "${global_cap}"
        local role
        while IFS= read -r role; do
          printf '%s %s\n' "${role}" "$(read_worker_cap "${role}")"
        done < <(list_available_workers)
      fi
      ;;
    count-in-flight)
      ensure_ready_with_lock
      if [[ -n "${1:-}" ]]; then
        count_in_flight "${1}"
      else
        count_in_flight
      fi
      ;;
    format-task-id)
      ensure_ready_with_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: format-task-id <number>"
        exit 1
      fi
      format_task_id "${1}"
      ;;
    allocate-task-id)
      ensure_ready_with_lock
      allocate_task_id
      ;;
    normalize-tmp-path)
      ensure_ready_with_lock
      if [[ -z "${1:-}" ]]; then
        log_error "Usage: normalize-tmp-path <path>"
        exit 1
      fi
      normalize_tmp_path "${1}"
      ;;
    audit-log)
      ensure_ready_with_lock
      if [[ -z "${1:-}" || -z "${2:-}" ]]; then
        log_error "Usage: audit-log <task> <message>"
        exit 1
      fi
      local task_name="${1}"
      shift
      log_task_event "${task_name}" "$*"
      ;;
    *)
      log_error "Unknown subcommand: ${cmd}"
      exit 1
      ;;
  esac
}
