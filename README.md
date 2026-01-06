# Governator
The agentic anti-swarm (read: just a state machine)

[![pipeline status](https://gitlab.com/cmtonkinson/governator/badges/main/pipeline.svg)](https://gitlab.com/cmtonkinson/governator/pipelines)

![Governator](img/governator_512.png)

More specificially, it's a file-backed, git-driven, auditable, deterministic,
waterfall orchestration framework for converting operator intent into working
software. Goals, requirements, constraints, and assumptions are defined in
`GOVERNATOR.md`. Then Governator deploys agentic workers to assess the gap
between the stated vision and the current repo, decomposes that gap into
individually executable discrete tasks, and oversees planning, dispatch and
quality control.

There is no shared memory, no long-lived agent state, and no hidden context. All
state, intent, decisions, and artifacts live on disk and in git.

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
`GOVERNATOR.md` is a markdown file that controls the system. It is required;
this is effectively your design-time "prompt" to control what the Governator
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

My primary use-case is to get me from 0 to 1 on a proofs of concept, because
`PoC || GFTO`, amirite?

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
2. You write `GOVERNATOR.md` for your project.
   - this is the only authoritative description of intent
   - workers never modify it
3. You run `governator.sh init` to configure the tool.
4. You set up a cron job that periodically runs `governator.sh run`.
5. Governator:
   - reads the repository state
   - manages project architecture & planning documentation
   - creates or updates tasks
   - assigns tasks to specific roles
   - spawns isolated Codex CLI workers to complete tasks
   - reviews results against requirements
   - merges approved work into `main`
6. You come back later to a working system.
7. Profit?

## Directory Structure
`_governator/` directory is the heart of the system. It contains the source
code, prompts, templates, and full task "database."

`.governator/` directory is the system configuration/state "database."
Customizable configuration and internal state are stored here.

Think `_governator` for shared state and `.governator` for runtime/local state.

### Key Concepts
- **Worker Contract** defines global, non-negotiable execution rules for all
  workers.

- **Roles** define authority and constraints for each type of worker (what they
  may and may not do).

- **Tasks** markdown files representing one unit of work, flowing through
  lifecycle directories.

## Task Naming and Assignment
Tasks are created with a 3-digit numeric prefix, a kebab-case slugified name, 
and a suffix matching the type of role the task should be assigned to.
- Example: `001-create-database-data_engineer.md`
- Example: `002-use-bundler-ruby.md`

Governator derives the role from the suffix after the last dash. If the suffix
does not match a role file in `_governator/roles/`, the task will be
assigned to the default `generalist` role.

The `templates/task.md` file is the stub for new tasks. `next_task_id`
stores the next auto-increment id.

There are some specialized task templates that are pre-programmed into the
system, mostly for initial bootstrapping and goal testing. Whenever those are
used by the control loop, they will be prefixed with the special number `000-`
just to distinguish them from anything unique to your project.

## Concurrency Controls
Governator limits concurrent work using:

- `.governator/global_worker_cap` for the global cap (default `1`)
- `.governator/worker_caps` for per-role caps (default `1` when absent)
- `.governator/worker_timeout_seconds` for worker timeouts (default `900`)

In-flight assignments are tracked in `.governator/in-flight.log` with one line
per task:

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
5. `task-done/`
   - Task accepted and merged into `main`
6. `task-proposed/`
   - Optional follow-up work suggested by workers
   - Governator decides whether to accept or discard

All state transitions are explicit and reviewable.

## Architecture Discovery and Project Planning
Governator relies on A) software architecture and B) project planning documents
to drive its workflow.

The key software architecture documents (the "Power Six" artifacts) are:
1. User Personas
2. Architecturally Significant Requirements
3. Wardley Map
4. Architecture Overview (in the "arc42" style)
5. C4 Model Diagrams
5. Architecture Decision Records

All of these have built-in predefined templates.

Governator will create canonical documentation in `_governator/docs` for these
artifacts (as appropriate), whether that means analyzing the current codebase or
synthesizing a new system design from scratch.

Once the system architecture is created/discovered and documented, Governator
moves on to the planning phase. By comparing the current system to the intent
specified in `GOVERNATOR.md`, Governator maps out a set of one or more project
milestones, then decomposes each of those into one or more epics. This structure
helps the system reason more reliably and produce more predictably useful
results. These files are written to:
- `_governator/docs/milestones.md`
- `_governator/docs/epics.md`

It is each epic, with the architectural documentation as context, which is used
to generate the actual tasks that get carried out by worker processes.

## Worker Execution Model
Each worker execution:
- Runs non-interactively (e.g. `codex exec`)
- Reads inputs in order:
  1. `_governator/worker-contract.md`
  2. `_governator/roles/<role>.md`
  3. `_governator/custom-prompts/_global.md`
  4. `_governator/custom-prompts/<role>.md`
  5. `_governator/<task-path>/<task>.md`
- Operates in a fresh clone on a dedicated branch
- Pushes its branch exactly once
- Exits

Each file we "send to" the worker (read: each file we prompt it to read) is just
another layer of prompt/context for execution. `_governator/custom-prompts/`
contains optional prompt files that are always included (even if empty) to give
the operator direct control over extra instructions:
- `_global.md` applies to all workers and reviewers.
- `<role>.md` applies to the specific role.

Workers never:
- merge to `main`
- create or modify tasks outside their scope
- retain memory between runs

## Review Flow
When a worker completes a task, it is pushed to the repository under a dedicated
branch. The task is moved to `task-worked` and Governator initiates a review to
determine whether the work done satisfies what was asked in the ticket.

Reviews produce a special artifact on their branch called `review.json`, which
Governator removes when marking an approved task as done.

## Updates
Governator can be updated at any time with `governator.sh update`. This will
automatically download the latest version and:
- for each code file: install a newer version if available
- for each prompt and template: if you haven't modified it, install the newer
  version. If you have, you are prompted with a choice to keep your changes or
  accept the newer version.
- run any unapplied bash migrations in `_governator/migrations`, recording
  applied entries in the tracked `.governator/migrations.json`.

_**Note**: `update` itself will never modify any tasks or docs; migrations are
opt-in and can change anything they touch. Most of the other stuff under
`_governator` is fair game._

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
The Governator itself is a CLI application written in bash. It will check to
ensure all of the prereqs met, and whine stubbornly if not.

It requires:
- git
- cron (or some other means of invocation)
- one or more non-interactive LLM CLIs (currently only supports Codex CLI)
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

The included `.gitlab-ci.yml` job runs `./scripts/all-tests.sh`.
