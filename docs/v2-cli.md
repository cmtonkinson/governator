# Governator v2 CLI Reference

This document captures the deterministic, non-interactive Governator v2
command surface. It mirrors the CLI contract in `specs/v2-cli-contract.md`
and explains the behaviors operators and automation should expect when
running `init`, `plan`, `run`, `status`, and `version`.

## Command Principles

- **Script-friendly**: All output is line-oriented, stable, and free of ANSI.
- **Deterministic semantics**: Commands never prompt, and exit codes are
  consistent (`0` success, `1` execution failure, `2` misuse such as missing
  repo or invalid invocation).
- **Paths**: Where practical, paths are reported relative to the repo root.
- **Refer to** `specs/v2-cli-contract.md` for the full contract including
  exit codes, example usage lines, and unsupported command handling.

## `governator init`

1. **Synopsis**  
   `governator init`

2. **What it does**
   - Creates the `_governator/_durable_state/` directories, config scaffolding,
     and any other repo-local state needed before planning or execution.
   - Is idempotent; running multiple times simply ensures the directories exist.
   - Exits with code `1` if it cannot write to the filesystem.

3. **Example output**
   ```
   init ok
   ```

## `governator plan`

1. **Synopsis**  
   `governator plan`

2. **What it does**
   - Ensures bootstrap artifacts exist and automatically runs the bootstrap
     stage when required.
   - Emits the canonical task index and flat task files under `_governator/tasks/`.
   - Validates deterministic ordering to keep planning repeatable.
   - Prints `bootstrap ok` when bootstrap runs (or is already satisfied) and
     `plan ok` once the index is emitted.
   - Fails with code `1` if planner logic or IO operations encounter errors.

3. **Example output**
   ```
   bootstrap ok
   plan ok
   ```

## `governator run`

1. **Synopsis**  
   `governator run`

2. **What it does**
   - Executes eligible tasks from the index until completion, respecting
     dependencies, caps, and overlap rules.
   - Logs task lifecycle events (`start`, `timeout`, `finish`, `failure`) to
     standard output; every message prefixes `task` for easy grepping.
   - Detects timeouts and task failures, printing a final `run failed` line and
     exiting with code `1`.
   - No separate `resume` command is required; rerunning `governator run`
     resumes based on the current index state.

3. **Example output**
   ```
   task start id=T-014 role=tester
   task timeout id=T-014 role=tester after=10m
   run failed
   ```

## `governator status`

1. **Synopsis**  
   `governator status`

2. **What it does**
   - Reads the centralized task index and reports a compact summary line that
     includes totals for `done`, `open`, and `blocked` work.
   - Never mutates repository state.

3. **Example output**
   ```
   tasks total=82 done=16 open=66 blocked=0
   ```

## `governator version`

1. **Synopsis**  
   `governator version`

2. **What it does**
   - Prints the Governator v2 version string plus any build metadata (e.g.,
     commit hash or build number) on a single line.
   - Useful for automation to confirm compatibility before running other commands.

3. **Example output**
   ```
   governator v2.0.0+build.123
   ```
