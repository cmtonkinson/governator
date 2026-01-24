# ADR 0002: Plan as serial agent prompts that emit Markdown

Date: 2026-01-22

## Status

Accepted

## Context

Governator v2 previously treated planning as a deterministic JSON pipeline: a single planner output was parsed into architecture baseline, gap analysis, roadmap, and task generation payloads, each of which had a schema version, a `kind`, and a strict JSON contract that the CLI validated before emitting plan artifacts.

The new intent is to run four independent agents (baseline, gap, roadmap, tasking) in sequence, with each agent writing Markdown documents instead of structured JSON payloads. The architecture baseline agent produces the Power Six artifacts under `_governator/docs/`, the gap analysis agent emits Markdown gap reports, the roadmap agent produces canonical `milestones.md` and `epics.md`, and the task planning agent writes Markdown tasks under `_governator/tasks/`. Because there is no longer a single JSON blob flowing through the system, the schema validation/plan parsing plumbing is dead weight.

> **Note:** The standalone `governator plan` helper described later in this ADR has been removed; planning phases now execute automatically via `governator run`, which reuses the templates/prompts described below.

## Decision

- Remove the JSON planner contract entirely and stop emitting `_governator/plan/*.json` and the supporting schema validation/plan parsing logic for the arch/gap/roadmap/task agents. The planner now expects the agents to write Markdown artifacts and the CLI no longer tries to parse them.
- Ensure the planning artifacts exist, digests for `_governator/docs/` are computed, and four Markdown prompts (`architecture-baseline.md`, `gap-analysis.md`, `roadmap.md`, `task-planning.md`) live under `_governator/_local-state/plan/`. Each prompt combines curated templates with `GOVERNATOR.md`, the current docs, and any existing `_governator/tasks` backlog so that the scheduled agents operate deterministically.
- Treat the plan phase as a manual sequence: each agent consumes the prompts referenced above, runs serially, and emits Markdown outputs (the Power Six baseline docs, a gap report, milestones/epics, and Markdown task files). Downstream automation or agents consume these artifacts directly instead of relying on parser-generated structs.

## Consequences

- The plan phase is no longer machine-validated; mistakes in the Markdown output must be caught by human reviewers or downstream automation that reads the docs. The new templates must therefore stay precise and be updated when expectations change.
- The planning templates still produce the Power Six docs and prompts, but the CLI intentionally leaves `_governator/task-index.json` and `_governator/tasks` untouched; those files must be created by the task planning agent or subsequent tooling.
- Removing the JSON parser eliminates a significant chunk of code, simplifying maintenance, but it also means the CLI no longer enforces schema versions for planning artifacts. The responsibility for structural integrity shifts to the agent prompts, the Markdown templates, and the human reviewers who run the agents.
