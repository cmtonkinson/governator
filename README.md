# Governator

[![pipeline status](https://gitlab.com/cmtonkinson/governator/badges/main/pipeline.svg)](https://gitlab.com/cmtonkinson/governator/pipelines)
[![latest tag](https://img.shields.io/gitlab/v/tag/77419954?label=latest&tag=latest)](https://gitlab.com/cmtonkinson/governator/-/tags/latest)

Governator is a deterministic, file-driven orchestration system for delegating software development work to
non-interactive LLM "workers" (e.g. Codex CLI), reviewing their output, and merging results safely into the configured
`default_branch` (default=`main`).

There is no shared memory, no long-lived agent state, and no hidden context. All state, intent, decisions, and artifacts
live on disk and in git.

---

## Quickstart
1. Install Governator at your project root.
2. Create `GOVERNATOR.md` in your project root.
2. Run `governator.sh init`
3. Schedule `governator.sh run` to run periodically.
4. Consider creating a shell alias/function.

### Installation
From your project root, run:

```bash
curl -fsSL https://gitlab.com/cmtonkinson/governator/-/archive/main/governator-main.tar.gz \
  | tar -xz --strip-components=1 -f - governator-main/_governator
```

### Configuration
`GOVERNATOR.md` is a markdown file that governs the system. It is required.
This is effectively your design-time "prompt" to control what the Governator
does. Here you should explain exactly what you want. Your vision, goals, non-
goals, ideas, hard requirements, nice to haves, assumptions, definitions of
"done," use cases, guiding principles, etc.

_TODO: add a sample `GOVERNATOR.md` for both new and existing projects._

You have to run `governator.sh init` before the system will do anything useful.

The most important question is whether this is a new or existing project: in
other words, is Governator bringing a brand new idea from 0 to 1, or
improving/extending an existing codebase? This changes bootstrapping logic and
decisions about system architecture and documenations.

### Scheduling
`governator.sh run` executes one iteration of Governator's main control loop.
Since the system effectively operates a git-based state machine, the design
intent is that `governator.sh run` can be scheduled as a cron job or
similar:


```bash
* * * * * /path/to/governator/governator.sh run
```

Internally, Governator uses a lock file, so if a `run` takes longer than your
scheduling interval, it won't cause overlaps or collisions. Other commands such
as `status` can be invoked at any time.

### Shortcut
Governator is a cool name but it's annoying to type all the time. I'd recommend
giving yourself a shortcut.

You could add an alias, but this only works from the project root:
```sh
alias gov="_governator/governator.sh"
```

What I use is a shell function so I can access it from anywhere in the project:
```sh
gov() {
  local root
  root=$(git rev-parse --show-toplevel 2>/dev/null) || return 1
  "$root/_governator/governator.sh" "$@"
}
```

---

## Why?
I created this project because as I became more time-efficient vibe coding with
CLI agents, and found some patterns that work well for me, I began to notice
some repetition in my workflow. I wanted a simple, no-overhead, understandable
system that could automate some of the same-y manual labor out of the process. I
hope this will prove to be a useful starting point.

Use at your own risk; this should be considered beta as of 2026-01-01.

It is intended for:
- autonomous code generation
- long-running background development
- constrained, reviewable LLM execution

My primary (or at least first) use-case is to get me from 0 to 1 on a proofs of
concept, because... `PoC || GFTO`, amirite?

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
1. You copy the `_governator/` directory into the project root.
2. You run `governator.sh init` to configure project mode, default branch/remote, and doc locations.
3. You write `GOVERNATOR.md` for your project.
   - this is the only authoritative description of intent
   - workers never modify it
4. You set up a cron job that periodically runs `governator.sh run`.
5. Governator:
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
├── worker-contract.md
├── custom-prompts/
│   ├── _global.md
│   ├── admin.md
│   ├── architect.md
│   ├── data_engineer.md
│   ├── planner.md
│   ├── reviewer.md
│   ├── ruby.md
│   ├── security_engineer.md
│   ├── sre.md
│   └── test_engineer.md
├── roles-worker/
│   ├── admin.md
│   ├── data_engineer.md
│   ├── planner.md
│   ├── ruby.md
│   ├── security_engineer.md
│   ├── sre.md
│   └── test_engineer.md
├── roles-special/
│   ├── architect.md
│   └── reviewer.md
├── templates/
│   ├── review.json
│   └── ticket.md
├── task-backlog/
├── task-assigned/
├── task-worked/
├── task-done/
├── task-blocked/
├── task-feedback/
└── task-proposed/
```

```
.governator/
├── default_branch
├── project_mode
├── next_ticket_id
├── reasoning_effort
├── remote_name
├── worker_timeout_seconds
├── global_worker_cap
└── worker_caps
```

### Key Concepts
- **Worker Contract** defines global, non-negotiable execution rules for all workers.

- **Roles** define authority and constraints for each type of worker (what they may and may not do).

- **Tasks** markdown files representing one unit of work, flowing through lifecycle directories.

## Ticket Naming and Assignment
Tasks are assigned to roles by their filename suffix. Filenames are kebab-case
and use a three-digit numeric id prefix:

- Example: `001-create-database-data_engineer.md`
- Example: `002-use-bundler-ruby.md`

Governator derives the role from the suffix after the last dash. If the suffix
does not match a role file in `_governator/roles-worker/`, the task is blocked.

The `templates/ticket.md` file is the stub for new tasks. `next_ticket_id`
stores the next auto-increment id.

## Concurrency Controls
Governator limits concurrent work using:

- `.governator/global_worker_cap` for the global cap (default `1`)
- `.governator/worker_caps` for per-role caps (default `1` when absent)
- `.governator/worker_timeout_seconds` for worker timeouts (default `900`)

In-flight assignments are tracked in `_governator/in-flight.log` with one line
per task:

```
001-set-up-initial-migrations-data_engineer -> data_engineer
```

## Audit Log
Governator writes fine-grained lifecycle events to `.governator/audit.log`:

```
2026-01-01T14:22Z 003-migrate-auth-sso-ruby -> assigned to ruby
2026-01-01T14:29Z 003-migrate-auth-sso-ruby -> moved to task-worked
2026-01-01T14:45Z 003-migrate-auth-sso-ruby -> moved to task-done
```

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
   - Includes a worker-written blocking reason
5. `task-feedback/`
   - The assigned worker needs additional guidance
6. `task-done/`
   - Task accepted and merged into `main`
7. `task-proposed/`
   - Optional follow-up work suggested by workers
   - Governator decides whether to accept or discard

All state transitions are explicit and reviewable.

## Worker Execution Model
Each worker execution:
- Runs non-interactively (e.g. `codex exec`)
- Reads inputs in order:
  1. `_governator/worker-contract.md`
  2. `_governator/roles-<type>/<role>.md`
  3. `_governator/custom-prompts/_global.md`
  4. `_governator/custom-prompts/<role>.md`
  5. `_governator/task-assigned/<task>.md`
- Operates in a fresh clone on a dedicated branch
- Pushes its branch exactly once
- Exits

Think of each file we "send" to the worker (ask it to read on boot) as just
an additional system prompt, because that's effectively what they are.

Workers never:
- merge to `main`
- create or modify tasks outside their scope
- retain memory between runs

## Review Flow
When a worker moves a task to `task-worked`, Governator invokes the reviewer
role defined in `_governator/roles-special/reviewer.md`. Review output is
captured in `review.json`, based on the template in
`_governator/templates/review.json`.

## Custom Prompts
`_governator/custom-prompts/` contains optional prompt files that are always
included (even if empty) to give the operator direct control over extra
instructions:

- `_global.md` applies to all workers and reviewers.
- `<role>.md` applies to the specific role.

## Determinism by Design
Governator intentionally avoids:
- chat-based orchestration
- shared agent memory
- implicit context
- conversational state

If something matters, it must exist:
- as a file
- in git

This makes the system:
- auditable
- reproducible
- debuggable
- safe to automate

## Requirements
The Governator itself is a single self-contained bash script. It will check to
ensure all of the prereqs met, and whine stubbornly if not.

It requires:
- git
- cron (or some other means of invocation)
- one or more non-interactive LLM CLIs (e.g. Codex, Claude)
- a fully-baked `GOVERNATOR.md`
  - overview
  - goals & non-goals
  - assumptions
  - constraints
  - high-level architecture

## Philosophy
Correctness and bounded execution matter more than speed or cleverness.

Governator treats LLMs as workers, not collaborators. Creativity lives in
planning and review; execution is mechanical.

If a task is ambiguous, it should block. If a decision is architectural,
it should be explicit. If work cannot be reviewed, it should not be merged.

## Hacking
Use `scripts/all-tests.sh` to run the full suite.

Dependencies for development testing live in `scripts/common.sh` and include:
- [shellcheck](https://github.com/koalaman/shellcheck) for shell linting
- [shfmt](https://github.com/patrickvane/shfmt) for formatting checks
- [bats](https://github.com/bats-core/bats-core) for test execution

Governator also exposes "hidden" subcommands (for targeted testing and ops
drills). They are intentionally undocumented; check `_governator/governator.sh`
for the full list and behavior.

### GitLab CI

The included `.gitlab-ci.yml` job installs `sgpt` and runs `./scripts/all-tests.sh`. To keep CI non-interactive:

- Define `SGPT_API_KEY` as a masked, protected GitLab CI variable that holds the API key used by `sgpt`.
- Define `SGPT_MODEL` if you want to pin a cheaper/faster model (it defaults to `gpt-4o-mini` when unset).

Once those variables are set, the pipeline can hit the real service without manual token entry.
