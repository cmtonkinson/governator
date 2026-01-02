# Task 000: Architecture Bootstrap (Power Six)

## Objective
Establish the architectural foundation for this project by producing the minimum
viable set of the **Power Six** architecture artifacts required to safely and
productively generate implementation tasks leading to the successful
implementation of this project.

Your task is to empower the rest of the project team to complete their tasks
both successfully and independently by reducing uncertainty with respect to the
design of the solution. Every project team member will need to make decisions
about how to complete their tasks; reducing uncertainty helps ensure alignment
across across roles and tasks.

## Required Artifacts

### Power Six
These are the **"Power Six"** artifacts: a minimal and forgiving collection of
architectural documentation which together yields a comprehensive understanding
of a technical solution.

These are the **Power Six** artifacts:
1. **User Personas** (recommended)
2. **Architecturally Significant Requirements (ASRs)** (required)
3. **Wardley Map** (recommended)
4. **Architecture Overview (arc42)** (required)
5. **C4 Diagrams** (recommended)
6. **Architectural Decision Records (ADRs)** (required)

The artifacts are listed not in order of importance, but rather the recommended
_chronological order_ in which they should be formalized.

_Note: You SHOULD consider creating the optional artifacts, but MAY choose to
omit/skip if the scope or complexity of the project do not justify their
production._

## Artifact Rules
- All artifacts MUST use the templates provided in `_governator/templates/`
- All artifacts MUST be written in markdown
- All artifacts MUST be stored in the `_governator/docs` directory
- Sections may not be removed
- Empty sections must be explicitly marked as intentionally omitted, with an
  explanation as to why.
- No implementation detail is allowed unless architecturally significant

## ADR Emission
If any decision:
- materially constrains future implementation, or
- eliminates viable alternatives, or
- commits to a technology, pattern, or platform

An ADR MUST be created using the ADR template.

## Definition of Done
This task is complete when:
- Required artifacts exist and are filled out
- Optional artifacts are either present or explicitly skipped with justification
- All significant decisions are momorialized as ADRs
- No feature or implementation tasks have been created
