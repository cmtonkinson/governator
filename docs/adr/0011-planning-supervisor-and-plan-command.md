# ADR 0011: Planning Supervisor and Plan Command

## Status

Proposed

## Context

Planning currently relies on repeated `governator run` invocations to detect
worker completion and advance to the next step. This results in a polling-style
operator experience, makes automatic prompt validations awkward to chain, and
blurs the boundary between planning and execution workflows. The desired UX is
explicit: `gov init` -> `gov plan` -> `gov execute`, with planning remaining
strictly serial and fully validated before execution begins.

We need a durable orchestration mechanism for planning that can dispatch a step
worker, chain any prompt validations automatically, advance the planning cursor,
and continue without requiring additional CLI invocations.

## Decision

- Introduce a dedicated `governator plan` command that starts a detached
  planning supervisor process.
- `governator plan` refuses to start if a planning supervisor is already
  running; operators must `governator stop` or `governator reset` first.
- The planning supervisor runs the entire planning workstream serially, step by
  step, and automatically chains prompt validations after each step worker
  completes successfully.
- Supervisor state (pid, metadata, and logs) lives under
  `_governator/_local-state/planning_supervisor/`.
- Planning uses a single planning task in the task index with a cursor field
  that records the current step id.
- Validation failures stop the supervisor and require operator intervention.
- Add `governator stop`, `governator restart`, and `governator reset` for
  supervisor control. By default, `stop` terminates the supervisor only; a
  `--worker` flag stops the active worker as well.
- `governator run` is deprecated and should emit a warning directing operators
  to `governator plan` / `governator execute`.

## Consequences

Positive:
- Planning becomes autonomous and deterministic within a single supervisor
  lifecycle, eliminating repeated CLI polling.
- Prompt validations can be chained immediately after a step completes.
- Operator intent is clearer with distinct `plan` and `execute` entrypoints.

Negative:
- Long-running supervisor processes require state persistence, restart logic,
  and explicit operator controls.
- Status reporting must surface supervisor state, including multiple supervisors
  in error scenarios.

Tradeoffs accepted:
- Planning orchestration shifts from a short-lived CLI command to a background
  process with explicit lifecycle management.

## Notes

- Date: 2026-01-28
- Related ADRs: 0004, 0006, 0009, 0010 (superseded)
- Related Tasks: Planning pipeline refactor and supervisor rollout
