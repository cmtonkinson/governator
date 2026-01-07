# Task 000: Blocked Task Analysis (Planner)

## Objective
Review blocked tasks for additional context, clarification, or disambiguation.
If a task can be unblocked, requeue it with a clear note. If it cannot, leave
it blocked and record your analysis.

## Input
- Blocked tasks in `_governator/task-blocked/`
- Any relevant artifacts in `_governator/docs/`

## Output
For each blocked task (skip tasks that already include an "Unblock Note" or
"Unblock Analysis" section):
1. If you can resolve the block:
   - Run `./_governator/governator.sh unblock <task-prefix> "<note>"`.
   - The note must explain what changed and how to proceed.

2. If you cannot resolve the block:
   - Append a `## Unblock Analysis` section to the blocked task explaining why
     it should remain blocked and what information is missing.
   - Do not unblock the task.

Only one unblock attempt is allowed per task. If a task is re-blocked after an
unblock, leave it blocked.
