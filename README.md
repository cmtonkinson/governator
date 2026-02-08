# Governator
The agentic anti-swarm (or: just a context management state machine)

[![Status: Beta](https://img.shields.io/badge/Status-Beta-blue.svg)](https://github.com/cmtonkinson/governator)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/cmtonkinson/governator/pulls)
[![CI Workflow Status](https://img.shields.io/github/actions/workflow/status/cmtonkinson/governator/ci.yml)](https://github.com/cmtonkinson/governator/actions?query=workflow%3ACI)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![macOS](https://img.shields.io/badge/macOS-supported-lightgrey?logo=apple&logoColor=white)](https://github.com/cmtonkinson/governator)
[![Linux](https://img.shields.io/badge/Linux-supported-lightgrey?logo=linux&logoColor=white)](https://github.com/cmtonkinson/governator)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE.txt)

![Governator](img/governator_512.png)

## Quick Start
```bash
# 0. Install
brew install cmtonkinson/tap/governator

# 1. Write your intent - document anything and everything you want: scope,
# context, requirements, boundaries, constraints, assumptions, stack, etc.
vim GOVERNATOR.md

# 2. Initialize the workspace
governator init

# 3. Begin orchestration (Governator plans first, then implements)
governator start

# 4a. (Optional)
# Say "Hasta la vista, baby," and go do something else for a while.

# 4b. (Optional) During orchestration, inspect the system via:
governator status    # Show workers and tasks
governator tail      # Stream both stderr/stdout worker logs (q to quit)
governator why       # Recent supervisor + blocked/failed task logs

# 5. Profit?
```

## Overview
**Problem:** Orchestrating agentic software development is Hard&trade; because
- Context windows are limited
- Context rot is a real problem
- Attention/intent drift can lead to unexplainable choices
- LLMs are great at retconning, which makes them irresponsible for
  challenging/defending their own decisions (see: the tendency to just change
  the test to make it pass)

You can only give a single agent so much scope at a time, or it starts to make
really poor choices. Multi-agent development is a perfect use-case for boring
old traditional waterfall process. Sexy? Nope. Effective here? Hell yeah.

> Weeks of coding can save you hours of thinking.

**Solution:** The Governator
1. Takes your idea (as specified in `GOVERNATOR.md`)
2. Designs a cohesive system architecture for it
3. Decomposes that design into a plan (milestones, epics, and tasks)
4. Assigns individual tasks to async workers (coding agents)
5. Uses different agents to verify results against requirements
6. Merges approved work into `main`
Governator is a file-backed, git-driven, auditable, waterfall orchestration
framework for converting operator intent into working software. There is no
shared memory, no long-lived agent state, and no hidden context. All state,
intent, decisions, and artifacts live on disk and in git.

Governator can be used in a completely blank repository to get something from 0
to 1 (this is why it was initially built), or in an existing project to improve,
extend, refine, and maintain.

### Installation Options
**Homebrew** (macOS/Linux):
```bash
brew install cmtonkinson/tap/governator
```

**Go install** (requires Go 1.25+):
```bash
go install github.com/cmtonkinson/governator@latest
```

**Debian/Ubuntu** (.deb package):
```bash
# Download the latest release for your architecture
wget https://github.com/cmtonkinson/governator/releases/latest/download/governator_<version>_amd64.deb

# Install
sudo dpkg -i governator_<version>_amd64.deb
```

**From source**:
```bash
git clone https://github.com/cmtonkinson/governator.git
cd governator
go build -o governator .
sudo mv governator /usr/local/bin/
```

### GOVERNATOR.md
`GOVERNATOR.md` is your design-time prompt; the north star and single source of
truth for system behavior. Consider including:
- Scope
- Context
- Use-cases
- Requirements
- Boundaries
- Constraints
- Assumptions
- Goals
- Non-goals
- Stack
- Example input/output
- References to existing work

The more detailed and precise you are, the more effective Governator will be.
That's just LLMs for you ¯\\\_(ツ)\_/¯

### Configuration
Some settings can be tuned during initialization with `governator init` options
(see `governator init -h` for full list and defaults):
```bash
governator init \
  --agent claude \            # Use Claude Code as the default agent 
  --concurrency 5 \           # Allow up to 5 concurrent agents
  --reasoning-effort high     # API budget? What's that?
```
You can always edit `_governator/_durable-state/config.json` post-init.

---
## How It Works
Governator works in three discrete "phases": planning, traige, and execution. A
supervisor process is responsible for orchestrating these phases.

### Planning
Up front, Governator loads the planning pipeline from
`_governator/planning.json` and runs each step, serially. Out of the box,
Governator ships with an opinionated planning pipeline:
1. **Architecture baseline** - analyze/design the system (personas, ASRs,
   arc42, Wardley map, C4, and ADRs)
2. **Gap analysis** - compare current project to documented intent (for
   greenfield, the gap will be "everything")
3. **Project planning** - decompose the gap into milestones and epics
4. **Task planning** - generate discrete, individually-executable task files

Whether you use the default planning logic or roll your own, the planning
pipeline is considered successful if-and-only-if there are task files in the
`_governator/tasks/` directory.

### Triage
Once planning is complete, Governator:
1. Loads all new task files into the backlog.
2. Looks across the backlog (plus any tasks which may have been previously
   triaged) and generates a Directed Acyclic Graph (the DAG) of dependencies.
3. Writes dependency information to the task index.
4. Dependency-resolved backlog tasks are moved into triage (the ready queue).

### Execution
With intent, a plan, and an open set of dependency-ordered tasks, Governator
begins dispatching non-interactive coding agents ("workers") asynchronously to
implement each task.

When a triaged task is first dispatched, a branch is created from main and
worktree is generated, both of which are preserved and reused for the remainder
of the task's lifecycle. When a task is completed, the worktree is deleted and
the branch is merged into main. Each worker assigned to that task is given its
own directory for invocation state, logs, etc.

_Note: In practice, the DAG usually winds up being the primary limiting factor
to effective parallelism during execution, so if you have allowed `C` amount of
concurrency per your config but see `< C` active workers, check the DAG._

### Task Lifecycle
On the happy path, tasks progress through the followig states:
```
backlog -> triaged -> implemented -> tested -> reviewed -> mergeable -> merged
```

### Re-planning
Governator is billed as a "waterfall" system but of course you don't get
everything right up front. When a worker needs to change architecture or
planning documents, Governator will detect those changes using file digests.
When that occurs, it will cease new task dispatch. Once all active workers have
stopped, Governator will restart planning and triage.

In practice, this likely only happens when a worker needs to add a new ADR, but
since those are core architectural documents and may invalidate or modify
exsiting data (not just add new work to the end of the project), the safe thing
to do is replan.

---
## CLI Reference
```
governator --help
governator - AI-powered task orchestration engine

USAGE:
    governator [global options] <command> [command options]

GLOBAL OPTIONS:
    -h, --help       Show this help message
    -v, --verbose    Enable verbose output for debugging
    -V, --version    Print version and build information

COMMANDS:
    init             Bootstrap a new governator workspace in the current repository
    start            Start the unified supervisor to plan, triage, and execute work
    plan             Deprecated alias for 'start'
    execute          Deprecated alias for 'start'
    status           Display current supervisor and task status
    why              Show the most recent supervisor log lines
    dag              Display task dependency graph
    stop             Stop the running supervisor gracefully
    restart          Stop and restart the current supervisor phase
    reset            Stop supervisor and clear all state (nuclear option)
    tail             Stream agent output logs in real-time

Run 'governator <command> -h' for command-specific help.
```

### Command Options
```text
governator init [options]
  -a, --agent <cli>             Set default worker CLI (codex, claude, gemini)
  -c, --concurrency <n>         Set global and default role concurrency limit
  -r, --reasoning-effort <lvl>  Set default reasoning effort (low, medium, high)
  -b, --branch <name>           Set base branch name (default: main)
  -t, --timeout <seconds>       Set worker timeout in seconds (default: 900)

governator status [options]
  -i, --interactive             Enable interactive mode with live task updates

governator why [options]
  -s, --supervisor-lines <n>    Supervisor trailing lines (default: 20)
  -t, --task-lines <n>          Per-task trailing lines (default: 20)

governator stop|restart|reset [options]
  -w, --worker                  Also stop running worker agents

governator dag [options]
  -i, --interactive             Enable interactive DAG mode (not yet implemented)

governator tail [options]
  --stdout                      Include stdout stream in addition to stderr
  --both                        Alias for --stdout (include both stdout and stderr)
```

---
## Directory Layout
```
_governator/
  .gitignore              # Ignores runtime-only local state
  _durable-state/         # Tracked config (config.json, migrations)
    migrations/           # Config/data migrations
  _local-state/           # Runtime state (gitignored): logs, worktrees, workers
    index.json            # Canonical task registry (runtime)
  docs/                   # Architecture & planning docs (generated)
  tasks/                  # Execution task files (markdown)
  planning.json           # Planning pipeline spec
  worker-contract.md      # Non-negotiable worker behavior rules
  roles/                  # Role prompts (architect, planner, default, ...)
  prompts/                # Planning step prompts
  custom-prompts/         # Operator overrides (_global.md, per-role)
  reasoning/              # Reasoning effort prompts (low, medium, high)
  templates/              # Architecture & planning templates
```
---
## Testing
Execute all automated verification by running `./test.sh`.

```text
./test.sh -h
Usage: ./test.sh [options]

Options:
  -a, --all        Run lint, native, and E2E tests (default).
  -n, --native     Run native tests only.
  -e, --e2e        Run E2E tests only.
  -l, --lint       Run lint checks only.
  -q, --quiet      Suppress go test output (failures still surface).
  -v, --verbose    Enable verbose go test output (default).
  -h, --help       Show this help message.
  -e2e-preserve-all     Preserve all E2E test repositories.
  -e2e-clear-all        Clear all E2E test repositories, even on failure.

Examples:
  ./test.sh -a
  ./test.sh --e2e --e2e-preserve-all
```

---
## Core Design
Why did I build this? To get PoCs from zero to one.

**Determinism by design.** Governator intentionally avoids chat-based
orchestration, shared agent memory, implicit context, and conversational state.
If something matters, it exists as a file. This makes the system auditable,
reproducible, debuggable, and safe to automate.

**Separation of concerns.** Governator owns task creation, assignment, review,
and merging. Workers own executing exactly one task, in exactly one role, on an
isolated branch, with explicit and reviewable output. All coordination happens
through the filesystem, git branches, and markdown documents.

**Worker accountability.** Every worker invocation is staged with a
deterministic prompt stack, environment variables (`GOVERNATOR_TASK_ID`,
`GOVERNATOR_WORKTREE_DIR`, etc.), and produces an `exit.json` artifact. Audit
logs track every state transition.

**Operator control.** Operators can override prompts, roles, concurrency caps,
even the entire planning stage itself is entirely JSON-configurable.

### Philosophy
- Correctness and bounded execution matter more than speed or cleverness.
- If a task is ambiguous, it should block.
- If a decision is architectural, it should be explicit.
- If work cannot be verified, it should not be merged.

---
## License
[MIT](LICENSE.txt)
