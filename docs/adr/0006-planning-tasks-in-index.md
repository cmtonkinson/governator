# ADR 0006: Seed planning tasks in the task index during init

- Status: Accepted
- Date: 2026-01-27

## Context

We want the task index to act as the control plane from the first `run`. That
requires the index to exist before execution work is available. Planning steps
also change planning artifacts, so the stored planning digests must be updated
as planning completes or planning drift will be reported incorrectly when
execution begins.

## Decision

We will:

- add `Task.Kind` with explicit values `planning` and `execution`,
- seed planning steps as `planning` tasks in `_governator/task-index.json`
  during `init`, and
- refresh index digests and mark the completed planning step inside the
  planning worktree before merging it into the base branch.

Execution scheduling, branch creation, and status reporting will operate only
on `execution` tasks.

## Consequences

### Positive

- The index exists immediately after `init`, so status and PID visibility have
  a stable foundation.
- Planning digests stay aligned with the merged planning artifacts.
- Planning and execution can share index-backed orchestration without
  cross-contamination.

### Tradeoffs

- Planning currently updates the index through the phase runner rather than a
  shared workstream runner.
- Planning tasks are present in the index even though their lifecycle is still
  driven by phase state.

