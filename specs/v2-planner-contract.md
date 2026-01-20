<!--
File: specs/v2-planner-contract.md
Purpose: Define the prompt input and output contract for LLM planning.
-->
# Governator v2 Planner Contract

## Purpose
Define the JSON input/output contract for the planner LLM that emits planning
artifacts used to generate tasks and the task index.

## Inputs
The planner request is a JSON object with these fields:

`schema_version` (number, required)
- Request schema version.

`kind` (string, required)
- Must be `planner_request`.

`governator_md` (object, required)
- `path` (string, required): repo-relative path to `GOVERNATOR.md`.
- `content` (string, required): full file contents.

`power_six` (array, required)
- Each item is an object:
  - `path` (string, required): repo-relative path to the artifact.
  - `title` (string, optional): human-friendly label.
  - `content` (string, required): full file contents.

`config` (object, required)
- Full contents of `config.json` after defaults are applied.

`repo_state` (object, required)
- `is_greenfield` (boolean, required): true for new repos.
- `summary` (string, optional): concise inventory summary.
- `inventory` (array, optional): key paths or modules to aid gap analysis.

## Pipeline semantics
The planner executes a strict, serial pipeline:
1. Architecture baseline (synthesis or discovery)
2. Gap analysis (optional; only when `is_greenfield` is false)
3. Roadmap decomposition (depth/width decisions)
4. Task generation

Pipeline outputs must align with `specs/v2-planning-subjobs.md`.

## Output
The planner response is a JSON object with these fields:

`schema_version` (number, required)
- Response schema version.

`kind` (string, required)
- Must be `planner_output`.

`architecture_baseline` (object, required)
- Matches the architecture baseline output schema.

`gap_analysis` (object, optional)
- Present only when `repo_state.is_greenfield` is false.
- When greenfield, it may be omitted or include `skipped: true`.

`roadmap` (object, required)
- Matches the roadmap decomposition output schema.
- Must express variable decomposition depth via `depth_policy` and item types.

`tasks` (object, required)
- Matches the task generation output schema.
- Each task must include `dependencies`, `order`, and `overlap` fields.

`notes` (array of strings, optional)
- Required when output could be interpreted multiple ways; clarify intent.

### Output rules
- Output must be JSON only (no prose wrappers).
- Unknown keys are ignored by the caller.
- Dependencies drive ordering; `order` is a deterministic tiebreaker.
- Overlap flags are labels indicating shared code surfaces.

## Example output (single-task plan)
```json
{
  "schema_version": 1,
  "kind": "planner_output",
  "architecture_baseline": {
    "schema_version": 1,
    "kind": "architecture_baseline",
    "mode": "synthesis",
    "summary": "Single CLI binary with minimal planning pipeline.",
    "components": [
      "cmd/governator",
      "internal/planner"
    ],
    "sources": [
      "_governator/architecture/context.md",
      "GOVERNATOR.md"
    ]
  },
  "roadmap": {
    "schema_version": 1,
    "kind": "roadmap_decomposition",
    "depth_policy": "single_task",
    "width_policy": "1 day",
    "items": [
      {
        "id": "task-01",
        "title": "Produce initial plan artifacts",
        "type": "task_group",
        "order": 10
      }
    ]
  },
  "tasks": {
    "schema_version": 1,
    "kind": "task_generation",
    "tasks": [
      {
        "id": "task-01",
        "title": "Produce initial plan artifacts",
        "summary": "Generate planning artifacts and task index for a trivial repo.",
        "role": "planner",
        "dependencies": [],
        "order": 10,
        "overlap": [
          "planning"
        ],
        "acceptance_criteria": [
          "Planning artifacts exist under _governator/plan/.",
          "Task index reflects the single task."
        ],
        "tests": [
          "N/A (spec-only)"
        ]
      }
    ]
  }
}
```

## Example output (multi-epic plan)
```json
{
  "schema_version": 1,
  "kind": "planner_output",
  "architecture_baseline": {
    "schema_version": 1,
    "kind": "architecture_baseline",
    "mode": "discovery",
    "summary": "Planner emits tasks and index artifacts for a staged pipeline.",
    "components": [
      "internal/planner",
      "internal/index"
    ],
    "sources": [
      "_governator/architecture/context.md",
      "GOVERNATOR.md"
    ]
  },
  "gap_analysis": {
    "schema_version": 1,
    "kind": "gap_analysis",
    "is_greenfield": false,
    "gaps": [
      {
        "area": "planning output",
        "current": "none",
        "desired": "JSON plan artifacts",
        "risk": "medium"
      }
    ]
  },
  "roadmap": {
    "schema_version": 1,
    "kind": "roadmap_decomposition",
    "depth_policy": "epic->task",
    "width_policy": "1-3 days",
    "items": [
      {
        "id": "epic-01",
        "title": "Planner contract and parsing",
        "type": "epic",
        "order": 10,
        "overlap": [
          "planner"
        ]
      },
      {
        "id": "epic-02",
        "title": "Task file emission",
        "type": "epic",
        "order": 20,
        "overlap": [
          "planner",
          "index"
        ]
      }
    ]
  },
  "tasks": {
    "schema_version": 1,
    "kind": "task_generation",
    "tasks": [
      {
        "id": "task-10",
        "title": "Specify planner contract",
        "summary": "Document input/output contract for planning.",
        "role": "planner",
        "dependencies": [],
        "order": 10,
        "overlap": [
          "planner"
        ],
        "acceptance_criteria": [
          "Contract document exists with example output."
        ],
        "tests": [
          "N/A (spec-only)"
        ]
      },
      {
        "id": "task-11",
        "title": "Implement planner output parser",
        "summary": "Parse planner JSON output into internal task models.",
        "role": "engineer",
        "dependencies": [
          "task-10"
        ],
        "order": 20,
        "overlap": [
          "planner",
          "index"
        ],
        "acceptance_criteria": [
          "Parser handles all planner output fields."
        ],
        "tests": [
          "Unit tests cover valid and invalid output."
        ]
      }
    ]
  }
}
```

## Clarifying notes
- If the roadmap depth could be interpreted multiple ways, use `notes` to
  explain why a given depth was selected.
- If any task dependencies are non-obvious, add a `notes` entry describing the
  dependency rationale.
