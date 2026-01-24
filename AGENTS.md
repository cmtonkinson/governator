Instructions for working in this repo:
- Ask clarifying questions when needed to resolve ambiguity or conflict.
- `'work-*/` directories:
  - Read-only inspection is allowed; writes are restricted.
  - These are transient agent/task management workspaces, not permanent project directories.
  - No work output should be stored in these directories.
  - Do not modify the contents (with the exception of `index.md`) unless instructed.
- Non-obvious architecture/design tradeoffs should be documented in an ADR.

When modifying code:
- Adhere to SOLID principles when they improve clarity or testability.
- Bias toward cohesion and locality.
- Prefer domain-aligned code and prioritize clarity of intent.
- Optimize for clarity first; refactor to DRY once repetition is stable.
- Avoid clever tricks and elegant patterns when it sacrifices legibility or testability.
- Respect existing patterns and conventions, fall back to idiomatic standards.
- Every file/class/unit should have a complete and meaningful docblock/docstring.
- Prefer smaller composable units over larger monolithic ones.
- When designing or implementing functionality, consider how 3rd party dependencies (open source packages via gems, npm,
  pip, mods, etc) may reduce effort and save time.
- Prefer explicit errors and observable failure modes over silent recovery.
- Before adding new units, search existing code for logic that may be reused/refactored.
- When modifying existing units, write/modify tests as appropriate.
- When creating new units, design them to be tested, and write appropriate tests.
- Run relevant linting, type checking, tests, etc. after making changes to validate behavior.
- When tests fail, do not blindly modify the tests to make them pass. First, suspect that the SUT is flawed and assess
  the root cause of the failure.
- Remember: The goal is not that the tests pass, the goal is correct code which implements the provided scope, which
  _also_ has passing tests that exercise it to ensure resiliency in the face of future changes.

Read v2.md. This is a description of the new design/direction/architecture of the project.
