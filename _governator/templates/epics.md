# Project Epics
> This document defines the project’s **epics**: coherent capabilities that
> collectively realize each milestone.
>
> Epics exist to preserve **intent, scope boundaries, and design decisions**
> while serving as the source of truth for story-level task generation.

## How to Read This Document
- Epics are grouped by **milestone**.
- Every epic maps to **exactly one milestone**.
- Tasks must map to **exactly one epic**.
- Epics should remain stable even if tasks change.

## Milestone M0 — _[Milestone Name]_

### Epic E0.1 — _[Epic Name]_
**Goal**  
A concise statement of the capability this epic delivers.

**Why This Epic Exists**  
What user or system value does this epic provide?
What problem does it solve within the milestone?

**In Scope**
- Capabilities, behaviors, or guarantees included in this epic
- Phrase in terms of outcomes, not tasks

**Out of Scope**
- Explicit exclusions to prevent scope bleed
- Anything intentionally deferred to other epics or milestones

**Key Decisions**
- Architectural or design choices made here
- Trade-offs accepted (with brief rationale)

**Interfaces / Contracts**
- APIs, events, schemas, CLIs, file formats, or user-visible contracts
- Stable boundaries this epic defines or consumes

**Risks & Mitigations**
- Known risks (technical, product, operational)
- How they are reduced or accepted in this epic

**Definition of Done (Epic-level)**
This epic is complete when:
- Capability is present and usable
- Acceptance conditions are met across all related stories
- Integration points are validated

### Epic E0.2 — _[Epic Name]_

_(Repeat the same structure)_

## Milestone M1 — _[Milestone Name]_

_(Repeat epic sections for this milestone)_

## Epic Dependency Notes
Optional.
Call out:
- Ordering constraints between epics
- Shared infrastructure or cross-cutting concerns

## Epic Granularity Guidelines
Epics should:
- Represent a **coherent capability**, not a grab-bag of tasks
- Be small enough to reason about in isolation
- Be large enough that splitting them further would reduce clarity

If an epic:
- Spans multiple milestones → split it
- Contains unrelated capabilities → split it
- Collapses to a single trivial story → consider inlining or merging
