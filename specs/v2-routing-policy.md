<!--
File: specs/v2-routing-policy.md
Purpose: Define deterministic routing for task selection across stages and roles.
-->
# Governator v2 Routing Policy

This document defines deterministic routing for selecting which tasks and roles
are dispatched when capacity is available.

## Inputs
- Task index entries (state, role, order, id, dependencies).
- Global concurrency cap.
- Default-role cap.
- Per-role caps (optional overrides).

## Stages and routing intent
Routing prioritizes tasks closer to completion. State is mapped to a routing
stage for dispatch intent.

| State | Stage | Dispatch intent |
| --- | --- | --- |
| `conflict` | conflict | conflict resolution |
| `resolved` | conflict | review/merge retry |
| `tested` | review | review/merge |
| `worked` | test | test |
| `open` | work | implementation |

## Deterministic ordering
1. Stage priority (highest first): `conflict` -> `tested` -> `worked` -> `open`.
2. Within the same stage, order by `order` (ascending).
3. Final tie-breaker: stable task id (lexicographic ascending).

Stage priority always overrides planning order to ensure close-to-done work is
routed first.

## Cap application
- Global cap limits the total number of dispatched tasks per routing pass.
- Default-role cap applies to any role without an explicit per-role cap.
- Per-role cap overrides the default cap for that role only.
- Caps are enforced while iterating through the deterministic ordering list.

## Selection algorithm
1. Filter to eligible tasks (dependencies satisfied, retries available, and
   state in `open`, `worked`, `tested`, `conflict`, or `resolved`).
2. Sort eligible tasks by stage priority, `order`, then id.
3. Iterate in sorted order, selecting a task when:
   - Global cap has remaining capacity, and
   - The task's role cap (per-role or default) has remaining capacity.
4. Stop when the global cap is reached or the list is exhausted.

## Example (global cap 5, default-role cap 3)
Caps:
- Global cap: 5
- Default-role cap: 3
- Per-role caps: `reviewer` = 1

Eligible tasks (already dependency-clean), shown in deterministic order:

| id | state | stage | role | order |
| --- | --- | --- | --- | --- |
| T-01 | conflict | conflict | resolver | 10 |
| T-02 | open | work | worker | 20 |
| T-03 | open | work | worker | 30 |
| T-04 | open | work | worker | 40 |
| T-05 | open | work | worker | 50 |
| T-06 | open | work | tester | 60 |
| T-07 | open | work | tester | 70 |

Routing result:
- Select T-01 (conflict first).
- Select T-02, T-03, T-04 (worker hits default-role cap of 3).
- Skip T-05 (worker cap exhausted).
- Select T-06 (global cap still has room).
- Stop after T-06 (global cap 5 reached; T-07 not selected).

This example shows conflict priority and the default-role cap blocking the
fourth worker task even though global capacity remains.
