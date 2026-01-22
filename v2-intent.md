# Governator v2 Intent

## Purpose
Create a clear, stable vision for a v2 refactor that keeps Governator small and
readable while simplifying its orchestration model. v2 should feel like a
purpose-built tool with a predictable flow, a single source of truth for task
state, and minimal moving parts.

## Core Ethos (Unchanged)
- Deterministic, auditable execution backed by files and git.
- Explicit constraints and governance over implicit or conversational flow.
- Small, understandable system that favors clarity over cleverness.
- Bounded, reviewable LLM execution with visible artifacts on disk.

## v2 Direction (High Level)
Governator v2 is a hardcoded, staged pipeline:
1. Architecture (the Power Six artifacts)
2. Planning (Waterfall: generate all tasks up front)
3. Execution ("Ralphian" iteration, optionally parallelized)

The orchestration logic is simplified: tasks live in a single flat directory,
state lives in one centralized task index file, and workers iterate within
concurrency and dependency constraints until the queue is complete.

## What "Ralphian Iteration" Means Here
- Workers continuously pull eligible tasks from a shared index.
- Eligibility is determined by explicit dependencies and role caps.
- Task lifecycle is represented as state fields (not directories).
- Human oversight focuses on the index and audit log, not file moves.

## Non-Goals
- Recreate the existing multi-directory state machine.
- Build a general workflow engine or a swarming framework.
- Add elaborate UIs or dashboards ahead of core stability.
- Sacrifice auditability or determinism for speed.

## Single Source of Truth
The authoritative task state is a centralized index file. It must:
- Be easy to read and diff in git.
- Encode state, role, dependencies, and retries.
- Allow deterministic scheduling decisions.
- Treat task files as read-only intent/instructions, not as state.

## Directory Layout (Conceptual)
- `_governator/_durable_state/`: persistent data; config, migrations, etc. (is committed to git)
- `_governator/_local_state/`: transient data; runtime logs, worktrees, audit log, etc. (not committed)
- `_governator/task-index.<ext>`: the canonical task list and metadata.
- `_governator/tasks/`: flat directory of task markdown files.

## Distribution and Install Model
- Governator v2 ships as a Go CLI installed at the system level.
- Target platforms: macOS and Ubuntu first.
- Package manager installs (Homebrew, dpkg) are expected; binary updates are
  handled externally.
- User defaults live under `~/.config/governator/` and are layered with
- per-project overrides from `_governator/_durable_state/config/` (legacy
  `_governator/config/` directories are still honored).
- Config precedence: user defaults -> project overrides -> CLI flags.
- Repo-local state remains under `_governator/`; no writes outside the repo
  except user defaults in the config dir.
- The CLI must resolve the repo root and refuse to run outside a git repo.

## Bootstrap and Planning
- Bootstrap is fixed and always runs first.
- Planning emits a full task list up front.
- Planning can be re-run only with explicit operator intent.

## Scheduling and Concurrency
- Tasks are eligible if dependencies are satisfied and role/global caps allow.
- The scheduler is deterministic given the same index and repo state.
- Retries are explicit and capped per task; no silent retry loops.

## Worker Output and Resilience
- Timeouts and failures are always logged and surfaced in standard output.
- Work is preserved (worktree retained or partial branch committed).
- Follow-up work reuses the same task entry—failed work is left incomplete so it can be explicitly resumed or re-dispatched.

## Success Criteria
- A new user can read the index and understand the system state in minutes.
- The flow from bootstrap to planning to execution is obvious and repeatable.
- Recovery from failure is explicit, logged, and deterministic.
- The system remains small, auditable, and easy to reason about.
