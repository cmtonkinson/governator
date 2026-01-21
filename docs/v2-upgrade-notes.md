# Governator v2 Upgrade Notes

These notes help operators and automation teams move from the legacy v1
Governator experience to the deterministic, file-backed v2 pipeline.

## Summary

- Governator v2 is shipped as a system-installed CLI (`init`, `plan`, `run`,
  `status`, `version`). All commands (except `version`) resolve the git root,
  write only under `_governator/`, and report stable exit codes so automation
  stays script-friendly.
- State now lives in `_governator/task-index.json` plus flat task files under
  `_governator/tasks/`. Task markdown is read-only intent, never a state machine.
- Planning always runs bootstrap first, writes the planner prompt into
  `_governator/_local_state/planner/plan-request.md`, and emits `plan ok
  tasks=<n>` when finished. Planning drift is detected via digest checks and
  surfaces a `planning=drift` message that points operators back to
  `governator plan`.
- The run command orchestrates tasks through the test → review →
  conflict-resolution → merge pathway, respects dependencies/concurrency caps,
  and logs every lifecycle transition as `task=<id> role=<role> stage=<stage>
  status=<event>` for easy parsing.

## Upgrade checklist

1. **Install v2** — use your platform package manager (Homebrew for macOS or
   dpkg/apt for Ubuntu, see `docs/system-install-distribution.md`). Verify the
   binary via `governator version`.
2. **Bootstrap** — from an existing repo run `governator init` to regenerate
   `_governator/_durable_state/`, `_governator/_local_state/`, and the config
   scaffolding. This never mutates tracked files outside `_governator`.
3. **Replan** — run `governator plan`. Planning enforces the Power Six artifacts,
   executes the configured planner command (must include `{task_path}`), and
   writes `_governator/task-index.json` plus `_governator/tasks/`. Delete or
   archive any v1 `work/` directories before committing; they are no longer
   honored.
4. **Exercise the new workflow** — use `governator run` to resume work and
   rerun it whenever needed; use `governator status` for a quick tally.

## Key behavioral differences from v1

- **Centralized index** — the only mutable orchestration state is
  `_governator/task-index.json`. Updates flow through the index, not by moving
  directories. Run automatically increments attempt counters, blocks tasks that
  exceed retries, and records digests for drift detection. `status` reads the
  same file and reports totals for `done`, `open`, and `blocked`.
- **Deterministic pipeline** — every run is bootstrap → planning → execution.
  Planning writes prompts into `_governator/_local_state/planner/`, saves the
  canonical index, and prints `plan ok tasks=<n>` once everything lands. The
  run command enforces planning drift detection and prints
  `planning=drift ... next_step="governator plan"` when digests disagree.
- **Stage logging** — `governator run` never prompts. Every agent emits
  `task=<id> role=<role> stage=<stage> status=<start|complete|failure|timeout>`
  plus optional `reason`/`timeout_seconds` fields so automation can react to
  failures, timeouts, and retries.
- **No background retries** — there is no hidden retry loop. Rerun `run` to
  continue, and election of eligible work obeys explicit dependencies, role
  caps, and overlap rules defined in `_governator/task-index.json`.
- **Role-based config and guards** — `config.Config` layers user defaults,
  repo overrides (`_governator/config/`), and CLI flags. Optional auto-rerun
  guards enforce cooldowns and locks before starting work, preventing overlapping
  runs.
- **New status/metadata commands** — `governator status` provides a read-only
  summary; `governator version` prints `version=<semver> commit=<git-sha>
  built_at=<rfc3339>` for verification before invoking other commands.

## Automation guidance

- Scripts that previously monitored `work/` directories should now read
  `_governator/task-index.json` and honor the lifecycle fields there.
- Any tooling that expected ANSI or interactive prompts should be refactored to
  parse the line-oriented output (`task=...`, `planning=drift`, `tasks total=...`).
- If you change `GOVERNATOR.md`, the planner digest will break. Re-run
  `governator plan` before running `governator run`, and consider pinning the
  planner command via `workers.commands.roles["planner"]` so automation controls
  the prompt execution.
- Expect `run` to surface blocked tasks (e.g., retry limit exceeded) as plain
  text so operators can triage. Logistics that need build metadata refer to
  `governator version` instead of parsing banners.

## Additional references

- `GOVERNATOR.md` — the authoritative v2 spec.
- `docs/v2-cli.md` — CLI semantics, command outputs, and pipeline detail.
- `docs/system-install-distribution.md` — package-manager install and upgrade flow.
- `docs/versioning-and-build-metadata.md` — how `governator version` is populated.
