# 0008 - Execution Workstream Runner Integration

## Status

Accepted

## Context

Planning and execution are moving toward a single deterministic control plane.
Planning already runs through the shared workstream runner, but execution stages
were still orchestrated via bespoke sequencing in the run command.

Execution stages already encapsulate collection and dispatch logic within their
stage functions. Rewriting those functions to split collection from dispatch
would increase churn with limited value.

## Decision

Route execution through the same workstream runner by adding an execution
controller that sequences stages and delegates each stage to the existing
stage functions. The runner handles step ordering; the execution controller
keeps collection inside the dispatch step for each stage.

## Consequences

- Planning and execution share the same runner loop and control plane.
- Execution stages remain authoritative for their own collection/dispatch rules.
- Dispatch can continue across stages in a single run without special casing.
