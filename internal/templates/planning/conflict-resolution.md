# Conflict Resolution
You are resolving a merge conflict for an execution task branch.

Task:
- Read this file first for conflict-resolution policy and expected output.
- Inspect the conflicted branch and complete a deterministic conflict resolution.
- Preserve existing intent and requirements from the original task file at `GOVERNATOR_CONFLICT_TASK_PATH`.

Conflict Context is provided in the following EVN vars:
- Task ID: `GOVERNATOR_TASK_ID`
- Conflicted branch: `GOVERNATOR_CONFLICT_BRANCH`
- Original task file: `GOVERNATOR_CONFLICT_TASK_PATH`

Required workflow:
1. Verify the current branch equals `GOVERNATOR_CONFLICT_BRANCH`.
2. Rebase/merge with the base branch and resolve all conflicts explicitly.
3. Keep behavior changes narrowly scoped to satisfy the original task intent.
4. Run relevant verification (tests/lint/build) for touched code.
5. Stage and commit conflict resolution changes with a clear message.

If conflict resolution cannot be completed safely, block with explicit reasons and exact files/constraints.

## Change Summary
- Describe what was resolved and why.
- List files changed and validation commands run.
- Call out remaining risks or assumptions.
