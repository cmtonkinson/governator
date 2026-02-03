# ADR 0012: Execution Backlog Triage

## Status

Accepted

## Context

Execution tasks can arrive in backlog without dependency ordering. The execution
supervisor currently dispatches only triaged tasks, which leaves backlog items
stuck unless an operator manually sets dependencies or task state. We need an
automatic step that derives a DAG for backlog tasks, applies it to the task
index, and moves those tasks into the triaged state before execution begins.

## Decision

- Add an execution supervisor triage loop that runs whenever backlog tasks
  exist.
- When backlog tasks exist, the supervisor first drains in-flight work without
  dispatching new tasks.
- After the pool drains, dispatch a triage agent (using
  `internal/templates/planning/dag-order-tasks.md`) to produce a JSON mapping at
  `_governator/_local-state/dag.json`.
- Apply the mapping to overwrite dependencies for backlog/triaged tasks and
  move those tasks to the triaged state.
- If triage fails to emit usable data, retry once; after the second failure,
  mark the execution supervisor as failed and exit without modifying the index.
- The execution supervisor may cycle between triage and execution as backlog
  items appear.

## Consequences

Positive:
- Backlog tasks are automatically ordered and become eligible for execution.
- Execution runs are resilient to ad-hoc task files that lack dependencies.
- Operators no longer need to manually edit the index for ordering.

Negative:
- A triage phase adds a new asynchronous worker lifecycle to execution.
- Execution startup latency increases when backlog tasks exist.

## Notes

- Date: 2026-02-03
- Related ADRs: 0011
