# Sub-job: task planning

Act as the task planning agent. Read every milestone and epic produced by the roadmap agent, then emit Markdown task files that capture the work orders needed to implement each epic.

Requirements:
- Tasks must live under `_governator/task-backlog/` (or the configured task queue directory) and use the markdown structure defined in this repo’s task template (`internal/templates/planning/task.md`, which mirrors `v1-reference/_governator/templates/task.md`).
- Every task file must include YAML frontmatter that references the milestone ID (`mX`), epic ID (`eY`), and a unique task number (`task: ###`). Filenames should remain `<id>-<slug>-<role>.md`.
- The body must describe objective, context, requirements, constraints, non-goals, and acceptance criteria, linking back to the roadmap and architecture documents as needed. Explicitly state dependencies on other tasks when known.
- Prefer tasks that represent bounded, single-purpose work (ideally ≤ 8 story points). If no natural decomposition exists, explain why the broader task should remain monolithic.
