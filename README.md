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
to 1, or in an existing project to improve, extend, and refine.

---
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

The more detailed, specific, and precise you are, the more effective Governator
will be. That's just LLMs for you ¯\\\_(ツ)\_/¯.

### Configuration
Configure during initialization with `governator init` options (see `governator
init -h` for full list and defaults):
```bash
governator init \
  --agent claude \            # Agent CLI (codex, claude, gemini)
  --concurrency 5 \           # Max concurrent workers
  --reasoning-effort high     # Reasoning level (low, medium, high)
```
Or edit `_governator/_durable-state/config.json` post-init:

| Key | Default | Description |
|-----|---------|-------------|
| `workers.cli.default` | `"codex"` | AI CLI backend (`codex`, `claude`, or `gemini`) |
| `concurrency.global` | `1` | Max concurrent workers |
| `timeouts.worker_seconds` | `900` | Worker timeout (15 min) |
| `retries.max_attempts` | `2` | Auto-retries before blocking a task |
| `branches.base` | `"main"` | Branch that worktrees merge into |
| `reasoning_effort.default` | `"medium"` | Reasoning level (`low`, `medium`, `high`) |

Per-role overrides are available for CLI backend, concurrency caps, and reasoning effort.

---
## How It Works
Governator orchestrates planning and execution in one unified supervisor flow:

### Planning (serial)
`governator start` walks through a deterministic planning pipeline defined in `_governator/planning.json`:
1. **Architecture baseline** - analyze/design the system (personas, ASR, Wardley map, arc42, C4, ADRs)
2. **Gap analysis** - compare current state to stated intent
3. **Project planning** - decompose the gap into milestones and epics
4. **Task planning** - generate discrete, individually-executable task files

Each step runs in an isolated worktree, validates its outputs, and merges back
to the base branch. Every prompt, artifact, and decision is committed to git.

### Execution (parallel)
`governator start` then continues through execution. It loads the task index,
respects concurrency caps, and dispatches workers through the lifecycle:

```
backlog -> triaged -> implemented -> tested -> reviewed -> mergeable -> merged
```

Each worker:
- Runs non-interactively (Codex, Claude Code, or Gemini CLI)
- Operates on a dedicated branch in an isolated worktree
- Reads a deterministic prompt stack (reasoning, contract, role, custom prompts, task)
- Pushes its branch exactly once and exits
- Never merges to `main`, never retains memory between runs

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
Correctness and bounded execution matter more than speed or cleverness.

Governator treats LLMs as workers, not collaborators. Creativity lives in
planning and review; execution is mechanical.

If a task is ambiguous, it should block. If a decision is architectural,
it should be explicit. If work cannot be reviewed, it should not be merged.

---
## License
[MIT](LICENSE.txt)
