---
milestone: 
epic: 
task: 
---

# Task: <Short Descriptive Title>

## Objective
Clearly and unambiguously describe **what outcome must exist** when this task is
complete.
- Focus on *end state*, not implementation
- One paragraph max
- If this cannot be stated precisely, the task should block

## Context
Provide the minimum background needed to execute safely. Include:
- relevant files or directories
- existing behavior being modified
- constraints imposed by prior decisions

Do **not** restate global project goals.

## Requirements
List concrete, testable requirements.
- Use bullet points
- Each item should be objectively verifiable
- Avoid subjective language (“clean”, “nice”, “elegant”)

Example:
- [ ] Configuration is read from `.governator/config.json`
- [ ] Script exits non-zero on failure
- [ ] No new runtime dependencies introduced

## Non-Goals
Explicitly state what **must not** be done. This is critical for safety.

Examples:
- Do not touch authorization logic
- Do not make assumptions about the contents of `import.dat`
- Do not trigger the `account.reflow` or `item.touch` events

## Constraints
Hard boundaries the worker must respect.

Examples:
- Module must remain POSIX-compatible
- New functionality must not require network access at runtime
- Changes cannot break or modify existing parsing tests

If violating a constraint seems necessary, the task must block.

## Acceptance Criteria
Define what “done” means for review. Reviewer should be able to answer
**yes/no** to each item.

Example:
- [ ] All requirements satisfied
- [ ] Tests pass (or explanation provided if none exist)
- [ ] Changes are limited to stated scope
- [ ] No violations of worker contract

=============================================================================
=============================================================================

## Notes
