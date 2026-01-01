# Governator

Governator is a deterministic, file-driven orchestration system for delegating software development work to
non-interactive LLM workers (e.g. Codex), reviewing their output, and merging results safely into `main`.

It is designed to be dropped into an existing repository, alongside a human-written `README.md`, and left to operate
autonomously via a cron-driven control loop.

There is no shared memory, no long-lived agent state, and no hidden context. All state, intent, decisions, and artifacts
live on disk and in git.

---

## Core Idea

Governator enforces a strict separation of concerns:

- Governator Owns:
  - task creation
  - task assignment
  - review and acceptance
  - merging to `main`

- Workers (LLM-driven, non-interactive) Own:
  - executing exactly one assigned task
  - within exactly one role
  - on an isolated branch
  - with explicit, reviewable output

All coordination happens through:
- the filesystem
- git branches
- markdown documents

There is no conversational back-and-forth.

## High-Level Workflow

1. You write a `README.md` for your project.
   - this is the only authoritative description of intent
   - workers never modify it
2. You copy the `_governator/` directory into the project root.
3. You set up a cron job that periodically runs `governator.sh`.
4. Governator:
   - reads the repository state
   - creates or updates task files
   - assigns tasks to roles
   - spawns isolated worker executions
   - reviews results
   - merges approved work into `main`
5. You come back later to a working system.

## Directory Structure

```
_governator/
├── governator.sh
├── worker_contract.md
├── roles/
│   ├── architect.md
│   ├── planner.md
│   ├── reviewer.md
│   ├── ruby.md
│   ├── data_engineer.md
│   ├── security_engineer.md
│   ├── sre.md
│   └── admin.md
├── task-backlog/
├── task-assigned/
├── task-worked/
├── task-done/
├── task-blocked/
├── task-feedback/
└── task-proposed/
```

### Key Concepts

- **Worker Contract** defines global, non-negotiable execution rules for all workers.

- **Roles** define authority and constraints for each type of worker (what they may and may not do).

- **Tasks** markdown files representing one unit of work, flowing through lifecycle directories.

## Task Lifecycle

A task moves through directories as its state changes:

1. `task-backlog/`
   - Accepted work, not yet assigned
2. `task-assigned/`
   - Assigned to a specific role and branch
   - Actively being worked by a worker
3. `task-worked/`
   - Worker claims task completion
   - Includes a worker-written summary
4. `task-blocked/`
   - Worker cannot proceed safely
   - Includes an explicit blocking reason
5. `task-feedback/`
   - Reviewer feedback requiring rework
6. `task-done/`
   - Task accepted and merged into `main`
7. `task-proposed/`
   - Optional follow-up work suggested by workers
   - Governator decides whether to accept or discard

All state transitions are explicit and reviewable.

## Worker Execution Model

Each worker execution:
- Runs non-interactively (e.g. `codex exec`)
- Reads exactly three inputs, in order:
  1. `worker_contract.md`
  2. a role file from `roles/`
  3. one task file from `task-assigned/`
- Operates in a fresh clone on a dedicated branch
- Pushes its branch exactly once
- Exits

Workers never:
- merge to `main`
- create or modify tasks outside their scope
- retain memory between runs

## Determinism by Design

Governator intentionally avoids:
- chat-based orchestration
- shared agent memory
- implicit context
- conversational state

If something matters, it must exist:
- as a file
- in git
- or in the README

This makes the system:
- auditable
- reproducible
- debuggable
- safe to automate

## Requirements
- git
- [shell_gpt](https://github.com/TheR1D/shell_gpt)
- cron (or equivalent scheduler)
- one or more non-interactive LLM CLIs (e.g. Codex, Claude)
- a fully-baked `README.md`
  - overview
  - goals & non-goals
  - assumptions
  - constraints
  - high-level architecture

The Governator script itself is shell-based and intentionally simple.

## Philosophy

Correctness and bounded execution matter more than speed or cleverness.

Governator treats LLMs as workers, not collaborators. Creativity lives in planning and review; execution is mechanical.

If a task is ambiguous, it should block. If a decision is architectural, it should be explicit. If work cannot be
reviewed, it should not be merged.

## Status

This project is experimental and opinionated.

It is intended for:
- autonomous code generation
- long-running background development
- constrained, reviewable LLM execution

Use at your own risk, preferably while sleeping.

