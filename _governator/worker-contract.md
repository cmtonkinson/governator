# Worker Contract
This is your primary binding contract. Failure to comply with this contract
risks invalidating your work and may result in rejection. This contract applies
to you at all times unlexx explicitly overridden by later instruction.

## 1. Execution Model
You are executing **one assigned task** under **one defined role**. You must
operate strictly within the authority and constraints of your assigned role and
task.

## 2. Required Inputs (Read in Order)
Before taking any action, you must read in full:
- GOVERNATOR.md. You must never modify this file.

## 3. Scope Rules
You must:
- Perform only the work explicitly requested in the assigned task.
- Modify only files necessary to complete the task.
- Prefer minimal, localized changes.

You must not:
- Expand or reinterpret the task.
- Combine this task with other work.
- Perform refactors, cleanup, or improvements unless explicitly instructed.

If the task is underspecified or ambiguous, do not guess.

## 4. Version Control Rules
You are operating in an isolated git branch created for this task. You may
create commits as needed. You must create clear and descriptive commit messages.

Before exiting, you must, in order:
- Commit any pending changes.
- Push this branch exactly once, as your final action.

You MAY `fetch` from `origin` and `rebase` onto the default branch configured
in `.governator/config.json` before you push.

Do NOT make or commit any changes to or within the following locations:
- _governator/custom-prompts/
- _governator/governator.sh
- _governator/roles/
- _governator/templates/
- _governator/worker-contract.md

You must not:
- Modify any other branch or tag.
- Push partial or exploratory work.

The eight rules of writing git commit messages:
1. You MUST prefix your git commit message with the string "[governator] "
2. Separate subject from body with a blank line
3. Limit the subject line to 50 characters
4. Capitalize the subject line
5. Do not end the subject line with a period
6. Use the imperative mood in the subject line
7. Wrap the body at 72 characters
8. Use the body to explain what and why vs. how

## 5. Blocking Conditions
You must block the task if you cannot proceed safely and correctly. Blocking
conditions include (but are not limited to):
- Missing or ambiguous requirements
- Conflicting instructions
- Required decisions outside your authority
- Unclear file ownership or modification boundaries

### How to Block
1. Move the assigned task file to `_governator/task-blocked/`.
2. Append a section titled `## Blocking Reason` to the task file.
3. Clearly describe:
  - What is unclear or missing
  - What decision or information is required to proceed

Do not make speculative changes when blocked.

## 6. Completing the Task
When you believe the task is complete:
1. Append a section titled `## Change Summary` to the task file.
- Describe what was changed.
- Note any assumptions made.
- Mention potential follow-up concerns without creating tasks for them.
2. Move the task file to `_governator/task-worked/`.
3. Ensure:
- All changes are committed
- The branch is pushed
- No uncommitted changes remain
4. Then exit.

## 7. Proposing Additional Work (Optional)
If you identify clearly separable follow-up work:
- Do not expand the current task.
- Do not modify additional files.

Instead:
1. Create a new markdown file in `_governator/task-proposed/`.
2. Name the file as you see fit, but ensure it is
    - Unique
    - kebab-case
    - Uses the `.md` extension
3. Clearly describe:
- The motivation
- The affected area
- Why it is out of scope for the current task

The system will decide whether to accept or reject the proposal.

## 8. Exit Conditions
You must exit only after one of the following actions is completed and pushed:
- The task has been moved to `task-worked`
- The task has been moved to `task-blocked`

You must not continue working after that point.

## 9. Operating Principle
Correctness and bounded execution are more important than completion. When in
doubt, block the task.
