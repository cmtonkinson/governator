# Task 000: Task Planning (Planner)

## Objective
Translate approved milestones and epics into concrete, executable tasks.

## Inputs
- The main project `GOVERNATOR.md` file.
- The milestones at `_governator/docs/milestones.md`.
- The epics at `_governator/docs/epics.md`.
- Existing task files for context.

## Output
Create the tasks needed to implement the defined epics in
`_governator/task-backlog/`, using the standard task template at
`_governator/templates/task.md`. Be sure to include the correct milestone and
epic numbers in the YAML frontmatter.

Each task file:
- must be part of the work required to implement a documented epic
- must be marked with the correct YAML frontmatter milestone, epic, and task
  identifiers (e.g. `milestone: m1`, `epic: e3`, `task: 024`)
- must include only one logical work order; any task which would be estimated at
  more than 8 fibonacci story points should be split into multiple tasks
- must be named according to the strict format: `<id>-<kebab-case-title>-<role>.md`
  (example: `001-exchange-adapter-generalist.md`)
- must not change milestones or epics
