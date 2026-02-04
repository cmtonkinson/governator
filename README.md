# Governator v2

Governator is a Git-native, deterministic engineer orchestration system. The operator writes their vision in `GOVERNATOR.md`, Governator iterates through a fully auditable planning pipeline, and autonomous workers implement the resulting tasks inside isolated worktrees. There is no hidden state—every decision, prompt, and artifact lives on disk or in git—and the CLI is written in Go so you get strong typing, structured logs, and native concurrency once planning completes.

## Vision & Core Ideas
- **Two-phase execution:** the CLI splits its work into a serial planning phase (planning supervisor → architecture, gap analysis, roadmap, tasks) and a parallel execution phase (task work ⇒ test ⇒ review ⇒ merge). Both phases reuse the same worker/dispatch/state plumbing.
- **Git-native state machine:** each planning step and execution task runs in a dedicated worktree/branch, commits artifacts/conflicts, and merges back when clean. `_governator/_local-state` tracks supervisors, worktrees, and in-flight workers while `_governator/_durable-state` stores configuration that should live in git.
- **Deterministic prompts:** workers read the reasoning prompt (optional), worker contract, role prompt, custom prompts, then the task itself. Prompt files are stitched together by the staging helpers so every worker run is repeatable.
- **Planning as code:** `planning.json` (version 2) declares each planning step, its role, and any validations (command, file, directory, prompt). Defaults cover architecture baseline, gap analysis, roadmap, and task planning, ensuring the Power Six architecture docs, milestone/epic planning, and `_governator/tasks` are created before execution.
- **Worker accountability:** every worker command template must include `{task_path}` (or `{prompt_path}`), and the dispatch logic exposes environment variables such as `GOVERNATOR_TASK_ID`, `GOVERNATOR_WORKTREE_DIR`, and `GOVERNATOR_PROMPT_PATH`. Exit reasons are recorded via `exit.json` and the audit logger for debugging.

## Quick Start
1. **Describe your intent** in `GOVERNATOR.md` (overview, goals/non-goals, constraints, requirements, definition of done). This file is the single source of project intent and is never mutated by workers.
2. **Initialize** the repo with `governator init`. This bootstraps `_governator/` (durable config, prompts, templates, planning spec, worker contract, `.gitignore`, etc.) and seeds the planning index.
3. **Run planning** with `governator plan`. This command launches a planning supervisor that executes the configured planning steps sequentially, validates the outputs, merges each worktree back to `main`, and indexes `_governator/tasks`. Use `--supervisor` only if you need to attach to the supervisor process directly.
4. **Execute tasks** with `governator execute`. After planning completes, this command launches the execution supervisor in the background; it loads `_governator/index.json`, applies concurrency caps, resumes stalled work, and dispatches workers through the work/test/review/conflict lifecycle. Use `--supervisor` to attach to the supervisor process directly.
5. **Inspect status** with `governator status` to see supervisor state, planning progress, and detailed task rows (state, PID, role, attributes, title).
6. **Control the supervisor** with `governator stop`, `governator restart`, or `governator reset`. Add `--worker` to stop or kill the active worker process as well.
7. **Check the version** with `governator version`. Avoid `governator run`—it still exists for backwards compatibility but simply calls plan+execute with a warning.

## Architecture Overview

### Planning Phase (serial, deterministic)
- Planning steps are defined in `_governator/planning.json` and default to architecture-baseline, gap-analysis, project-planning, and task-planning. Each step names a prompt under `_governator/prompts/`, a role prompt under `_governator/roles/`, and any validations it must satisfy.
- Validation types cover:
  1. `command`: run a CLI and expect exit code 0 plus optional stdout checks.
  2. `file`: assert files/globs exist (with optional regex content checks).
  3. `directory`: assert directories exist.
  4. `prompt`: ensure reasoning/prompt files are present before the worker executes.
- Each step runs in a dedicated worktree branch, stages prompts and env files, dispatches according to the configured worker command, and merges back to the base branch (default `main`) once `exit.json` reports success.
- `internal/run/task_inventory.go` scans `_governator/tasks` after planning and seeds new execution tasks into `_governator/index.json` so the execution phase can pick them up.
- The planning supervisor stores state/logs under `_governator/_local-state/planning_supervisor/` and can be managed via the CLI, ensuring only one supervisor runs at a time.

### Execution Phase (parallel worker-driven)
- Tasks flow through the lifecycle defined in `internal/index`: `backlog → triaged → implemented → tested → reviewed → mergeable → merged` (plus `blocked`, `conflict`, and `resolved`). The scheduler enforces global and per-role concurrency caps from the config file.
- Worker staging arranges prompts in the deterministic order: reasoning prompt (optional), `_governator/worker-contract.md`, role prompt, custom prompts, and the task file. It writes `prompt.md`, `prompt-files.txt`, `env`, and `exit.json` into `_governator/_local-state/worker/<stage>/<task-id>`.
- The worker command template (configured under `workers.commands`) substitutes tokens `{task_path}`, `{prompt_path}`, `{repo_root}`, and `{role}`. `{task_path}` is required (or `{prompt_path}` when you pre-bake prompts manually).
- Timeouts, retries, and branch base customization are available through the same config.
- Audit logs, exit codes, merge conflicts, and task attributes (blocked reason, PID, attempts) are persisted so you can reason about every step.

## Configuration & Prompts
- `_governator/_durable-state/config.json` stores the canonical config. Key sections are:
  - `workers.cli`: Select which AI CLI to use. Built-in support for:
    - **`"codex"`** (default): Uses `codex exec --sandbox=workspace-write {prompt_path}`
    - **`"claude"`**: Uses `claude --print {prompt_path}`
    - **`"gemini"`**: Uses `gemini {prompt_path}`
    - Set `workers.cli.default` for the default CLI and `workers.cli.roles` for per-role overrides
  - `workers.commands`: Optional custom command overrides (advanced usage). If set, takes precedence over `workers.cli`. Every command must contain `{task_path}` or `{prompt_path}`.
  - `concurrency`: `global`, `default_role`, and `roles` caps to control parallelism.
  - `timeouts.worker_seconds`: how long a worker can run before timing out and blocking the task.
  - `retries.max_attempts`: how many automatic attempts before a task is blocked.
  - `branches.base`: the branch that new worktrees merge into (`main` by default).
  - `reasoning_effort`: default level plus optional per-role overrides (e.g., `low`, `medium`, `high`). Codex uses CLI flags (`--config model_reasoning_effort="high|low"`), while Claude Code and Gemini use reasoning prompt files from `_governator/reasoning/`.
- Prompt assets and templates:
  - Architecture and planning templates (ASR, arc42, personas, Wardley, C4, ADR, milestones, epics, etc.) live under `_governator/templates` and are populated at init.
  - Role prompts (`architect.md`, `default.md`, `planner.md`) plus optional custom prompts `_global.md` and per-role overrides let you inject guardrails or extra instructions.
  - `_governator/worker-contract.md` codifies non-negotiable worker behavior.
  - `_governator/prompts/` contains the prompts invoked during planning (default four steps).

## Directory Layout
- `_governator/docs/`: planning docs (architecture, gap ledger, milestones, epics, etc.) often generated from templates in `internal/templates/bootstrap` and `internal/templates/planning`.
- `_governator/tasks/`: execution task markdown files. Governance expects human-readable prompts with `# Title` headings (used by the task inventory to auto-create index entries).
- `_governator/index.json`: canonical task registry used by `governator execute`, `status`, and the scheduler.
- `_governator/_durable-state/`: tracked config, migrations, and other durable metadata.
- `_governator/_local-state/`: runtime artifacts—planning supervisor logs, worker staging, in-flight entries; this directory is gitignored except for `.keep` files.
- `_governator/roles`, `_governator/custom-prompts`, `_governator/reasoning`: prompt fragments used to control agent behavior.
- `.keep` files inside every required directory keep empty trees checked into git.

## CLI Reference
- `governator init`: create the v2 directory layout, seed configs, planning spec, prompts, and worker contract, and commit `_governator` as `Governator initialized`.
- `governator plan`: run the planning supervisor (background by default). It executes each planning step in order, validates the outputs, and merges or fast-forwards the worktrees into `branches.base`. The log is kept at `_governator/_local-state/planning_supervisor/supervisor.log`.
- `governator execute`: run the execution supervisor (background by default). It resumes or dispatches work/test/review/conflict stages after planning finishes, respecting concurrency caps, timeouts, and retries.
- `governator status`: prints supervisor info, planning step progress, and a table of in-progress execution tasks with their state, PID, role, and attributes.
- `governator stop [--worker|-w]`: stop the planning supervisor and optionally the current worker process.
- `governator restart [--worker|-w]`: stop (and optionally kill) the supervisor/worker, then `governator plan` again.
- `governator reset [--worker|-w]`: stop everything, wipe `planning_supervisor` state/logs, and allow a fresh plan run.
- `governator run`: deprecated wrapper that calls `plan` → `execute` with a warning. Prefer the explicit sequence for clarity.
- `governator version`: print build info.

## Monitoring & Recovery
- Planning supervisors persist state (phase, step, PIDs, errors) in `_governator/_local-state/planning_supervisor/state.json` so CLI commands can avoid duplicate planners.
- Execution tracks in-flight tasks via `_governator/_local-state/inflight.json` so tasks can resume after restarts.
- When a worker times out, exceeds retries, or exits non-zero, the scheduler logs the reason, atomically updates `_governator/index.json`, and moves the task to `blocked` (with the option to unblock manually).

## Testing & Verification
- Run the full suite with `./test.sh` (Lua script runs the native Go tests then the E2E planning test) to ensure the architecture pipeline still passes. The script requires a working Go toolchain.
- Target the E2E scenario directly with `go test -v -run TestE2EPlanning ./test`.
- `test/test-worker.sh` is a deterministic worker that reads `test/fixtures/worker-actions-realistic.yaml`, matches prompt regexps, and executes file actions instead of calling a real LLM. `yq` must be installed to parse the fixtures (e.g., `brew install yq`).
- Use `GOVERNATOR_TEST_FIXTURES` to point the test worker at custom fixture files.
- Sample intent lives at `test/testdata/GOVERNATOR.md` for reference.
- See `test/README.md` for additional debugging tips.

## Additional Resources
- `current_status.md` documents the implementation status, test health (all planning tests passing), and remaining gaps.
- The ADR collection under `docs/adr/` explains the planning prompts (`0002`), compound-task planning (`0004`), workstream interface (`0005`), indexing tasks (`0006`), supervisor behavior (`0011`), and more.
- The old `v1-reference/README.md` pairs with this file for historical context if you want to compare the Bash-era workflow.

Governator relies on discipline more than clever magic: keep `GOVERNATOR.md` honest, guard your prompts, and all state will remain auditable and git-controlled. Ready for v2? Run `governator init`, then watch the planning pipeline write the architecture and task backlog for you.
