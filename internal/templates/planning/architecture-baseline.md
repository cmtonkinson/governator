# Architecture Baseline

## Objective
Establish the architectural foundation for this project by producing the **Power Six** architecture artifacts required
to clearly and confidently generate a project plan leading to the successful implementation of the intent described in
`GOVERNATOR.md`.

Your task is to empower the rest of the project team to complete their tasks both successfully and independently by
reducing uncertainty with respect to the design of the solution. Every project team member will need to make decisions
about how to complete their tasks an may reference your work; reducing uncertainty helps ensure alignment across tasks
and roles.

## Artifacts

### Power Six
These are the **"Power Six"** artifacts: a minimal and forgiving collection of architectural documentation which
together yields a comprehensive understanding of a technical solution:
1. **User Personas** (recommended)
2. **Architecturally Significant Requirements (ASRs)** (required)
3. **Wardley Map** (recommended)
4. **Architecture Overview (arc42)** (required)
5. **C4 Diagrams** (recommended)
6. **Architectural Decision Records (ADRs)** (required)

The artifacts are listed not in order of importance, but rather the recommended _chronological order_ in which they
should be generated.

_Note: You SHOULD consider creating the optional artifacts, but MAY choose to omit/skip if the scope or complexity of
the project do not justify their production._

## Artifact Rules
- All artifacts MUST use the templates provided in `_governator/templates/`.
- All artifacts MUST be written in markdown.
- All artifacts MUST be stored in the `_governator/docs` directory.
- Sections may not be removed.
- Empty sections must be explicitly marked as intentionally omitted, with an explanation as to why.
- No implementation detail is allowed unless architecturally significant.

## ADR Emission
If any decision:
- materially constrains future implementation, or
- eliminates viable alternatives, or
- commits to a technology, pattern, or platform,

then an ADR MUST be created using the ADR template.

## Existing Projects
An 'existing' project is defined as one that already has source code. For an existing project:
- Your task is to discover and document the current code in order to populate the Power Six.
- If `GOVERNATOR.md` specifies architecturally relevant details, note those details where appropriate in the Power Six.
- Note that the code may not be complete, correct, or functional.
- Note that the code may not be congruent with `GOVERNATOR.md`. In this case, defer to `GOVERNATOR.md` and assume the
  implementation will need to change.
- If you are unable to reliably determine the state of the existing system:
    - Note this in the Power Six where appropriate.
    - Do not skip the task, or the artifacts. Instead, design those details in the Power Six as if this project were
      new.
    - This may mean prescribing or establishing new standards, conventions, patterns, or best practices in the Power Six
      which do not appear in the existing system, and which may require refactoring parts of the existing system to
      bring them into compliance.

## Definition of Done
This task is complete when:
- All required artifacts:
  - exist
  - are complete
  - are correct
  - do not contain errors
  - are free of omissions
  - do not conflict with one another
  - form a cohesive set of documentation to guide project planning and implementation
- Optional artifacts exist (pursuant to the rules above) or else are explicitly skipped, with justification.
- Any and all significant decisions are memorialized as ADRs.
- No project planning artifacts or implementation tasks have been created.
