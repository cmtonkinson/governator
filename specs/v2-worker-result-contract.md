<!--
File: specs/v2-worker-result-contract.md
Purpose: Define how workers signal completion and how results are interpreted.
-->
# Governator v2 Worker Result Contract

## Purpose
Define the required signals that workers emit to indicate completion or
blockage for a task stage.

## Success signals
A worker run is considered successful only when both of the following are
present in the task worktree:
- A commit on the task branch.
- A stage marker file for the stage being completed.

If either signal is missing, the task is treated as blocked.

## Required commit behavior
- Workers must create a commit to indicate success.
- Commit content represents the work performed for the stage.
- Commit messages follow conventions for readability only; Governator does not
  parse or enforce message formats.

### Commit message conventions (examples)
- `governator: TASK-123 worked - implement index writer`
- `governator: TASK-123 tested - add scheduler coverage`
- `governator: TASK-123 reviewed - resolve review notes`
- `governator: TASK-123 resolved - fix merge conflict`

## Stage marker files
Workers must emit a stage marker file under the task worktree at:
`<worktree>/_governator/_local_state/{worked,tested,reviewed,resolved,blocked}.md`

Each marker file:
- Contains a short summary of what was done or why the task is blocked.
- Is treated as a presence-only signal (no schema, validation, or parsing).

### Stage to marker mapping
- `worked` -> `worked.md`
- `tested` -> `tested.md`
- `reviewed` -> `reviewed.md`
- `resolved` -> `resolved.md`
- `blocked` -> `blocked.md`

## Blocking conditions
- If the commit is missing, the task is blocked.
- If the expected stage marker file is missing, the task is blocked.

## Examples
Worked marker file:
```md
Implemented index writer helpers and updated documentation.
```

Blocked marker file:
```md
Cannot proceed: missing upstream schema for task index JSON.
```
