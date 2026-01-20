<!--
File: specs/v2-role-assignment-llm-contract.md
Purpose: Define the prompt and response contract for runtime role assignment.
-->
# Governator v2 Role Assignment LLM Contract

## Purpose
Define the prompt and response contract used to select a role at runtime for a
specific task stage.

## Inputs
The role assignment request is a JSON object with these fields:

`task` (object, required)
- `id` (string, required): Task identifier.
- `title` (string, optional): Task title.
- `path` (string, required): Repo-relative path to the task file.
- `content` (string, required): Full task file content.

`stage` (string, required)
- One of: `work`, `test`, `review`, `resolve`.

`available_roles` (array of strings, required)
- Deterministic, caller-provided ordering.
- The selected role must be one of these values.

`caps` (object, required)
- `global` (number, required): Global in-flight cap.
- `default_role` (number, required): Default cap for roles without overrides.
- `roles` (object, required): Map of role -> cap overrides.
- `in_flight` (object, required): Map of role -> current in-flight count.

## Prompt
The prompt used for this contract is stored at:
`_governator/prompts/role-assignment.md`.

The prompt instructs the model to:
- Read the task content and stage.
- Consider available roles and caps.
- Choose the best-fit role for the stage.
- Return JSON only, matching the response schema below.

## Response schema
The model must return a JSON object with these fields:

`role` (string, required)
- Selected role from `available_roles`.

`rationale` (string, required)
- One short sentence explaining the choice.

No additional keys are allowed; callers ignore extra keys if present.

## Deterministic fallback
If the response is invalid JSON, missing required fields, or selects a role not
in `available_roles`, the caller must:
- Select the first entry in `available_roles` as the fallback role.
- Log a warning noting the invalid response and fallback choice.

## Example input
```json
{
  "task": {
    "id": "task-014",
    "title": "Implement scheduler ordering",
    "path": "_governator/tasks/task-14-implement-scheduler-ordering.md",
    "content": "# Task 14: Implement Scheduler Ordering\n\nBuild the scheduler..."
  },
  "stage": "work",
  "available_roles": [
    "architect",
    "engineer",
    "tester"
  ],
  "caps": {
    "global": 3,
    "default_role": 2,
    "roles": {
      "architect": 1,
      "tester": 1
    },
    "in_flight": {
      "architect": 0,
      "engineer": 1,
      "tester": 0
    }
  }
}
```

## Example output (happy path)
```json
{
  "role": "engineer",
  "rationale": "Implementation work best matches the engineer role for this stage."
}
```

## Example outcome (sad path)
If the model returns invalid JSON or a role not in `available_roles`, the caller
selects the first entry (`architect` in the example input) and logs a warning.
