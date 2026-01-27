# ADR 0007: Introduce a workstream runner for planning orchestration

- Status: Accepted
- Date: 2026-01-27

## Context

Planning and execution are converging on a shared control loop: gate, dispatch,
collect via `exit.json`, and deterministic post-processing. Planning already
uses the in-flight store and Go-owned git operations, but its orchestration
logic is still embedded in the phase runner.

## Decision

Introduce a `workstreamRunner` that drives the common control loop via a
controller adapter. Planning now uses this runner through a
`planningController` that binds phase state, in-flight tracking, and gating to
the shared runner.

## Consequences

### Positive

- A single control loop orchestrates planning work, making the path deterministic.
- Planning state handling is isolated behind a controller, enabling future
  reuse for execution workstreams.

### Tradeoffs

- Planning still depends on phase state for step selection until execution
  adopts the same runner.
