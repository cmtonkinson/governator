<!--
File: specs/v2-planning-subjobs.md
Purpose: Define the serial planning sub-jobs, their inputs, and outputs.
-->
# Governator v2 Planning Sub-Jobs

## Overview
Planning runs as a strict, serial pipeline. Each sub-job emits a concise,
JSON-parseable artifact that becomes input to the next stage.

Order:
1. Architecture baseline (synthesis or discovery)
2. Gap analysis (optional, brownfield only)
3. Roadmap decomposition (depth/width decisions)
4. Task generation

## Shared conventions
- Output format: JSON object; unknown keys are ignored.
- `schema_version` is required for every output.
- Paths are repo-relative unless noted otherwise.
- Default output location: `_governator/plan/`
  - `architecture-baseline.json`
  - `gap-analysis.json`
  - `roadmap.json`
  - `tasks.json`

## 1. Architecture baseline
Purpose: Summarize the target architecture intent derived from the Power Six
artifacts and repo context.

Inputs:
- Power Six artifacts from bootstrap.
- `GOVERNATOR.md` constraints and non-goals.

Output fields:
`schema_version` (number, required)
`kind` (string, required): `architecture_baseline`
`mode` (string, required): `synthesis` or `discovery`
`summary` (string, required): concise architectural narrative
`components` (array, optional): list of named components/modules
`interfaces` (array, optional): integration points or APIs
`constraints` (array, optional): must-follow rules
`risks` (array, optional): key technical risks
`assumptions` (array, optional): explicit assumptions
`sources` (array, required): list of input doc paths

Example:
```json
{
  "schema_version": 1,
  "kind": "architecture_baseline",
  "mode": "synthesis",
  "summary": "Single binary CLI with internal packages for planning and scheduling.",
  "components": [
    "cmd/governator",
    "internal/planner",
    "internal/index"
  ],
  "constraints": [
    "Deterministic file-backed state",
    "Single task index source of truth"
  ],
  "sources": [
    "_governator/architecture/context.md",
    "GOVERNATOR.md"
  ]
}
```

## 2. Gap analysis (optional)
Purpose: Identify deltas between current repo state and target architecture.
Skipped for greenfield repos.

Inputs:
- Architecture baseline output.
- Repo snapshot/inventory (current modules, constraints, TODOs).

Output fields:
`schema_version` (number, required)
`kind` (string, required): `gap_analysis`
`is_greenfield` (boolean, required)
`skipped` (boolean, required when `is_greenfield` is true)
`gaps` (array, optional): each item lists `area`, `current`, `desired`, `risk`

Greenfield behavior:
- If `is_greenfield` is true, either omit the file entirely or emit a stub
  with `skipped: true`. Downstream stages must proceed either way.

Example:
```json
{
  "schema_version": 1,
  "kind": "gap_analysis",
  "is_greenfield": false,
  "gaps": [
    {
      "area": "planner output",
      "current": "none",
      "desired": "JSON plan artifacts",
      "risk": "medium"
    }
  ]
}
```

## 3. Roadmap decomposition
Purpose: Decide decomposition depth/width and enumerate roadmap items.

Inputs:
- Architecture baseline output.
- Gap analysis output if present.

Output fields:
`schema_version` (number, required)
`kind` (string, required): `roadmap_decomposition`
`depth_policy` (string, required): how deep to decompose (e.g., `epic->task`)
`width_policy` (string, required): target task sizing (e.g., `1-2 days`)
`items` (array, required): list of roadmap nodes

Item fields:
`id` (string, required)
`title` (string, required)
`type` (string, required): `milestone`, `epic`, `feature`, or `task_group`
`parent_id` (string, optional)
`goal` (string, optional)
`order` (number, required)
`overlap` (array, optional): labels for likely shared code surfaces

Example:
```json
{
  "schema_version": 1,
  "kind": "roadmap_decomposition",
  "depth_policy": "epic->task",
  "width_policy": "1-3 days",
  "items": [
    {
      "id": "epic-06",
      "title": "Planner emits full task index",
      "type": "epic",
      "order": 10,
      "overlap": [
        "planner",
        "index"
      ]
    }
  ]
}
```

## 4. Task generation
Purpose: Emit the final, ordered task list with dependencies and task content
inputs for file generation.

Inputs:
- Roadmap decomposition output.
- Architecture baseline output.
- Gap analysis output if present.

Output fields:
`schema_version` (number, required)
`kind` (string, required): `task_generation`
`tasks` (array, required)

Task fields:
`id` (string, required)
`title` (string, required)
`summary` (string, required)
`role` (string, required)
`dependencies` (array, required)
`order` (number, required)
`overlap` (array, required)
`acceptance_criteria` (array, required)
`tests` (array, required)

Example:
```json
{
  "schema_version": 1,
  "kind": "task_generation",
  "tasks": [
    {
      "id": "task-83",
      "title": "Specify planning sub-jobs",
      "summary": "Define the serial planning sub-jobs and their outputs.",
      "role": "planner",
      "dependencies": [],
      "order": 10,
      "overlap": [
        "planning-docs"
      ],
      "acceptance_criteria": [
        "Spec document exists describing each sub-job."
      ],
      "tests": [
        "N/A (spec-only)"
      ]
    }
  ]
}
```

## Notes
- Each output feeds the next stage in order; later stages must not require data
  outside the prior outputs plus `GOVERNATOR.md`.
- The task generation output is the sole source for task file and index
  emission; no additional hidden heuristics are allowed.
