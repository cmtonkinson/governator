# ADR 0003: Use exit.json as completion contract and commit in Go

## Status
Accepted - January 27, 2026

## Context
Governator previously treated worker success as the combination of two agent
signals:

1. A git commit already exists on the task branch.
2. A stage marker file (for example, `worked.md`) exists.

In practice, worker agents have not been reliably creating commits or marker
files. Meanwhile, the dispatcher already writes `_governator/_local-state/.../exit.json`
as a deterministic signal that a worker has finished, including its exit code.

## Decision
We treat `exit.json` and the worker exit code as the primary completion
contract for all worker stages.

On a successful exit code:

- Governator captures `git status --untracked-files=all` into
  `_governator/_local-state/.../git-changes.txt` for posterity.
- Governator stages all worktree changes with `git add -A`.
- Governator creates a deterministic commit with a message of the form:
  - subject: `[<state>] <task title>`
  - body: contents of `stdout.log` (truncated)

Marker files are no longer required for stage success.

## Consequences

### Positive
- Completion is based on a mechanical signal (`exit.json`) rather than prompt
  adherence.
- Git history becomes more consistent and easier to audit.
- Worker prompts can be simplified because they no longer need to manage git
  operations.

### Tradeoffs
- A worker that exits `0` without producing meaningful changes will still be
  treated as successful.
- Commit messages now reflect worker stdout, which may be noisy or incomplete.
- Governator must handle git failures explicitly during stage finalization.

## Notes
This change is intentionally biased toward deterministic orchestration behavior
over agent-managed version control.
