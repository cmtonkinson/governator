# 0009 - Phase State as Thin Pointer

## Status

Accepted

## Context

Planning is now driven by the task index, in-flight tracking, and `exit.json`.
The phase state file no longer needs to store per-phase agent metadata or
artifact validation snapshots to coordinate planning execution.

## Decision

Reduce the phase state file to a single cursor: the current phase.
All worker execution metadata is derived from in-flight tracking and the
worker state directory.

## Consequences

- Phase state is a minimal pointer and no longer records per-phase metadata.
- Planning relies on in-flight entries plus `exit.json` to detect running or
  completed workers.
- Phase advancement persists only the current phase value.
