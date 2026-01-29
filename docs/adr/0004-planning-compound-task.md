# ADR 0004: Model planning phases as a compound task

- Status: Superseded by ADR 0011 (2026-01-28)
- Date: 2026-01-27

## Context

Governator v2 advances through planning phases 1-4 (architecture baseline, gap analysis, project planning, and task
planning) before entering task execution. The current orchestration treats these phases as special cases with bespoke
rules for dispatching agents, validating artifacts, finalizing worktrees, and merging results back into the repository
root.

Recent changes made the Go runtime responsible for deterministic completion handling by relying on `exit.json` and
performing Git operations directly. However, planning phases still require dedicated logic for:

- mapping phases to prompts and roles,
- computing workstream and branch names,
- performing merge behavior, and
- sequencing phase transitions and gates.

This special handling increases the number of places that must be updated when planning behavior changes, and it makes
it harder to evolve planning into a configurable operator-defined process.

## Decision

We will model planning phases 1-4 as a single compound task composed of ordered steps. Each step declares the minimum
configuration required to run deterministically:

- the phase it represents,
- the prompt path,
- the role assignment, and
- explicit success actions (for now: merge to base and advance the phase machine).

The phase runner will interpret this planning task definition instead of relying on a phase-spec map and scattered
assumptions. Step completion still requires an `exit.json` with exit code 0, and Git finalization continues to be owned
by the Go runtime.

## Consequences

### Positive

- Planning orchestration becomes more cohesive and easier to reason about as a state machine with declared transitions.
- Future changes can evolve the planning task schema rather than proliferating new special-case branching logic.
- Merge and transition behavior are encoded as explicit step actions rather than implicit assumptions.

### Negative

- The phase tracker still persists per-phase metadata, so the compound task is an interpretation layer rather than a
  full state model replacement.
- The initial planning task definition is still hardcoded; additional work is required to make it user-configurable.

## Notes

This ADR focuses on the orchestration model only. Prompt content and worker contracts remain unchanged.
