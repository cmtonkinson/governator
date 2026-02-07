# Governator
The agentic anti-swarm (or: just a context management state machine)

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE.txt)
[![CI Workflow Status](https://img.shields.io/github/actions/workflow/status/cmtonkinson/governator/ci.yml)](https://github.com/cmtonkinson/governator/actions?query=workflow%3ACI)
[![Status: Beta](https://img.shields.io/badge/Status-Beta-blue.svg)](https://github.com/cmtonkinson/governator)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/cmtonkinson/governator/pulls)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![GoDoc](https://pkg.go.dev/badge/github.com/cmtonkinson/governator.svg)](https://pkg.go.dev/github.com/cmtonkinson/governator)
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
# 1. Write your intent
vim GOVERNATOR.md

# 2. Initialize the workspace
governator init

# 3. Run the planning phase
governator plan

# 4. Execute the plan
governator execute

# 5. During planning and execution, you may:
governator status           # Show workers and tasks   
governator status --watch   # Live view of workers and tasks (q to quit)
governator tail             # Stream worker logs to your terminal (q to quit)
```

### Install

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

`GOVERNATOR.md` is the single source of project intent. This is your design-time
prompt: explain what you want. Your vision, goals, non-goals, constraints,
requirements, assumptions, and definition of done. Workers never modify it.

### Configuration

Configure during initialization with `governator init` options (see `governator init -h` for full list and defaults):

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

Governator splits work into two phases:

### Phase 1: Planning (serial)

`governator plan` launches a background supervisor that walks through a
deterministic planning pipeline defined in `_governator/planning.json`:

1. **Architecture baseline** - analyze/design the system (personas, ASR, Wardley map, arc42, C4, ADRs)
2. **Gap analysis** - compare current state to stated intent
3. **Project planning** - decompose the gap into milestones and epics
4. **Task planning** - generate discrete, individually-executable task files

Each step runs in an isolated worktree, validates its outputs, and merges back
to the base branch. Every prompt, artifact, and decision is committed to git.

### Phase 2: Execution (parallel)

`governator execute` launches the execution supervisor. It loads the task index,
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
governator [global options] <command> [command options]
```

| Command | Description |
|---------|-------------|
| `init` | Bootstrap `_governator/` in the current git repo |
| `plan` | Start the planning supervisor (background) |
| `execute` | Start the execution supervisor (background) |
| `status [-i]` | Show supervisor state and task progress (`-i` for interactive mode) |
| `stop [-w]` | Graceful shutdown (`-w` to also stop worker agents) |
| `restart [-w]` | Stop and restart the current phase |
| `reset [-w]` | Stop everything and clear all state |
| `tail [--stdout]` | Stream real-time agent output logs |
| `-V, --version` | Print version and build info |

Run `governator <command> -h` for detailed help.

---

## Directory Layout

```
_governator/
  _durable-state/         # Tracked config (config.json, migrations)
  _local-state/           # Runtime state (gitignored): logs, worktrees, workers
  docs/                   # Architecture & planning docs (generated)
  tasks/                  # Execution task files (markdown)
  index.json              # Canonical task registry
  planning.json           # Planning pipeline spec
  worker-contract.md      # Non-negotiable worker behavior rules
  roles/                  # Role prompts (architect, planner, default, ...)
  prompts/                # Planning step prompts
  custom-prompts/         # Operator overrides (_global.md, per-role)
  reasoning/              # Reasoning effort prompts (low, medium, high)
  templates/              # Architecture & planning templates
```

---

## Core Design

Why did I build this? To get PoCs from zero to one. Plain and simple.

**Determinism by design.** Governator intentionally avoids chat-based
orchestration, shared agent memory, implicit context, and conversational state.
If something matters, it exists as a file in git. This makes the system
auditable, reproducible, debuggable, and safe to automate.

**Separation of concerns.** Governator owns task creation, assignment, review,
and merging. Workers own executing exactly one task, in exactly one role, on an
isolated branch, with explicit and reviewable output. All coordination happens
through the filesystem, git branches, and markdown documents.

**Worker accountability.** Every worker invocation is staged with a deterministic
prompt stack, environment variables (`GOVERNATOR_TASK_ID`, `GOVERNATOR_WORKTREE_DIR`,
etc.), and produces an `exit.json` artifact. Audit logs track every state transition.

**Operator control.** Operators can override prompts, roles, concurrency caps, even the entire planning phase is
entirely JSON-configurable.

---

## Testing

Simply invoke `./test.sh`.

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

## Philosophy

Correctness and bounded execution matter more than speed or cleverness.

Governator treats LLMs as workers, not collaborators. Creativity lives in
planning and review; execution is mechanical.

If a task is ambiguous, it should block. If a decision is architectural,
it should be explicit. If work cannot be reviewed, it should not be merged.

---

## License

[MIT](LICENSE.txt)
