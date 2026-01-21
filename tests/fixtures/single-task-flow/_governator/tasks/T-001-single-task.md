# Task: Single task flow

## Objective
Verify that a repository with exactly one open worker task reports status and dependencies correctly.

## Context
This task represents the most basic Governator run: a single open work item with no dependencies or overlaps.

## Requirements
- A worker role is assigned so the scheduler can select the task.
- The open state should be visible to reporting tools.
- No dependencies, retries, or overlap conflicts should exist.
