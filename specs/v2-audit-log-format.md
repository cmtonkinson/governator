<!--
File: specs/v2-audit-log-format.md
Purpose: Define the append-only audit log schema and required events.
-->
# Governator v2 Audit Log Format

The audit log is a single append-only text file. Each entry is one line in a
logfmt-style format to keep it human-readable and diff-friendly.

## Required fields
Every entry includes the following fields:
- `ts`: RFC3339 timestamp in UTC.
- `task_id`: Task identifier from the index.
- `role`: Role assigned to the task.
- `event`: Event name.

## Optional fields
Additional fields are event-specific and may be included to improve diagnostics.
They should remain short, stable, and single-line.

## Event catalog
### Worktree events
- `worktree.create`: worktree path created for a task.
- `worktree.delete`: worktree path deleted for a task.

### Branch events
- `branch.create`: branch created for a task.
- `branch.delete`: branch deleted for a task.

### Task lifecycle events
- `task.transition`: task state transitions.

### Agent events
- `agent.invoke`: agent started (planner, worker, tester, reviewer).
- `agent.outcome`: agent finished with a result.

## Examples
Worktree create:
`ts=2025-01-14T19:02:11Z task_id=T-014 role=worker event=worktree.create path=_governator/_local_state/worktrees/T-014 branch=gov/T-014`

Worktree delete:
`ts=2025-01-14T19:12:54Z task_id=T-014 role=worker event=worktree.delete path=_governator/_local_state/worktrees/T-014 branch=gov/T-014`

Branch create:
`ts=2025-01-14T19:02:10Z task_id=T-014 role=worker event=branch.create branch=gov/T-014 base=main`

Branch delete:
`ts=2025-01-14T19:13:01Z task_id=T-014 role=worker event=branch.delete branch=gov/T-014`

Task transition:
`ts=2025-01-14T20:07:03Z task_id=T-014 role=worker event=task.transition from=open to=worked`

Agent invoke:
`ts=2025-01-14T20:08:11Z task_id=T-014 role=tester event=agent.invoke agent=test-runner attempt=1`

Agent outcome:
`ts=2025-01-14T20:09:45Z task_id=T-014 role=tester event=agent.outcome agent=test-runner status=failed exit_code=1`
