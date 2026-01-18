# Governator
The agentic anti-swarm (or: just a context management state machine)

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE.txt)
[![Status: Active Development](https://img.shields.io/badge/Status-Active%20Development-green.svg)](https://github.com/cmtonkinson/governator)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/cmtonkinson/governator/pulls)
[![CI](https://github.com/cmtonkinson/governator/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/cmtonkinson/governator/actions/workflows/ci.yml)
[![Bash 5+](https://img.shields.io/badge/Bash-5%2B-blue?logo=gnu-bash&logoColor=white)](https://www.gnu.org/software/bash/)
[![macOS](https://img.shields.io/badge/macOS-supported-lightgrey?logo=apple&logoColor=white)](https://github.com/cmtonkinson/governator)
[![Linux](https://img.shields.io/badge/Linux-supported-lightgrey?logo=linux&logoColor=white)](https://github.com/cmtonkinson/governator)

![Governator](img/governator_512.png)

## Overview
**Problem:**
Orchestrating agentic software development is Hard&trade; because
- Context windows are limited
- Context rot is a real problem
- Attention/intent drift can lead to unexplainable choices
- LLMs are great at retconning, which makes them irresponsible for
  challenging/defending their own decisions (see: the tendency to just change
  the test to make it pass)

In other words, you can only give a single agent so much scope at a time, or it
starts to make really poor choices. The ceiling of capability seems to grow
quarter after quarter as LLMs improve, but the core limitation isn't going away.

I see a lot of effort going into "swarming" but I don't see the remedy there
(yet); only high-order challenges of alignment, coordination, decision making,
and too much "human in the loop" to control the chaos. Not to mention, a ton of
noise as these systems attempt to direct inter-agent communication.

In my opinion, multi-agent development is a perfect use-case for boring old
traditional waterfall process. Sexy? Nope. Effecitve here? Hell yeah.

> Weeks of coding can save you hours of thinking.

**Solution:** The Governator
1. Takes your idea (as specified in `GOVERNATOR.md`)
2. Designs a cohesive system architecture for it
3. Decomposes that design into a plan (milestones, epics, and tasks)
4. Assigns individual tasks to async workers (coding agents)
5. Uses different agents to verify results against requirements
6. Merges approved work into `main`

Governator is a file-backed, git-driven, auditable, deterministic, waterfall
orchestration framework for converting operator intent into working software.
Goals, requirements, constraints, and assumptions are defined in
`GOVERNATOR.md`. Then Governator deploys agentic workers to assess the gap
between the stated vision and the current repo, translates that gap into
individually executable discrete tasks, and oversees planning, dispatch and
quality Control.

There is no shared memory, no long-lived agent state, and no hidden context. All
state, intent, decisions, and artifacts live on disk and in git.

Governator can be used in a completely blank repository to get something from 0
to 1, or in an existing project to improve, extend, and refine. Whatever you
want.

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
curl -fsSL https://github.com/cmtonkinson/governator/archive/refs/heads/main.tar.gz \
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
as `status`, `restart`, and `unblock` can be invoked at any time.

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
  - spawns isolated CLI agent workers to complete tasks
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
in `.governator/config.json` controls the auto-increment id.

There are some specialized task templates that are pre-programmed into the
system, mostly for initial bootstrapping and goal testing. Whenever those are
used by the control loop, they will be prefixed with the special number `000-`
just to distinguish them from anything unique to your project.

## Concurrency Controls
Governator limits concurrent work using a global cap (`worker_caps.global`) and
optional per-role caps. In-flight assignments are tracked in
`.governator/in-flight.log` (one line per task). The planner also has basic
understanding of task dependency, and Governator will respect a simplistic DAG
constraint for planning tasks.

(So if you have N available workers defined, but fewer-than-N tasks in flight,
it could be because of a milestone or task constraint.)

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
   - Governator may schedule a planner analysis task to clarify or unblock
5. `task-done/`
   - Task accepted and merged into `main`
6. `task-proposed/`
   - Optional follow-up work suggested by workers
   - Governator decides whether to accept or discard
7. `task-archive/`
   - Archived `000-` system tasks from `task-done/`, timestamp-suffixed to keep
     history

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
- Runs non-interactively (e.g. `codex exec`, `claude --print`, etc.)
- Reads inputs in order:
  1. `_governator/reasoning/<level>.md` (when needed\*)
  2. `_governator/worker-contract.md`
  3. `_governator/roles/<role>.md`
  4. `_governator/custom-prompts/_global.md`
  5. `_governator/custom-prompts/<role>.md`
  6. `_governator/<task-path>/<task>.md`
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

\* _Note: Codex CLI natively exposes a `--reasoning-effort` flag to help control
quota use, but it's not as straightforward with other agents, and it would be
both ethically and technically wrong to attempt to modify a global config file
(e.g. `~/.claude/settings.json`) for this purpose. So for agents that lack this
kind of native flag, instead we inject a custom prompt file to the front of the
"prompt queue" to approxomate the behavior. The `medium.md` prompt is empty on
purpose as we assume "medium" effort by default, and only nudging the model
explicitly in low & high cases, but the file is there if you want it._

## Review Flow
When a worker completes a task, it is pushed to the repository under a dedicated
branch. The task is moved to `task-worked` and Governator initiates a review to
determine whether the work done satisfies what was asked in the ticket.

_Note: Reviews produce a special artifact on their branch called `review.json`,
which Governator removes after processing a review decision._

## Updates
Governator can be updated at any time with `governator.sh update`. This will
automatically download the latest version and:
- for each code file: install a newer version if available
- for each prompt and template: if you haven't modified it, install the newer
  version. If you have, you are prompted with a choice to keep your changes or
  accept the newer version.
- run any unapplied bash migrations in `_governator/migrations`, recording
  applied entries in the tracked `.governator/migrations.json`.

You can pin updates to a specific tag or commit ref:
- `governator.sh update --ref v1.2.3` (release tag)
- `governator.sh update --ref abc1234` (commit SHA/prefix)

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
- a working coding agent (Codex, Claude Code, Gemini CLI)
- git
- jq
- a SHA256 tool (shasum, sha256sum, or openssl)
- cron (or some other means of invocation)
- a fully-baked `GOVERNATOR.md`
  - overview/summary
  - goals & non-goals
  - assumptions
  - constraints
  - requirements
  - any design or architecture guidance (high or low level)

## Philosophy
Correctness and bounded execution matter more than speed or cleverness.

Governator treats LLMs as workers, not collaborators. Creativity lives in
planning and review; execution is mechanical.

If a task is ambiguous, it should block. If a decision is architectural,
it should be explicit. If work cannot be reviewed, it should not be merged.

## Examples
"Does this thing actually work?" Shockingly, yes.
- A toy implementation of the [ls command in
  C](https://github.com/cmtonkinson/governator-example-ls)
- A [bitcoin trading
  bot](https://github.com/cmtonkinson/governator-example-arbitrage)
  proof-of-concept

The trading bot provides a good example of a `GOVERNATOR.md` file, while the ls
clone shows what the system is capable of producing with even minimal guidance.

## Hacking
Use `scripts/all-tests.sh` to run the full suite.
- `scripts/test.sh --fast` will run the bats tests in parallel

Dependencies for development testing live in `scripts/common.sh` and include:
- [shellcheck](https://github.com/koalaman/shellcheck) for shell linting
- [shfmt](https://github.com/patrickvane/shfmt) for formatting checks
- [bats](https://github.com/bats-core/bats-core) for test execution
- _(optional)_ [parallel](https://www.gnu.org/software/parallel/) for faster
  test runs

Governator also exposes "hidden" subcommands (for targeted testing and ops
drills). They are intentionally undocumented; check
`_governator/lib/internal.sh` for the full list and behavior.
