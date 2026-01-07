# Task 000: Gap Analysis (Planner)

## Objective
Translate the completion-check review feedback into actionable and concrete tasks in
order to complete the project (as per `GOVERNATOR.md`).

## Input
- The main project `GOVERNATOR.md` file.
- Any/all architecture artifacts in `_governator/docs/`.
- Any/all project planning artifacts in `_governator/docs/`.

## Output
### 1. Milestones
If a milestones file exists at `_governator/docs/milestones.md`, read and review
it against the project `GOVERNATOR.md` file.

If no milestones file exists, create it as per the template provided at
(`_governator/templates/milestones.md`).

In either case, ensure the milestones as defined in the file are aligned with
the stated intent in the project `GOVERNATOR.md` file. Do not invent scope,
requirements, constraints, or assumptions not stated in the `GOVERNATOR.md`
file. If the current state of milestones is not aligned with the
`GOVERNATOR.md`, you may choose to update existing milestones where appropriate,
or create new ones.

Milestones exist only to guide high-level planning and work sequencing.

Milestones represent the delivery phasese of the project. Each milestone should
represent an articulable advancement of the project, demonstrating meaningful
progress towards the stateg goals.

Milestones should be concise, operational, and used to drive definition of
useful epics for subsequent task planning.

Small projects like one-off utilites, proofs-of-concept, or toy programs may
only have a single ("Delivery") milestone, whereas larger, more complex, or
production-grade projects may have many, depending on the scope and
architecture.

Start milestone identifiers at 1 prefixed with a lowercase "m" (e.g. "m1"), and
increment by 1 for each subsequent milestone.

### 2. Epics
If an epics file exists at `_governator/docs/epics.md`, read and review it
against the defined project milestones at `_governator/docs/milestones.md`.

If no epics file exists, create it as per the template provided at
(`_governator/templates/epics.md`).

In either case, ensure the epics as defined in the file are aligned with the
defined milestones. Do not invent scope, requirements, constraints, or
assumptions not stated in the milestones. If the current state of epics is not
aligned with defined milestones, you may choose to update existing epics where
appropriate, or create new ones.

Each epic:
- exists to help bridge the scope/effort gap break milestone work into smaller,
  more manageable tasks
- must map to exactly one milestone
- represent a clear, coherent user- or system-visible capability
- must include epic-level in-scope and out-of-scope definitions
- must include an epic-level definition of "done"
- must not include tasks or implementation steps

A milestone should usually contain about 3-7 epics.

Start epic identifiers at 1 prefixed with a lowercase "e" (e.g. "e1"), and
increment by 1 for each subsequent epic.

### 3. Tasks
Read and review existing tasks (except "done") for context. Where gaps were 
identified, and now certain epics exist without all the necessary tasks required
to implement them, create those new tasks in `_governator/task-backlog/` using
the standard task template at `_governator/templates/task.md`. Be sure to
include the correct milestone and epic numbers in the YAML frontmatter.

Each task file:
- must be part of the work required to implement a documented epic
- must be marked with the correct YAML frontmatter milestone, epic, and task
  identifiers (e.g. `milestone: m1`, `epic: e3`, `task: 024`)
- must include only one logical work order; any task which would be estimated at
  more than 8 fibonacci story points should be split into multiple tasks
- must be named according to the strict format: `<id>-<kebab-case-title>-<role>.md`
  (example: `001-exchange-adapter-generalist.md`)
- must support closing a gap identified by the review
