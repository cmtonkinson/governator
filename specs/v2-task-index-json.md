<!--
File: specs/v2-task-index-json.md
Purpose: Define the JSON structure for the Governator v2 task index.
-->
# Governator v2 Task Index JSON

Concise specification for the task index JSON file that drives scheduling and
state transitions.

## Format
- JSON object; unknown keys are ignored.
- Best-effort parsing: invalid or missing values fall back to defaults.
- Paths are repo-relative unless explicitly stated otherwise.

## Digest algorithm
- Algorithm: SHA-256 over raw file bytes with no normalization (no newline
  conversion, trimming, or encoding changes).
- Encoding: lowercase hex with a `sha256:` prefix.
- Compatibility: matches standard tools (`shasum -a 256`, `sha256sum`, `openssl
  dgst -sha256`); no known conflicts with existing tooling.
- Sample: file bytes `governator\n` digest to
  `sha256:328961dd5885fa93c7c1f184d3489723f202e870088c9ae747f1454dc406176a`.

## Top-level fields
`schema_version` (number, required)
- Schema version for the index format.

`digests` (object, required)
- `governator_md` (string, required): digest of `GOVERNATOR.md`.
- `planning_docs` (object, required): map of planning artifact path -> digest.

`tasks` (array, required)
- List of task entries. The scheduler only reads state from this list.

## Task fields
`id` (string, required)
- Stable task identifier.

`title` (string, optional)
- Human-friendly description for the task.

`path` (string, required)
- Relative path to the task markdown file.

`state` (string, required)
- Lifecycle state. Values are defined by the lifecycle state machine.

`role` (string, required)
- Role label for worker routing and caps.

`dependencies` (array of strings, required)
- Task ids that must complete before this task is eligible.

`retries` (object, required)
- `max_attempts` (number, required): maximum attempts before blocking.

`attempts` (object, required)
- `total` (number, required): total attempts so far.
- `failed` (number, required): failed attempts so far.

`order` (number, required)
- Execution ordering hint. Lower numbers are scheduled first when eligible.

`overlap` (array of strings, required)
- Overlap flags indicating likely shared code surfaces. Tasks sharing any flag
  are treated as overlapping for parallelism.

## Defaults
- Missing `dependencies` or `overlap` default to empty arrays.
- Missing `attempts` defaults to `{ "total": 0, "failed": 0 }`.
- Missing `retries` defaults to `{ "max_attempts": 0 }`.
- Missing `order` defaults to `0`.
- Missing `state` defaults to `open`.

## Example: single task
```json
{
  "schema_version": 1,
  "digests": {
    "governator_md": "sha256:ef3a4c...",
    "planning_docs": {}
  },
  "tasks": [
    {
      "id": "task-01",
      "title": "Initialize repo",
      "path": "_governator/tasks/task-01-initialize.md",
      "state": "open",
      "role": "planner",
      "dependencies": [],
      "retries": {
        "max_attempts": 2
      },
      "attempts": {
        "total": 0,
        "failed": 0
      },
      "order": 10,
      "overlap": []
    }
  ]
}
```

## Example: multi-level plan
```json
{
  "schema_version": 1,
  "digests": {
    "governator_md": "sha256:ef3a4c...",
    "planning_docs": {
      "_governator/plan/milestone-1.md": "sha256:aa11bb...",
      "_governator/plan/epic-2.md": "sha256:cc22dd..."
    }
  },
  "tasks": [
    {
      "id": "task-01",
      "title": "Define index format",
      "path": "_governator/tasks/task-01-define-index.md",
      "state": "done",
      "role": "architect",
      "dependencies": [],
      "retries": {
        "max_attempts": 2
      },
      "attempts": {
        "total": 1,
        "failed": 0
      },
      "order": 10,
      "overlap": [
        "index"
      ]
    },
    {
      "id": "task-02",
      "title": "Implement index IO",
      "path": "_governator/tasks/task-02-implement-index-io.md",
      "state": "open",
      "role": "engineer",
      "dependencies": [
        "task-01"
      ],
      "retries": {
        "max_attempts": 2
      },
      "attempts": {
        "total": 0,
        "failed": 0
      },
      "order": 20,
      "overlap": [
        "index"
      ]
    },
    {
      "id": "task-03",
      "title": "Write index tests",
      "path": "_governator/tasks/task-03-write-index-tests.md",
      "state": "open",
      "role": "tester",
      "dependencies": [
        "task-02"
      ],
      "retries": {
        "max_attempts": 2
      },
      "attempts": {
        "total": 0,
        "failed": 0
      },
      "order": 30,
      "overlap": [
        "index",
        "tests"
      ]
    }
  ]
}
```

## Clarifying notes
- Overlap flags are labels, not booleans. Any shared label blocks parallelism.
- `order` is only a hint; dependencies always take precedence.
- Digests are required even if the files are missing; use an empty string when
  a file is absent to make drift detection explicit.
