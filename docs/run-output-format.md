# Run Output Format

Governator `run` emits a small set of deterministic, machine-friendly lines whenever work happens so operators and automation can see task status without digging through logs.

## Task events

Each task event line is emitted to stdout and follows this shape:

```
task=<id> role=<role> stage=<stage> status=<status> [reason="..."] [timeout_seconds=<n>]
```

- `task` and `role` are always present.
- `stage` reflects the worker lifecycle stage (`test`, `review`, `resolve`, or `merge`).
- `status` is one of `start`, `complete`, `failure`, or `timeout`.
- `reason` is quoted when present and carries human-friendly context (e.g. why a task failed).
- `timeout_seconds` is attached to timeout records so callers can correlate with configured timeouts.

Examples:

```
task=T-001 role=test-engineer stage=test status=start
```
```
task=T-001 role=test-engineer stage=test status=complete
```
```
task=T-002 role=reviewer stage=review status=timeout reason="worker execution timed out" timeout_seconds=120
```
```
task=T-003 role=reviewer stage=review status=failure reason="missing merge marker"
```

## Planning drift

When planning artifacts no longer match the workspace, `run` writes a single guidance line before exiting with `ErrPlanningDrift`:

```
planning=drift status=blocked reason="..." next_step="governator plan"
```

The `reason` field repeats the drift detection summary and `next_step` tells operators which command to run to recover.
