#!/usr/bin/env bash
set -euo pipefail

# TODO: Good God this AI slop seems to get worse by the second. This whole file is just fucked, innit?


ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$ROOT_DIR/.." && pwd)"

GOV_DIR="$REPO_ROOT/_governator"
ROLES_DIR="$GOV_DIR/roles"

BACKLOG="$GOV_DIR/task-backlog"
ASSIGNED="$GOV_DIR/task-assigned"
WORKED="$GOV_DIR/task-worked"
BLOCKED="$GOV_DIR/task-blocked"
DONE="$GOV_DIR/task-done"
FEEDBACK="$GOV_DIR/task-feedback"
PROPOSED="$GOV_DIR/task-proposed"

TMP_BASE="/tmp"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"

log() {
  echo "[governator] $*"
}

require_clean_git() {
  if ! git diff --quiet || ! git diff --cached --quiet; then
    log "working tree not clean; aborting"
    exit 1
  fi
}

ensure_dirs() {
  mkdir -p \
    "$BACKLOG" "$ASSIGNED" "$WORKED" "$BLOCKED" \
    "$DONE" "$FEEDBACK" "$PROPOSED"
}

# -------------------------------------------------------------------
# 1. Review completed or blocked work
# -------------------------------------------------------------------

review_completed_work() {
  shopt -s nullglob
  for task in "$WORKED"/*.md "$BLOCKED"/*.md; do
    log "reviewing $(basename "$task")"

    # Reviewer decision via sgpt
    decision="$(
      sgpt <<EOF
You are the reviewer.

Repository README:
$(cat "$REPO_ROOT/README.md")

Task file:
$(cat "$task")

Decide exactly one of:
- ACCEPT
- REJECT with feedback

Respond with either:
ACCEPT
or
REJECT:
<feedback text>
EOF
    )"

    if [[ "$decision" == "ACCEPT"* ]]; then
      branch="$(git branch --show-current)"

      log "accepting work on branch $branch"
      git checkout main
      git merge --ff-only "$branch"
      git push origin main

      mv "$task" "$DONE/"
    else
      feedback="${decision#REJECT:}"
      fb_file="$FEEDBACK/$(basename "$task")"
      {
        cat "$task"
        echo
        echo "## Reviewer Feedback"
        echo "$feedback"
      } > "$fb_file"

      rm -f "$task"
    fi
  done
}

# -------------------------------------------------------------------
# 2. Accept or reject proposed tasks
# -------------------------------------------------------------------

process_proposals() {
  shopt -s nullglob
  for proposal in "$PROPOSED"/*.md; do
    log "evaluating proposal $(basename "$proposal")"

    decision="$(
      sgpt <<EOF
You are the planner.

Repository README:
$(cat "$REPO_ROOT/README.md")

Proposed task:
$(cat "$proposal")

Decide exactly one:
- ACCEPT
- REJECT

Respond with ACCEPT or REJECT only.
EOF
    )"

    if [[ "$decision" == "ACCEPT" ]]; then
      mv "$proposal" "$BACKLOG/"
    else
      rm -f "$proposal"
    fi
  done
}

# -------------------------------------------------------------------
# 3. Generate new tasks (planner role)
# -------------------------------------------------------------------

generate_tasks() {
  if ls "$BACKLOG"/*.md >/dev/null 2>&1; then
    return
  fi

  log "no backlog tasks; generating new ones"

  sgpt <<EOF > "$BACKLOG/initial-plan-$TIMESTAMP.md"
You are the planner.

Based on the repository README below, produce exactly one task.

The task must:
- be concrete
- be assignable to a single role
- be small enough to complete independently

Repository README:
$(cat "$REPO_ROOT/README.md")
EOF
}

# -------------------------------------------------------------------
# 4. Assign and dispatch one task
# -------------------------------------------------------------------

dispatch_one_task() {
  shopt -s nullglob
  local task
  task="$(ls "$BACKLOG"/*.md 2>/dev/null | head -n1 || true)"
  [[ -z "$task" ]] && return

  filename="$(basename "$task")"
  role="${filename%%--*}"

  if [[ ! -f "$ROLES_DIR/$role.md" ]]; then
    log "unknown role $role; blocking task"
    mv "$task" "$BLOCKED/"
    return
  fi

  branch="work/$role/$TIMESTAMP"
  log "dispatching $filename to $branch"

  git checkout -b "$branch"
  mv "$task" "$ASSIGNED/$filename"
  git add "$ASSIGNED/$filename"
  git commit -m "Assign task $filename"
  git push -u origin "$branch"

  workspace="$TMP_BASE/$(basename "$REPO_ROOT")-$role-$TIMESTAMP"
  git clone -b "$branch" "$REPO_ROOT" "$workspace"

  (
    cd "$workspace"
    codex exec \
      --dangerously-bypass-approvals-and-sandbox \
      --prompt "
Read and follow, in order:
1. _governator/worker_contract.md
2. _governator/roles/$role.md
3. _governator/task-assigned/$filename
"
  )
}

# -------------------------------------------------------------------
# Main loop
# -------------------------------------------------------------------

main() {
  cd "$REPO_ROOT"
  require_clean_git
  ensure_dirs

  review_completed_work
  process_proposals
  generate_tasks
  dispatch_one_task
}

main "$@"
