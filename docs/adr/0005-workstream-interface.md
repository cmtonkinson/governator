# ADR 0005: Introduce a unified planning workstream interface

- Status: Accepted
- Date: 2026-01-27

## Context

Planning phases (1-4) have moved toward deterministic completion by relying on `exit.json` and Go-owned Git operations.
ADR 0004 reframed these phases as a compound planning task, but the phase runner still baked in transition rules such as
"gate the current phase before dispatch" and "gate the next phase before advancing."

We want to support different workstream types (for example, planning versus execution) without proliferating special-case
orchestration logic for each one. In particular, we need a place to declare state transition gates and success actions in
a way the engine can interpret mechanically.

## Decision

We will introduce a small workstream interface in the run package:

- `phaseWorkstream` provides `stepForPhase(phase.Phase) (workstreamStep, bool)`.
- `workstreamStep` declares prompt, role, success actions, and gate targets for both dispatch and advance transitions.

The phase runner now consumes this interface and evaluates gates via the configured targets rather than hardcoding gate
selection in multiple places. The planning workstream definition supplies the current behavior by setting:

- dispatch gates to the current phase, and
- advance gates to the next phase.

## Consequences

### Positive

- Transition rules are declared alongside the step definition rather than scattered across orchestration code.
- We can evolve toward operator-defined workstreams by extending the step schema instead of rewriting the runner.
- The runner remains deterministic: it still requires `exit.json` and performs Git finalization and merges in Go.

### Negative

- The workstream interface currently covers planning phases only; execution workstreams remain separate.
- Gate targets are phase-based, which keeps the model simple but may need to expand for non-phase transitions later.

## Notes

This ADR intentionally introduces a minimal interface to reduce risk while unlocking future workstream types.

