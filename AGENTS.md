Instructions for working in this repo:
- Ask clarifying questions when needed.
- Never create JSONB fields, or string/text fields meant to hold JSON data, in
  an RDBMS without:
  - A very good rationale.
  - Clearly and explicitly explaining why you believe it is justfied.
  - Explicit user approval.
- `'work-*/` directories:
  - These are transient agent/task management workspaces.
  - These are not permanent project directories.
  - No work output should be stored in these directories.
  - Do not modify the contents of these directories (with the exception of
    `index.md`) unless explicitly instructed to do so or without requesting
    permission.

When modifying code:
- Write code that is concise, readable, maintainable, and idiomatic.
- Every file/class/unit should have a complete docblock/docstring.
- Prefer smaller composable units over larger monolithic ones.
- When designing or implementing functionality, consider how 3rd party
  dependencies (open source packages via gems, npm, pip, mods, etc) may reduce
  effort and save time.
- Code defensively; assume failure; assume invalid input.
- Before adding new units, search existing code for logic that may be
  reused/refactored.
- When modifying existing untis, write/modify tests as appropriate.
- When creating new units, design them to be tested, and write appropriate
  tests.
- Run all tests after making changes to validate behavior (e.g. using
  `./test.sh`).
- When tests fail, don't just modify the tests to make them pass. First,
  suspect that the SUT is flawed.

