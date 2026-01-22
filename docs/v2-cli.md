# Governator v2 CLI Reference

This document captures the deterministic, non-interactive Governator v2
command surface. Refer to `GOVERNATOR.md` for the authoritative system spec;
this page highlights the operational pipeline and observable CLI behavior.

## Pipeline Overview

Governator v2 is intentionally staged: `init` bootstraps the repository layout,
`plan` emits a full task index and flat task files, and `run` iterates every
eligible task to completion. `status` lets operators inspect that index
without mutating state, while `version` reports the installed build metadata.
Commands (except `version`) resolve the git repository root before touching any
files and restrict writes to `_governator/` so execution is auditable.

## Canonical State Layout

- `_governator/_durable_state/`: configuration scaffolding, bootstrap content,
  and any persistent bookkeeping the CLI requires.
- `_governator/_local_state/`: transient data such as planner prompts,
  guard timestamps, and logs that do not belong in the canonical index.
- `_governator/plan/`: planning artifacts emitted by the planner (architecture,
  roadmap, task generation, optional gap analysis).
- `_governator/tasks/`: read-only task markdown that describes each unit of work.
- `_governator/task-index.json`: the single source of truth for task state,
  dependencies, retries, and digests.

## Command Principles

- **Script-friendly output**: CLI logging is line-oriented, lacks ANSI control
  characters, and keeps whitespace stable.
- **Deterministic semantics**: exit `0` for success, `1` when execution fails,
  `2` for misuse (missing repo, invalid subcommand, etc.).
- **Paths**: when paths appear, they are reported relative to the repository root
  to keep automation simple.
- **Scope**: CLI writes only under `_governator/` plus optional preferences in
  `~/.config/governator/`; no other files or directories are mutated.

## `governator init`

1. **Synopsis**  
   `governator init`

2. **What it does**
   - Requires a git repository; exits `2` otherwise after printing the usage
     line (see `cmd/governator/main.go`).
   - Ensures `_governator/_durable_state/`, `_governator/_local_state/`, and
     the config layout exist by running `config.InitFullLayout`.
   - Does not touch tracked files outside `_governator` except `~/.config/governator/`.
   - Prints `init ok` on success and exits `1` when filesystem operations fail.

3. **Example output**
   ```
   init ok
   ```

## `governator plan`

1. **Synopsis**  
   `governator plan`

2. **What it does**
   - Ensures bootstrap artifacts exist; if any required Power Six document is
     missing, it runs the bootstrap stage automatically and prints `bootstrap ok`.
   - Loads `config.Config` to discover planner commands. It prefers
     `workers.commands.roles["planner"]`, falls back to the default worker
     command, and requires every template to include `{task_path}` so the
     planner prompt path can be injected.
   - Assembles the planner prompt, writes it under
     `_governator/_local_state/planner/plan-request.md`, and executes the planner
     command in the repository root.
   - Parses the planner output, writes the flat task files under
     `_governator/tasks/`, saves digests and task metadata to
     `_governator/task-index.json`, and preserves plan artifacts (`architecture`,
     `roadmap`, `tasks`, and optionally `gap-analysis`) in `_governator/plan/`.
   - Keeps planning deterministic by validating prompt order, planner exit
     statuses, and digest computation before updating the index.

3. **Example output**
   ```
   bootstrap ok
   plan ok tasks=42
   ```

## `governator run`

1. **Synopsis**  
   `governator run`

2. **What it does**
   - Reads `_governator/task-index.json` and enforces planning digests. If
     digests diverge, it prints a deterministic message such as
     `planning=drift status=blocked reason="... the change ..." next_step="governator plan"`
     and fails so the operator can rerun `plan`.
   - Applies any configured auto-run guard (cooldown and locking) before
     executing work. If the guard blocks execution, the run exits without
     mutating state and emits a descriptive message.
   - Detects resume candidates for tasks that were previously worked or tested,
     prepares their worktrees, increments attempt counters, and logs
     `Resuming task ...`.
   - Orchestrates each stage in order: work → test → review → conflict resolution →
     merge.
   - Ensures all open tasks have an accompanying branch via
     `EnsureBranchesForOpenTasks`.
   - Emits task lifecycle events to stdout using the pattern
     `task=<id> role=<role> stage=<stage> status=<start|complete|failure|timeout> ...`,
     and surfaces rich failure reasons and timeout meta fields (e.g.,
     `timeout_seconds=900`), so automation can grep the stream.
   - Saves the updated index whenever tasks are resumed, blocked, tested,
     reviewed, resolved, or orphan branches are created.
   - Rerunning `governator run` picks up where it left off; there is no
     separate resume command.

3. **Example output**
   ```
   Resuming task task-11 (attempt 2)
   task=task-11 role=engineer stage=test status=start
   task=task-11 role=engineer stage=test status=complete
   task=task-12 role=reviewer stage=conflict status=failure reason="merge conflict"
   planning=drift status=blocked reason="GOVERNATOR.md changed" next_step="governator plan"
   ```

## `governator status`

1. **Synopsis**  
   `governator status`

2. **What it does**
   - Reads the centralized `_governator/task-index.json` without mutating
     repository state.
   - Counts tasks by state and reports totals for `done`, `open`, and `blocked`.
     `blocked` includes tasks in the `blocked` or `conflict` states; `open`
     includes tasks that are `open`, `worked`, `tested`, or `resolved`.

3. **Example output**
   ```
   tasks total=82 done=16 open=65 blocked=1
   ```

## `governator version`

1. **Synopsis**  
   `governator version`

2. **What it does**
   - Prints build metadata from `internal/buildinfo` on a single line:
     `version=<semver> commit=<git-sha> built_at=<rfc3339>`.
   - Useful for automation to confirm the installed binary before running any
     other command.

3. **Example output**
   ```
   version=2.0.0 commit=abcdef0 built_at=2025-02-14T09:30:00Z
   ```
