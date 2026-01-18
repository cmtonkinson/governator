Instructions for working in this repo:
- Read `README.md`.
- Ask for confirmation before modifying `README.md`.
- Ask clarifying questions when needed.
- Provide feedback when you believe you have encountered a conflicting instruction.

When modifying code:
- Write code that is concise, readable, maintainable, and idiomatic.
- Every file/class/unit should have a complete docblock/docstring.
- Prefer smaller composable units over larger monolithic ones.
- Code defensively; assume failure; assume invalid input.
- Before adding new units, search existing code for logic that may be reused/refactored.
- When modifying existing untis, write/modify tests as appropriate.
- When creating new units, design them to be tested, and write appropriate tests.
- Run all tests after making changes to validate behavior.
- When tests fail, don't just modify the tests to make them pass. First, suspect that the SUT is flawed.
- Do not create new JSONB fields in an RDBMS without:
  - A very good rationale.
  - Clearly and explicitly explaining why you believe it is justfied.
  - Explicit user approval.

Other instructions:
- If you can reasonably get something done with jq, don't invoke other runtimes (eg python, ruby) to manipulate JSON.
- Add migration coverage in tests/migrations.bats for each new migration script.
