# 0010 - Planning Progress Derived From Task Index

## Status

Accepted

## Context

Planning execution now relies on the task index and in-flight tracking.
The phase state file is no longer used during runtime, so determining the
current planning step should not depend on a separate persisted cursor.

## Decision

Derive the current planning step directly from the task index by selecting
the first planning task not marked merged. Planning completion is declared
when all planning tasks are merged.

## Consequences

- Planning progress is fully represented by the task index plus in-flight data.
- The runtime no longer reads or writes a phase state cursor.
- Planning completion logic becomes deterministic and replayable from the index.
