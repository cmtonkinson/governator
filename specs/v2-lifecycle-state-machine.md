<!--
File: specs/v2-lifecycle-state-machine.md
Purpose: Define the lifecycle states and allowed transitions for task execution.
-->
# Governator v2 Lifecycle State Machine

Concise specification for task lifecycle states and the allowed transitions
between them. Transitions are index-driven and invalid transitions must return
clear errors.

## States
- `open`: task has not been started.
- `worked`: worker completed implementation and awaits testing.
- `tested`: tests completed and awaits review/merge.
- `done`: task is complete; terminal state.
- `blocked`: task cannot proceed without intervention; terminal for automation.
- `conflict`: review/merge step encountered a conflict.
- `resolved`: conflict resolution completed and awaits merge retry.

## Allowed transitions
| From | Agent/action | To | Notes |
| --- | --- | --- | --- |
| `open` | worker success | `worked` | Worker produced work artifacts. |
| `open` | worker failure (retries exhausted) | `blocked` | Operator intervention required. |
| `worked` | test success | `tested` | Tests passed and artifacts recorded. |
| `worked` | test failure (retries exhausted) | `blocked` | Operator intervention required. |
| `tested` | review/merge success | `done` | Merge to main completed. |
| `tested` | review/merge conflict | `conflict` | Rebase or merge conflict detected. |
| `tested` | review/merge failure (retries exhausted) | `blocked` | Operator intervention required. |
| `conflict` | conflict-resolution success | `resolved` | `resolved.md` marker recorded. |
| `conflict` | conflict-resolution failure (retries exhausted) | `blocked` | Operator intervention required. |
| `resolved` | review/merge success | `done` | Merge to main completed. |
| `resolved` | review/merge conflict | `conflict` | Retry still conflicts. |
| `blocked` | operator reset | `open` | Manual override to restart automation. |

## Disallowed transitions
All transitions not listed above are invalid. For example, `done` cannot
transition back to `worked`.
