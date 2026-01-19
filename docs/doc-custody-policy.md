# Doc Custody and Suggestion Flow Policy

## Purpose
Define how workers suggest changes to authoritative and planning documents
without editing them directly.

## Authority
- `GOVERNATOR.md` is the only authoritative spec.
- Power Six and planning docs are read-only for workers.
- Workers must use the suggestion flow below for any proposed edits.

## Suggestion Location and Format
- Location: `docs/suggestions/`
- Naming: `suggestion-YYYYMMDD-<short-slug>.md`
- Format:
  - Title
  - Target document and section
  - Problem statement
  - Proposed change (diff-style or clear replacement text)
  - Rationale and impact
  - Risk/rollback notes
  - Author, timestamp, and task reference

## Operator Workflow
- Operators review suggestion files during planning or review passes.
- Accept: apply the change directly to the authoritative doc and note the
  acceptance in the suggestion file.
- Reject: record the rejection rationale in the suggestion file.
- Archive: keep suggestion files as audit artifacts; do not delete them.

## Example
See `docs/suggestions/suggestion-20250101-doc-custody-template.md`.
