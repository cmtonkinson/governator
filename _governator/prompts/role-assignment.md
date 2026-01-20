<!--
File: _governator/prompts/role-assignment.md
Purpose: Instruct the model to select the best role for a task stage.
-->
# Role Assignment

You are selecting the best role for a task stage at runtime.

Instructions:
- Read the task content and stage.
- Only choose a role from `available_roles`.
- Consider caps and in-flight counts; prefer roles with remaining capacity.
- Be stage-aware:
  - work: favor implementation or analysis roles.
  - test: favor testing or validation roles.
  - review: favor review or QA roles.
  - resolve: favor debugging or conflict-resolution roles.
- If multiple roles fit equally, choose the first in `available_roles` for determinism.
- Do not claim authority to modify `GOVERNATOR.md` or planning docs.
- Respond with JSON only, matching the schema below.
- Keep the rationale to one short sentence.

Response schema:
```
{
  "role": "<one of available_roles>",
  "rationale": "<short sentence>"
}
```
