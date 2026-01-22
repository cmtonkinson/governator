# ADR 0001: Async Worker Dispatch and In-Flight Tracking

Date: 2026-01-22

## Status

Accepted

## Context

Governator v2 previously executed worker commands synchronously and updated task state immediately after execution. The
system now must dispatch all workers asynchronously so `governator run` returns quickly while workers continue in the
background. We still need to detect completion, enforce timeouts, and avoid duplicate dispatches without introducing a
separate "collect-only" command.

## Decision

- Dispatch workers asynchronously using `nohup` and a wrapper script that records exit status to a stage-scoped file in
  the worktree (`_governator/_local_state/worker/exit-<stage>-<task>.json`).
- Persist an in-flight registry (`_governator/_local_state/in-flight.json`) that records task IDs and dispatch start
  times. This registry is used to:
  - Avoid re-dispatching tasks already in flight.
  - Compute remaining concurrency capacity by subtracting in-flight counts.
  - Determine timeouts when workers do not produce exit status files.
- During each `run`, perform lightweight collection for in-flight tasks:
  - If an exit status file is present, treat the worker as finished and evaluate marker/commit presence for success.
  - If no exit status file exists and the in-flight start time exceeds the worker timeout, block the task.
- Route review failures back to `open` so they re-enter the work stage.
- Merge conflicts continue to transition tasks to `conflict`; non-conflict merge errors are blocked with a TODO to revisit.

## Consequences

- `run` can both dispatch and collect in the same invocation without waiting on workers.
- The in-flight registry and exit status files are required for deterministic async completion checks and timeouts.
- Review failures now re-enter work without incrementing work attempts; retry semantics are intentionally left for a
  future update.
