# Sub-job: task generation

Produce the task generation JSON object defined in `specs/v2-planning-subjobs.md`.

Requirements:
- Output JSON only.
- Include `schema_version` and `kind: "task_generation"`.
- Provide ordered tasks with deterministic `order` values.
- Include `dependencies`, `overlap`, `acceptance_criteria`, and `tests` for each task.
