<!--
File: specs/v2-planning-output-model.md
Purpose: Describe how planning outputs map to tasks, ordering, and overlap flags.
-->
# Governator v2 Planning Output Model

## Purpose
Define how planning outputs map to task index entries, ordering, and overlap
flags so the planner and planner output parser can implement a deterministic,
auditable flow.

## Planning outputs
Planning emits flat JSON artifacts under `_governator/plan/`:
- `architecture-baseline.json`
- `gap-analysis.json` (optional)
- `roadmap.json`
- `tasks.json`

The roadmap and tasks outputs drive the task index and task files.

## Mapping roadmap items to artifacts
Roadmap decomposition (`roadmap.json`) is the canonical hierarchy for planning.
It may include any combination of `milestone`, `epic`, `feature`, or
`task_group` items. These items are written as flat plan files only; they are
not task index entries.

Tasks (`tasks.json`) are the only entries that become:
- Task files in `_governator/tasks/`
- Task index entries in `_governator/task-index.json`

Each task must reference the roadmap intent via its `id` and must be traceable
to a roadmap item via shared IDs or ancestry. The planner chooses the depth and
width of decomposition; trivial projects may emit a single task with no roadmap
parents.

## Ordering semantics
Ordering is deterministic and dependency-driven:
1. Dependencies define required sequencing. A task is eligible only when all
   dependencies are complete.
2. `order` is a deterministic tiebreaker among eligible tasks.
3. If two eligible tasks have the same `order`, the scheduler must apply a
   stable secondary sort (for example, lexical `id`) to preserve determinism.

Planner guidance:
- `order` should be increasing within a roadmap subtree to reflect intended
  sequencing.
- Dependencies should be explicit; do not rely on `order` alone to imply hard
  prerequisites.

## Overlap semantics
`overlap` is a set of labels that indicate likely shared code surfaces:
- Tasks sharing any overlap label are treated as overlapping for parallelism.
- Overlap labels are descriptive tokens (for example, `planner`, `index`,
  `cli`), not booleans.

Planner guidance:
- Roadmap items may include `overlap` labels to convey shared surfaces for the
  entire subtree.
- Task overlap is the union of the task's own labels plus any ancestor roadmap
  labels when the planner emits tasks.

## Examples
Trivial project (single task):
- Roadmap: one `task_group` with `order: 10`, no overlap.
- Task: one task with `dependencies: []`, `order: 10`, `overlap: []`.

Complex project (milestone -> epic -> task):
- Roadmap milestone `overlap: ["planner"]` with ordered epics.
- Tasks inherit `planner` overlap and add `index` where needed.
- Dependencies reference prior tasks for critical sequencing; `order` provides
  stable within-epic ordering when dependencies are equivalent.
