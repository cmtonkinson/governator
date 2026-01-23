# ADR 0002: Plan as serial agent prompts that emit Markdown

Date: 2026-01-22

## Status

Accepted

## Context

Governator v2 previously treated planning as a deterministic JSON pipeline: a single planner output was parsed into architecture baseline, gap analysis, roadmap, and task generation payloads, each of which had a schema version, a `kind`, and a strict JSON contract that the CLI validated before emitting plan artifacts.

The new intent is to run four independent agents (baseline, gap, roadmap, tasking) in sequence, with each agent writing Markdown documents instead of structured JSON payloads. The architecture baseline agent produces the Power Six artifacts under `_governator/docs/`, the gap analysis agent emits Markdown gap reports, the roadmap agent produces canonical `milestones.md` and `epics.md`, and the task planning agent writes Markdown tasks under `_governator/tasks/`. Because there is no longer a single JSON blob flowing through the system, the schema validation/plan parsing plumbing is dead weight.

## Decision

- Remove the JSON planner contract entirely and stop emitting `_governator/plan/*.json` and the supporting schema validation/plan parsing logic for the arch/gap/roadmap/task agents. The planner now expects the agents to write Markdown artifacts and the CLI no longer tries to parse them.
- Keep `governator plan` as a helper that ensures the Power Six artifacts exist, computes the documented digests for `_governator/docs/` (used by drift detection), and materializes four Markdown prompts in `_governator/_local_state/plan/` (`architecture-baseline.md`, `gap-analysis.md`, `roadmap.md`, `task-planning.md`). Each prompt combines the curated template instructions with `GOVERNATOR.md`, the current docs, and any existing `_governator/tasks` backlog so an agent can operate deterministically.
- Treat the plan phase as a manual sequence: after `gov plan` publishes the prompts, the operator runs each agent serially. Each agent writes Markdown outputs (the Power Six baseline docs, a gap report, milestones/epics, and Markdown task files), and downstream automation/agents consume those Markdown artifacts directly instead of parser-generated structs.

## Consequences

- The plan phase is no longer machine-validated; mistakes in the Markdown output must be caught by human reviewers or downstream automation that reads the docs. The new templates must therefore stay precise and be updated when expectations change.
- `governator plan` still prints `bootstrap ok` when it creates the Power Six docs and `plan ok prompts=4`, but it no longer touches `_governator/task-index.json` or `_governator/tasks`; those files must be created by the task planning agent or subsequent tooling.
- Removing the JSON parser eliminates a significant chunk of code, simplifying maintenance, but it also means the CLI no longer enforces schema versions for planning artifacts. The responsibility for structural integrity shifts to the agent prompts, the Markdown templates, and the human reviewers who run the agents.
