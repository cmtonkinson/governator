# LLM Quota Exhaustion Detection Plan

## Goals
- Detect when a worker failed because the provider is out of credits or quota.
- Distinguish temporary rate limits from hard quota exhaustion.
- Avoid repeatedly dispatching workers that cannot run.

## Detection Hook
- Integrate detection in `_governator/lib/workers.sh` inside `check_zombie_workers` before `handle_zombie_failure`.
- Inspect the worker log (for example `_governator/_local_state/logs/<task>.log` or reviewer log) because CLI exit codes are not captured.

## Classification Signals
- `temporary_rate_limit`:
  - `429`, `rate limit`, `overloaded`, `try again`.
  - Gemini: `RESOURCE_EXHAUSTED`.
  - Claude: `rate_limit_error`.
- `hard_quota_exhausted`:
  - `insufficient_quota`, `credit balance too low`, `billing hard limit`, `exceeded your current quota`.
- `unknown_failure`:
  - No match, fall back to existing retry or block logic.

## Provider State and Backoff
- Maintain a `_governator/_local_state/provider-status.json` keyed by provider.
- On `temporary_rate_limit`, set a cooldown timestamp with exponential backoff.
- On `hard_quota_exhausted`, mark provider as exhausted and skip new dispatches.
- Enforce cooldown and exhaustion checks in `_governator/lib/queues.sh` before assignment.

## Task State Handling
- `temporary_rate_limit`: retry the task after cooldown and log the reason in the audit log.
- `hard_quota_exhausted`: block the task with a clear reason and log the provider status.

## Tests
- Add bats tests for classifier behavior based on sample logs for Codex, Claude, and Gemini.
- Add a test verifying cooldown enforcement in assignment logic.
