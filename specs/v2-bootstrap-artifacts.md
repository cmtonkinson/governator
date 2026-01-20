<!--
File: specs/v2-bootstrap-artifacts.md
Purpose: Define required vs optional Power Six artifacts for bootstrap.
-->
# Governator v2 Bootstrap Artifacts

## Required Power Six artifacts
Bootstrap must produce the following artifacts, matching v1 requirements:
1. Architecturally Significant Requirements (ASRs)
2. Architecture Overview (arc42 style)
3. Architectural Decision Records (ADRs)

## Optional Power Six artifacts
Bootstrap may produce these artifacts when scope or complexity justify them,
matching v1 recommendations:
1. User Personas
2. Wardley Map
3. C4 Diagrams

## Location and custody
- Artifacts live under `_governator/docs/` and use the bootstrap templates.
- Workers may suggest edits in their outputs but must not directly edit or
  rewrite Power Six artifacts.

## Rationale
The required set provides the minimum durable architecture context needed for
planning and execution while remaining small and auditable. Optional artifacts
remain available for projects that benefit from deeper discovery or modeling.

## v1 alignment check
No discrepancies found between the v1 Power Six rules and the lists above.
